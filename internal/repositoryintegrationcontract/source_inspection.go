package repositoryintegrationcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

const RepositorySourceInspectionProtocol = "suma.repository-source-inspection.v1"
const RepositorySourceMaxManifests = 200
const RepositorySourceMaxTreeEntries = 10000
const RepositorySourceMaxFileBytes = 256 << 10
const RepositorySourceMaxContentBytes = 8 << 20
const RepositorySourceMaxTransportBytes = 12 << 20
const RepositorySourceMaxReportBytes = 1 << 20

// Every proof field identifies the original successful apply operation. There
// are no local directories, free-form Git revisions or caller-raised limits.
type RepositorySourceInspectionRequest struct {
	Protocol           string   `json:"protocol"`
	TenantID           string   `json:"tenant_id"`
	ProjectID          string   `json:"project_id"`
	ProviderID         string   `json:"provider_id"`
	TargetID           string   `json:"target_id"`
	TargetAlias        string   `json:"target_alias"`
	TargetRef          string   `json:"target_ref"`
	TargetConfigSHA256 string   `json:"target_config_sha256"`
	OperationID        string   `json:"operation_id"`
	RequestSHA256      string   `json:"request_sha256"`
	ReceiptSHA256      string   `json:"receipt_sha256"`
	ExactCommit        string   `json:"exact_commit"`
	RelativeRoots      []string `json:"relative_roots"`
}
type RepositorySourceInspectionScope struct {
	RelativeRoot string `json:"relative_root"`
	TreeOID      string `json:"tree_oid"`
}
type RepositorySourceManifest struct {
	Path          string `json:"path"`
	BlobOID       string `json:"blob_oid"`
	SHA256        string `json:"sha256"`
	SizeBytes     int64  `json:"size_bytes"`
	ContentBase64 string `json:"content_base64"`
	Format        string `json:"format"`
}
type RepositorySourceInspectionCoverage struct {
	State          string   `json:"state"`
	TreeEntries    int      `json:"tree_entries"`
	ManifestCount  int      `json:"manifest_count"`
	ContentBytes   int64    `json:"content_bytes"`
	SkippedEntries int      `json:"skipped_entries"`
	Reasons        []string `json:"reasons"`
}
type RepositorySourceInspectionLimits struct {
	MaxManifests    int `json:"max_manifests"`
	MaxTreeEntries  int `json:"max_tree_entries"`
	MaxFileBytes    int `json:"max_file_bytes"`
	MaxContentBytes int `json:"max_content_bytes"`
	MaxReportBytes  int `json:"max_report_bytes"`
}
type ProviderSourceInspection struct {
	Protocol                string                             `json:"protocol"`
	InspectionRequestSHA256 string                             `json:"inspection_request_sha256"`
	SnapshotSHA256          string                             `json:"snapshot_sha256"`
	ExactCommit             string                             `json:"exact_commit"`
	TreeOID                 string                             `json:"tree_oid"`
	Scopes                  []RepositorySourceInspectionScope  `json:"scopes"`
	Manifests               []RepositorySourceManifest         `json:"manifests"`
	Coverage                RepositorySourceInspectionCoverage `json:"coverage"`
	Limits                  RepositorySourceInspectionLimits   `json:"limits"`
}

func RepositorySourceLimits() RepositorySourceInspectionLimits {
	return RepositorySourceInspectionLimits{RepositorySourceMaxManifests, RepositorySourceMaxTreeEntries, RepositorySourceMaxFileBytes, RepositorySourceMaxContentBytes, RepositorySourceMaxReportBytes}
}

