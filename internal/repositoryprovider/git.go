package repositoryprovider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	rc "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, fmt.Errorf("bounded Git output exceeded")
	}
	return b.Buffer.Write(p)
}
func (s *Server) git(ctx context.Context, t *target, input io.Reader, limit int, args ...string) ([]byte, error) {
	prefix := []string{"-c", "core.hooksPath=" + filepath.Join(s.cfg.StateDir, "disabled-hooks"), "-c", "core.quotePath=false", "-c", "transfer.fsckObjects=true", "-c", "fetch.fsckObjects=true", "-c", "protocol.allow=never"}
	c := exec.CommandContext(ctx, s.cfg.GitExecutable, append(prefix, args...)...)
	c.Dir = t.config.BareRepository
	c.Stdin = input
	for _, name := range []string{"PATH", "SystemRoot", "WINDIR", "TEMP", "TMP"} {
		if v := os.Getenv(name); v != "" {
			c.Env = append(c.Env, name+"="+v)
		}
	}
	c.Env = append(c.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_ATTR_NOSYSTEM=1", "GIT_NO_REPLACE_OBJECTS=1", "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	b := &limitedBuffer{limit: limit}
	c.Stdout = b
	c.Stderr = &limitedBuffer{limit: 8192}
	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("repository Git operation failed")
	}
	return b.Bytes(), nil
}
func (s *Server) validateTarget(ctx context.Context, t *target, initial bool) error {
	st, e := os.Stat(t.config.BareRepository)
	if e != nil || !os.SameFile(st, t.identity) {
		return fmt.Errorf("repository identity changed")
	}
	path, e := safeDirectory(t.config.BareRepository, false)
	if e != nil || path != t.config.BareRepository {
		return fmt.Errorf("repository path changed")
	}
	count := 0
	if err := fs.WalkDir(t.root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		count++
		if count > 20000 || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("repository contains links or exceeds supported metadata bound")
		}
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("repository contains nonregular files")
			}
			f, err := t.root.Open(path)
			if err != nil {
				return err
			}
			opened, err := f.Stat()
			linked := !singleLink(f)
			f.Close()
			if err != nil || linked || !os.SameFile(info, opened) {
				return fmt.Errorf("repository file identity is linked or changed")
			}
		}
		return nil
	}); err != nil {
		return err
	}
	for _, p := range []string{"shallow", "objects/info/alternates", "objects/info/http-alternates", "info/grafts"} {
		if _, err := t.root.Lstat(p); !os.IsNotExist(err) {
			return fmt.Errorf("repository shallow, alternates or grafts are forbidden")
		}
	}
	if entries, e := fs.ReadDir(t.root.FS(), "refs/replace"); e == nil && len(entries) > 0 {
		return fmt.Errorf("repository replacement refs are forbidden")
	}
	for _, p := range []string{"config", "objects", "refs"} {
		st, e := t.root.Lstat(p)
		if e != nil || st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("repository control path linked or unavailable")
		}
	}
	bare, e := s.git(ctx, t, nil, 4096, "rev-parse", "--is-bare-repository")
	if e != nil || strings.TrimSpace(string(bare)) != "true" {
		return fmt.Errorf("target must be an administrator bare repository")
	}
	config, e := s.git(ctx, t, nil, 65536, "config", "--local", "--no-includes", "--list")
	if e != nil {
		return e
	}
	for _, line := range strings.Split(strings.ToLower(string(config)), "\n") {
		if strings.HasPrefix(line, "include.") || strings.HasPrefix(line, "includeif.") || strings.HasPrefix(line, "core.worktree=") || strings.HasPrefix(line, "extensions.refstorage=") {
			return fmt.Errorf("unsupported repository configuration")
		}
	}
	if _, e = s.git(ctx, t, nil, 4096, "symbolic-ref", "-q", t.config.Ref); e == nil {
		return fmt.Errorf("symbolic target refs are forbidden")
	}
	format, e := s.git(ctx, t, nil, 4096, "rev-parse", "--show-object-format")
	if e != nil {
		return e
	}
	f := strings.TrimSpace(string(format))
	if f != "sha1" && f != "sha256" {
		return fmt.Errorf("unsupported Git object format")
	}
	raw, e := t.root.ReadFile("config")
	if e != nil {
		return e
	}
	git, e := os.ReadFile(s.cfg.GitExecutable)
	if e != nil {
		return e
	}
	if initial {
		t.format = f
	} else if f != t.format || hashBytes(raw) != t.rawConfigSHA || hashBytes(git) != t.gitSHA {
		return fmt.Errorf("repository or Git configuration drift")
	}
	return nil
}
func fenceRef(t *target) string {
	return "refs/suma/repository/fences/" + hashBytes([]byte(t.config.Ref))
}
func receiptRef(id string) string { return "refs/suma/repository/receipts/" + hashBytes([]byte(id)) }
func (s *Server) ref(ctx context.Context, t *target, ref string) (string, error) {
	b, e := s.git(ctx, t, nil, 4096, "show-ref", "--verify", "--hash", ref)
	if e != nil {
		return "", e
	}
	oid := strings.TrimSpace(string(b))
	if !cc.ValidCommit(oid) {
		return "", fmt.Errorf("invalid Git ref object")
	}
	return oid, nil
}
func (s *Server) fence(ctx context.Context, t *target) (string, int64, error) {
	// --verify --quiet distinguishes an absent ref (exit 1) from a corrupt repo.
	oid, e := s.ref(ctx, t, fenceRef(t))
	if e != nil {
		b, e2 := s.git(ctx, t, nil, 65536, "for-each-ref", "--format=%(refname)", "refs/suma/repository/fences/")
		if e2 != nil {
			return "", 0, e2
		}
		for _, line := range strings.Split(string(b), "\n") {
			if line == fenceRef(t) {
				return "", 0, e
			}
		}
		return "", 0, nil
	}
	b, e := s.git(ctx, t, nil, 65536, "cat-file", "blob", oid)
	if e != nil {
		return "", 0, e
	}
	var receipt rc.RepositoryReceipt
	if json.Unmarshal(b, &receipt) != nil || receipt.Protocol != rc.RepositoryProtocol || receipt.TargetRef != t.config.Ref || receipt.TargetConfigSHA256 != t.configSHA || receipt.Generation < 1 {
		return "", 0, fmt.Errorf("invalid repository fence receipt")
	}
	return oid, receipt.Generation, nil
}
func bundleHeader(data []byte, base, head string) (int, error) {
	r := bufio.NewReader(bytes.NewReader(data))
	version, e := r.ReadString('\n')
	if e != nil || (version != "# v2 git bundle\n" && version != "# v3 git bundle\n") {
		return 0, fmt.Errorf("unsupported candidate bundle")
	}
	size := len(version)
	prereq, heads, caps := 0, 0, 0
	for size <= 16<<10 {
		line, e := r.ReadString('\n')
		if e != nil {
			return 0, fmt.Errorf("incomplete bundle header")
		}
		size += len(line)
		if line == "\n" {
			if prereq != 1 || heads != 1 || (len(head) == 64 && caps != 1) {
				return 0, fmt.Errorf("candidate bundle prerequisites differ")
			}
			return size, nil
		}
		if strings.HasPrefix(line, "@") {
			format := "sha1"
			if len(head) == 64 {
				format = "sha256"
			}
			if version != "# v3 git bundle\n" || caps != 0 || prereq != 0 || heads != 0 || line != "@object-format="+format+"\n" {
				return 0, fmt.Errorf("unsupported bundle capability")
			}
			caps++
			continue
		}
		if strings.HasPrefix(line, "-") {
			id, _, ok := strings.Cut(strings.TrimSuffix(line, "\n"), " ")
			if !ok || id != "-"+base || prereq != 0 || heads != 0 {
				return 0, fmt.Errorf("bundle base differs")
			}
			prereq++
			continue
		}
		if line != head+" HEAD\n" || heads != 0 {
			return 0, fmt.Errorf("bundle head differs")
		}
		heads++
	}
	return 0, fmt.Errorf("oversized bundle header")
}
func (s *Server) importBundle(ctx context.Context, t *target, op *operation) error {
	e := op.Request.RepositoryExecution
	data, err := s.bundle(ctx, op)
	if err != nil {
		return err
	}
	if int64(len(data)) != e.Source.SizeBytes || hashBytes(data) != e.Source.BundleSHA256 {
		return fmt.Errorf("candidate bundle bytes differ")
	}
	offset, err := bundleHeader(data, e.Source.BundleBase(), e.Source.HeadCommit)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.cfg.StateDir, "bundle-*.tmp")
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if _, err = s.git(ctx, t, nil, 8192, "bundle", "verify", path); err != nil {
		return fmt.Errorf("candidate bundle verification failed")
	}
	if _, err = s.git(ctx, t, bytes.NewReader(data[offset:]), 8192, "index-pack", "--stdin", "--fix-thin", "--strict", "--max-input-size=67108864"); err != nil {
		return fmt.Errorf("candidate pack verification failed")
	}
	resolved, err := s.git(ctx, t, nil, 4096, "rev-parse", "--verify", e.Source.HeadCommit+"^{commit}")
	if err != nil || strings.TrimSpace(string(resolved)) != e.Source.HeadCommit {
		return fmt.Errorf("candidate commit missing")
	}
	for _, relation := range [][2]string{{e.Source.BundleBase(), e.Source.BaseCommit}, {e.Source.BaseCommit, e.Source.HeadCommit}} {
		if _, err = s.git(ctx, t, nil, 4096, "merge-base", "--is-ancestor", relation[0], relation[1]); err != nil {
			return fmt.Errorf("candidate selected base or head is outside bundle anchor lineage")
		}
	}
	return nil
}
func (s *Server) preview(ctx context.Context, t *target, e rc.RepositoryExecution) (*rc.RepositoryPreview, error) {
	head, err := s.ref(ctx, t, t.config.Ref)
	if err != nil || head != e.ExpectedTargetHead {
		return nil, fmt.Errorf("target changed before preparation")
	}
	fence, generation, err := s.fence(ctx, t)
	if err != nil || fence != e.ExpectedFenceOID || generation != e.ExpectedGitGeneration {
		return nil, fmt.Errorf("target fence changed")
	}
	if _, err = s.git(ctx, t, nil, 4096, "merge-base", "--is-ancestor", head, e.Source.HeadCommit); err != nil {
		return nil, fmt.Errorf("candidate is not a fast-forward of target")
	}
	raw, err := s.git(ctx, t, nil, 256<<10, "diff", "--raw", "-z", "--no-abbrev", "--no-renames", "--no-ext-diff", "--no-textconv", "--ignore-submodules=none", head, e.Source.HeadCommit, "--")
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(raw, []byte{0})
	files := []rc.RepositoryFileChange{}
	for i := 0; i+1 < len(parts) && len(parts[i]) > 0; i += 2 {
		fields := strings.Fields(string(parts[i]))
		if len(fields) == 5 && (fields[0] == ":160000" || fields[1] == "160000") {
			return nil, fmt.Errorf("submodule candidate review is unsupported")
		}
		path := string(parts[i+1])
		if len(fields) != 5 || len(path) > 4096 || !utf8.ValidString(path) || strings.ContainsAny(path, "\x00\r\n") {
			return nil, fmt.Errorf("unsupported candidate file change")
		}
		files = append(files, rc.RepositoryFileChange{Path: path, Status: fields[4], OldOID: fields[2], NewOID: fields[3]})
		if len(files) > rc.RepositoryMaxFiles {
			return nil, fmt.Errorf("candidate has more than 1000 changed files")
		}
	}
	stats, err := s.git(ctx, t, nil, 256<<10, "diff", "--numstat", "-z", "--no-renames", "--no-ext-diff", "--no-textconv", "--ignore-submodules=none", head, e.Source.HeadCommit, "--")
	if err != nil {
		return nil, err
	}
	for _, row := range bytes.Split(stats, []byte{0}) {
		if bytes.HasPrefix(row, []byte("-\t-\t")) {
			return nil, fmt.Errorf("binary candidate review is unsupported")
		}
	}
	diff, err := s.git(ctx, t, nil, rc.RepositoryMaxDiffBytes, "diff", "--no-renames", "--no-ext-diff", "--no-textconv", "--ignore-submodules=none", "--no-color", "--unified=3", head, e.Source.HeadCommit, "--")
	if err != nil || !utf8.Valid(diff) {
		return nil, fmt.Errorf("candidate diff unavailable or exceeds 512 KiB")
	}
	p := &rc.RepositoryPreview{TargetHead: head, CandidateHead: e.Source.HeadCommit, Files: files, Diff: string(diff), DiffSHA256: hashBytes(diff), SourceBundleSHA256: e.Source.BundleSHA256, ConfigSHA256: e.TargetConfigSHA256, FastForward: true}
	p.PreparedSHA256, _ = rc.RepositoryPreviewDigest(*p)
	encoded, err := json.Marshal(p)
	if err != nil || len(encoded) > 900<<10 {
		return nil, fmt.Errorf("candidate preview exceeds encoded callback bound")
	}
	return p, nil
}
func (s *Server) inspect(ctx context.Context, t *target, op operation) (rc.RepositoryObservation, error) {
	o := op.Observation
	e := op.Request.RepositoryExecution
	oid, err := s.ref(ctx, t, receiptRef(e.OperationID))
	if err != nil {
		if e.Phase == "apply" {
			if o.State == "applied" || o.State == "not_applied" {
				return o, fmt.Errorf("repository receipt reference is missing")
			}
			if o.State != "unknown" {
				o.Version++
			}
			o.State = "unknown"
			o.Receipt = nil
			o.ReceiptOID = ""
			o.ReceiptSHA256 = ""
			o.Reason = "repository receipt missing; original operation requires reconciliation"
			o.ObservationSHA256, _ = rc.RepositoryObservationDigest(o)
		}
		return o, nil
	}
	raw, err := s.git(ctx, t, nil, 65536, "cat-file", "blob", oid)
	if err != nil {
		return o, err
	}
	var receipt rc.RepositoryReceipt
	if json.Unmarshal(raw, &receipt) != nil {
		return o, fmt.Errorf("invalid Git receipt JSON")
	}
	if err = rc.RepositoryValidateReceipt(receipt, e, o.RequestSHA256); err != nil {
		return o, err
	}
	expectedOID, _ := rc.RepositoryReceiptOID(receipt, t.format)
	if expectedOID != oid {
		return o, fmt.Errorf("Git receipt object differs")
	}
	o.State = receipt.State
	o.Receipt = &receipt
	o.ReceiptOID = oid
	o.ReceiptSHA256, _ = rc.RepositoryReceiptDigest(receipt)
	o.EffectGrantID = receipt.EffectGrantID
	o.Reason = ""
	if op.Observation.ReceiptOID != oid {
		o.Version++
		// A read-only recovered proof has a stable version/hash until explicitly
		// persisted. Repeated GETs must not invent different same-version facts.
		o.ObservedAt = op.Observation.ObservedAt
	}
	o.ObservationSHA256, _ = rc.RepositoryObservationDigest(o)
	return o, nil
}
func (s *Server) commitGit(ctx context.Context, t *target, op *operation, negative bool, grantID string) error {
	e := op.Request.RepositoryExecution
	state, result := "applied", e.Source.HeadCommit
	if negative {
		state, result = "not_applied", ""
	}
	receipt := rc.RepositoryReceipt{Protocol: rc.RepositoryProtocol, OperationID: e.OperationID, RequestSHA256: op.Observation.RequestSHA256, TargetID: e.TargetID, TargetAlias: e.TargetAlias, TargetRef: e.TargetRef, TargetConfigSHA256: e.TargetConfigSHA256, BeforeCommit: e.ExpectedTargetHead, ResultCommit: result, SourceBundleSHA256: e.Source.BundleSHA256, PreparedSHA256: e.PreparedSHA256, Generation: e.Generation, EffectGrantID: grantID, State: state}
	raw, _ := json.Marshal(receipt)
	oidRaw, err := s.git(ctx, t, bytes.NewReader(raw), 4096, "hash-object", "-w", "--stdin")
	if err != nil {
		return err
	}
	oid := strings.TrimSpace(string(oidRaw))
	expected, _ := rc.RepositoryReceiptOID(receipt, t.format)
	if oid != expected {
		return fmt.Errorf("Git receipt object hash differs")
	}
	zero := strings.Repeat("0", len(e.ExpectedTargetHead))
	oldFence := e.ExpectedFenceOID
	if oldFence == "" {
		oldFence = zero
	}
	var tx strings.Builder
	tx.WriteString("start\n")
	if !negative {
		fmt.Fprintf(&tx, "update %s %s %s\n", e.TargetRef, e.Source.HeadCommit, e.ExpectedTargetHead)
	}
	fmt.Fprintf(&tx, "create %s %s\nupdate %s %s %s\nprepare\ncommit\n", receiptRef(e.OperationID), oid, fenceRef(t), oid, oldFence)
	_, err = s.git(ctx, t, strings.NewReader(tx.String()), 8192, "update-ref", "--stdin")
	if s.afterGit != nil {
		s.afterGit()
	}
	if err != nil {
		return err
	}
	observed, err := s.inspect(ctx, t, *op)
	if err != nil || observed.ReceiptOID != oid {
		return fmt.Errorf("Git effect receipt inspection failed")
	}
	op.Observation = observed
	return nil
}
