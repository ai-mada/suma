package repositoryprovider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func (s *Server) sourceInspectionHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.cfg.SourceReadTokenEnv == "" || !token(r, s.cfg.SourceReadTokenEnv) {
		writeError(w, 403, "repository_source_forbidden")
		return
	}
	if r.Method != http.MethodPost || r.URL.RawQuery != "" {
		writeError(w, 400, "repository_source_invalid")
		return
	}
	var request ri.RepositorySourceInspectionRequest
	if decode(r, &request) != nil {
		writeError(w, 400, "repository_source_invalid")
		return
	}
	if request.TenantID != s.cfg.TenantID || request.ProjectID != s.cfg.ProjectID || request.ProviderID != s.cfg.ProviderID {
		writeError(w, 403, "repository_source_forbidden")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	response, err := s.inspectSource(ctx, request)
	if err != nil {
		writeError(w, 409, "repository_source_unavailable")
		return
	}
	writeJSON(w, 200, response)
}

// inspectSource only reads immutable Git objects in an already registered bare
// target. It does not check out files, invoke filters, install or run scripts.
func (s *Server) inspectSource(ctx context.Context, q ri.RepositorySourceInspectionRequest) (out ri.ProviderSourceInspection, err error) {
	if ri.ValidateRepositorySourceInspectionRequest(q) != nil || q.ProviderID != s.cfg.ProviderID || q.TenantID != s.cfg.TenantID || q.ProjectID != s.cfg.ProjectID {
		return out, fmt.Errorf("source binding is unavailable")
	}
	t := s.targets[q.TargetAlias]
	if t == nil || t.config.Ref != q.TargetRef || t.configSHA != q.TargetConfigSHA256 {
		return out, fmt.Errorf("source target changed")
	}
	if err = s.validateTarget(ctx, t, false); err != nil {
		return out, err
	}
	op, err := s.load(q.OperationID)
	if err != nil {
		return out, err
	}
	e := op.Request.RepositoryExecution
	if e.Phase != "apply" || e.TenantID != q.TenantID || e.ProjectID != q.ProjectID || e.ProviderID != q.ProviderID || e.TargetID != q.TargetID || e.TargetAlias != q.TargetAlias || e.TargetRef != q.TargetRef || e.TargetConfigSHA256 != q.TargetConfigSHA256 || op.Observation.RequestSHA256 != q.RequestSHA256 {
		return out, fmt.Errorf("original source apply differs")
	}
	proof, err := s.inspect(ctx, t, op)
	if err != nil || proof.State != "applied" || proof.Receipt == nil || proof.Receipt.State != "applied" || proof.ReceiptSHA256 != q.ReceiptSHA256 || proof.Receipt.ResultCommit != q.ExactCommit || ri.RepositoryValidateObservation(proof, e, q.RequestSHA256) != nil {
		return out, fmt.Errorf("original source receipt is unavailable")
	}
	typeRaw, err := s.git(ctx, t, nil, 4096, "cat-file", "-t", q.ExactCommit)
	if err != nil || strings.TrimSpace(string(typeRaw)) != "commit" {
		return out, fmt.Errorf("source commit is unavailable")
	}
	treeRaw, err := s.git(ctx, t, nil, 4096, "rev-parse", "--verify", q.ExactCommit+"^{tree}")
	if err != nil {
		return out, err
	}
	tree := strings.TrimSpace(string(treeRaw))
	if !cc.ValidCommit(tree) {
		return out, fmt.Errorf("invalid source tree")
	}
	requestSHA, _ := ri.RepositorySourceInspectionRequestDigest(q)
	out = ri.ProviderSourceInspection{Protocol: ri.RepositorySourceInspectionProtocol, InspectionRequestSHA256: requestSHA, ExactCommit: q.ExactCommit, TreeOID: tree, Scopes: []ri.RepositorySourceInspectionScope{}, Manifests: []ri.RepositorySourceManifest{}, Coverage: ri.RepositorySourceInspectionCoverage{State: "complete", Reasons: []string{}}, Limits: ri.RepositorySourceLimits()}
	reasons := map[string]bool{}
	partial := func(code string) { out.Coverage.State = "partial"; reasons[code] = true }
	for _, root := range q.RelativeRoots {
		scopeTree := tree
		if root != "" {
			raw, e := s.git(ctx, t, nil, 4096, "rev-parse", "--verify", q.ExactCommit+":"+root)
			if e != nil {
				return out, fmt.Errorf("source root is absent")
			}
			scopeTree = strings.TrimSpace(string(raw))
			kind, e := s.git(ctx, t, nil, 4096, "cat-file", "-t", scopeTree)
			if !cc.ValidCommit(scopeTree) || e != nil || strings.TrimSpace(string(kind)) != "tree" {
				return out, fmt.Errorf("source root is not a Git tree")
			}
		}
		out.Scopes = append(out.Scopes, ri.RepositorySourceInspectionScope{RelativeRoot: root, TreeOID: scopeTree})
		if out.Coverage.TreeEntries >= ri.RepositorySourceMaxTreeEntries {
			partial("tree_entry_limit")
			continue
		}
		// A byte bound also protects against a single pathological Git path.
		raw, e := s.git(ctx, t, nil, 12<<20, "ls-tree", "-r", "-t", "-l", "-z", scopeTree)
		if e != nil {
			partial("tree_scan_unavailable")
			continue
		}
		for _, entry := range strings.Split(string(raw), "\x00") {
			if entry == "" {
				continue
			}
			if out.Coverage.TreeEntries >= ri.RepositorySourceMaxTreeEntries {
				partial("tree_entry_limit")
				break
			}
			out.Coverage.TreeEntries++
			header, name, ok := strings.Cut(entry, "\t")
			parts := strings.Fields(header)
			if !ok || len(parts) != 4 || !utf8.ValidString(name) || !ri.RepositorySourceValidPath(name, false) {
				out.Coverage.SkippedEntries++
				partial("unsupported_git_path")
				continue
			}
			if root != "" {
				name = root + "/" + name
			}
			if !ri.RepositorySourceValidPath(name, false) {
				out.Coverage.SkippedEntries++
				partial("unsupported_git_path")
				continue
			}
			if parts[1] == "tree" {
				continue
			}
			if parts[1] != "blob" || parts[0] != "100644" && parts[0] != "100755" {
				out.Coverage.SkippedEntries++
				partial("linked_or_submodule_entry")
				continue
			}
			format := ri.RepositorySourceManifestFormat(name)
			if format == "" {
				continue
			}
			if len(out.Manifests) >= ri.RepositorySourceMaxManifests {
				out.Coverage.SkippedEntries++
				partial("manifest_count_limit")
				continue
			}
			size, err := strconv.ParseInt(parts[3], 10, 64)
			if err != nil || size < 0 || size > ri.RepositorySourceMaxFileBytes || out.Coverage.ContentBytes+size > ri.RepositorySourceMaxContentBytes {
				out.Coverage.SkippedEntries++
				partial("manifest_byte_limit")
				continue
			}
			if !cc.ValidCommit(parts[2]) {
				return out, fmt.Errorf("invalid source object")
			}
			out.Manifests = append(out.Manifests, ri.RepositorySourceManifest{Path: name, BlobOID: parts[2], SizeBytes: size, Format: format})
			out.Coverage.ContentBytes += size
		}
	}
	sort.Slice(out.Manifests, func(i, j int) bool { return out.Manifests[i].Path < out.Manifests[j].Path })
	out.Coverage.ManifestCount = len(out.Manifests)
	for reason := range reasons {
		out.Coverage.Reasons = append(out.Coverage.Reasons, reason)
	}
	sort.Strings(out.Coverage.Reasons)
	// Batch raw object reads avoid one subprocess per package in large bounded
	// workspaces. --batch does not use working-tree filters or source scripts.
	if len(out.Manifests) > 0 {
		var objects strings.Builder
		for _, file := range out.Manifests {
			objects.WriteString(file.BlobOID + "\n")
		}
		raw, e := s.git(ctx, t, strings.NewReader(objects.String()), ri.RepositorySourceMaxContentBytes+32768, "cat-file", "--batch")
		if e != nil {
			return out, fmt.Errorf("source blobs are unavailable")
		}
		reader := bufio.NewReader(bytes.NewReader(raw))
		for i := range out.Manifests {
			file := &out.Manifests[i]
			header, e := reader.ReadString('\n')
			parts := strings.Fields(header)
			if e != nil || len(parts) != 3 || parts[0] != file.BlobOID || parts[1] != "blob" {
				return out, fmt.Errorf("source batch object differs")
			}
			size, e := strconv.ParseInt(parts[2], 10, 64)
			if e != nil || size != file.SizeBytes {
				return out, fmt.Errorf("source batch size differs")
			}
			content := make([]byte, int(size))
			if _, e = io.ReadFull(reader, content); e != nil {
				return out, fmt.Errorf("source batch content incomplete")
			}
			terminator, e := reader.ReadByte()
			if e != nil || terminator != '\n' {
				return out, fmt.Errorf("source batch terminator differs")
			}
			file.SHA256 = hashBytes(content)
			file.ContentBase64 = base64.StdEncoding.EncodeToString(content)
		}
		if _, e := reader.ReadByte(); e != io.EOF {
			return out, fmt.Errorf("unexpected source batch object")
		}
	}
	// Recheck the original receipt and target identity after bounded object IO.
	if err = s.validateTarget(ctx, t, false); err != nil {
		return out, err
	}
	again, err := s.inspect(ctx, t, op)
	if err != nil || again.ReceiptSHA256 != q.ReceiptSHA256 || again.State != "applied" {
		return out, fmt.Errorf("source receipt changed")
	}
	out.SnapshotSHA256, err = ri.RepositorySourceInspectionDigest(out)
	if err != nil {
		return out, err
	}
	if err = ri.ValidateProviderSourceInspection(q, out); err != nil {
		return out, err
	}
	encoded, err := json.Marshal(out)
	if err != nil || len(encoded) > ri.RepositorySourceMaxTransportBytes {
		return out, fmt.Errorf("source transport limit")
	}
	return out, nil
}