var sourceIdentity = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// Empty is the sole repository-root spelling. Git paths remain canonical data;
// a source root is never passed as a process working directory or URL.
func RepositorySourceValidPath(value string, allowRoot bool) bool {
	if value == "" {
		return allowRoot
	}
	if len(value) > 1024 || !utf8.ValidString(value) || path.Clean(value) != value || strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\\:*?[]{}\x00") {
		return false
	}
	for _, c := range value {
		if unicode.IsControl(c) {
			return false
		}
	}
	for _, part := range strings.Split(value, "/") {
		if part == "." || part == ".." || strings.EqualFold(part, ".git") {
			return false
		}
	}
	return true
}
func ValidateRepositorySourceInspectionRequest(q RepositorySourceInspectionRequest) error {
	if q.Protocol != RepositorySourceInspectionProtocol || !RepositoryValidOperationID(q.OperationID) || !RepositoryValidRef(q.TargetRef) || !cc.ValidHash(q.TargetConfigSHA256) || !cc.ValidHash(q.RequestSHA256) || !cc.ValidHash(q.ReceiptSHA256) || !cc.ValidCommit(q.ExactCommit) || len(q.RelativeRoots) < 1 || len(q.RelativeRoots) > 16 {
		return fmt.Errorf("invalid repository source inspection")
	}
	for _, id := range []string{q.TenantID, q.ProjectID, q.ProviderID, q.TargetID, q.TargetAlias} {
		if !sourceIdentity.MatchString(id) {
			return fmt.Errorf("invalid source identity")
		}
	}
	for i, root := range q.RelativeRoots {
		if !RepositorySourceValidPath(root, true) || i > 0 && q.RelativeRoots[i-1] >= root {
			return fmt.Errorf("source roots must be canonical, sorted and disjoint")
		}
		for _, previous := range q.RelativeRoots[:i] {
			if previous == "" || strings.HasPrefix(root, previous+"/") {
				return fmt.Errorf("source roots overlap")
			}
		}
	}
	return nil
}
func (q *RepositorySourceInspectionRequest) UnmarshalJSON(raw []byte) error {
	if len(raw) > 32768 {
		return fmt.Errorf("source request exceeds bound")
	}
	keys := []string{"protocol", "tenant_id", "project_id", "provider_id", "target_id", "target_alias", "target_ref", "target_config_sha256", "operation_id", "request_sha256", "receipt_sha256", "exact_commit", "relative_roots"}
	allowed := map[string]bool{}
	for _, key := range keys {
		allowed[key] = true
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return fmt.Errorf("invalid source request shape")
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err = decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || !allowed[key] || seen[key] {
			return fmt.Errorf("noncanonical source request")
		}
		seen[key] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("null source request field")
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') || len(seen) != len(keys) {
		return fmt.Errorf("incomplete source request")
	}
	if _, err = decoder.Token(); err != io.EOF {
		return fmt.Errorf("extra source request")
	}
	type wire RepositorySourceInspectionRequest
	var result wire
	if json.Unmarshal(raw, &result) != nil {
		return fmt.Errorf("invalid source request types")
	}
	if err = ValidateRepositorySourceInspectionRequest(RepositorySourceInspectionRequest(result)); err != nil {
		return err
	}
	*q = RepositorySourceInspectionRequest(result)
	return nil
}
func RepositorySourceInspectionRequestDigest(q RepositorySourceInspectionRequest) (string, error) {
	if err := ValidateRepositorySourceInspectionRequest(q); err != nil {
		return "", err
	}
	return RepositoryCanonicalDigest(q)
}
func RepositorySourceInspectionDigest(s ProviderSourceInspection) (string, error) {
	s.SnapshotSHA256 = ""
	return RepositoryCanonicalDigest(s)
}
func sourceBytesSHA(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func RepositorySourceManifestFormat(name string) string {
	switch path.Base(name) {
	case "package.json":
		return "package-json"
	case "pnpm-workspace.yaml":
		return "pnpm-workspace-yaml"
	case "pnpm-lock.yaml":
		return "pnpm-lock-yaml"
	case "package-lock.json":
		return "npm-lock-json"
	}
	return ""
}

// The control plane and parser validate byte hashes before interpreting any
// untrusted manifest. This does not turn declarations into resolved packages.
func ValidateProviderSourceInspection(q RepositorySourceInspectionRequest, s ProviderSourceInspection) error {
	request, err := RepositorySourceInspectionRequestDigest(q)
	if err != nil {
		return err
	}
	if s.Protocol != RepositorySourceInspectionProtocol || s.InspectionRequestSHA256 != request || s.ExactCommit != q.ExactCommit || !cc.ValidCommit(s.TreeOID) || len(s.TreeOID) != len(q.ExactCommit) || s.Limits != RepositorySourceLimits() || len(s.Scopes) != len(q.RelativeRoots) || s.Manifests == nil || len(s.Manifests) > RepositorySourceMaxManifests || s.Coverage.Reasons == nil || s.Coverage.TreeEntries < 0 || s.Coverage.TreeEntries > RepositorySourceMaxTreeEntries || s.Coverage.SkippedEntries < 0 || s.Coverage.SkippedEntries > s.Coverage.TreeEntries || s.Coverage.ManifestCount != len(s.Manifests) || s.Coverage.State != "complete" && s.Coverage.State != "partial" {
		return fmt.Errorf("invalid repository source snapshot")
	}
	for i, scope := range s.Scopes {
		if scope.RelativeRoot != q.RelativeRoots[i] || !cc.ValidCommit(scope.TreeOID) || len(scope.TreeOID) != len(q.ExactCommit) {
			return fmt.Errorf("source scope differs")
		}
	}
	var size int64
	for i, file := range s.Manifests {
		if !RepositorySourceValidPath(file.Path, false) || !cc.ValidCommit(file.BlobOID) || len(file.BlobOID) != len(q.ExactCommit) || !cc.ValidHash(file.SHA256) || file.Format == "" || RepositorySourceManifestFormat(file.Path) != file.Format || file.SizeBytes < 0 || file.SizeBytes > RepositorySourceMaxFileBytes || i > 0 && s.Manifests[i-1].Path >= file.Path {
			return fmt.Errorf("invalid source manifest")
		}
		inside := false
		for _, root := range q.RelativeRoots {
			if root == "" || strings.HasPrefix(file.Path, root+"/") {
				inside = true
			}
		}
		if !inside {
			return fmt.Errorf("manifest outside fixed source roots")
		}
		decoded, err := base64.StdEncoding.Strict().DecodeString(file.ContentBase64)
		if err != nil || int64(len(decoded)) != file.SizeBytes {
			return fmt.Errorf("invalid source manifest encoding")
		}
		if sourceBytesSHA(decoded) != file.SHA256 {
			return fmt.Errorf("source manifest bytes changed")
		}
		size += file.SizeBytes
	}
	if size > RepositorySourceMaxContentBytes || size != s.Coverage.ContentBytes || s.Coverage.State == "complete" && (s.Coverage.SkippedEntries != 0 || len(s.Coverage.Reasons) != 0) || !sort.StringsAreSorted(s.Coverage.Reasons) {
		return fmt.Errorf("invalid source coverage")
	}
	digest, err := RepositorySourceInspectionDigest(s)
	if err != nil || digest != s.SnapshotSHA256 {
		return fmt.Errorf("source snapshot digest changed")
	}
	raw, err := json.Marshal(s)
	if err != nil || len(raw) > RepositorySourceMaxTransportBytes {
		return fmt.Errorf("source transport bound exceeded")
	}
	return nil
}
