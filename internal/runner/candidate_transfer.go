package runner

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

// candidateTransfer is a durable continuation of a completed process. Replaying
// it may transfer bytes and report that original result, never execute code.
type candidateTransfer struct {
	Assignment Assignment      `json:"assignment"`
	Result     Result          `json:"result"`
	Request    cc.StartRequest `json:"request"`
	BundlePath string          `json:"bundle_path"`
}

func candidateRef(a Assignment) cc.AssignmentRef {
	if a.PoolID == "" {
		return cc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}
	}
	return cc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, TenantID: a.TenantID, ProjectID: a.ProjectID, PoolID: a.PoolID, PoolRunnerID: a.PoolRunnerID, PoolSlotID: a.PoolSlotID, PoolGrantID: a.PoolGrantID, PoolGrantVersion: a.PoolGrantVersion, PoolCredentialID: a.PoolCredentialID, PoolCredentialVersion: a.PoolCredentialVersion, DispatchID: a.DispatchID, DispatchState: a.DispatchState, LeaseUntil: a.LeaseUntil}
}

func candidateOperation(a Assignment, phase string) string {
	return digest([]byte("candidate-v1/" + key(a) + "/" + a.WorkerID + "/" + phase))
}

func (c *Client) candidateRequest(ctx context.Context, method, path string, a Assignment, body io.Reader, length int64, operation string) (*http.Response, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) || strings.TrimSpace(c.Token) == "" {
		return nil, fmt.Errorf("candidate transfer requires an authenticated secure control origin")
	}
	namespace := "runner"
	if c.Pool != nil {
		if c.Pool.Assignment == nil || !samePoolAssignmentIdentity(a, *c.Pool.Assignment) || a.PoolID != c.Pool.PoolID || a.PoolRunnerID != c.Pool.RunnerID || validPoolAssignment(a) != nil {
			return nil, fmt.Errorf("candidate request differs from frozen pool identity")
		}
		namespace = "pool-runner"
	} else if a.PoolID != "" {
		return nil, fmt.Errorf("pool candidate requires a scoped pool client")
	}
	ref := candidateRef(a)
	if !ref.Valid() {
		return nil, fmt.Errorf("candidate assignment identity invalid")
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+"/api/v1/"+namespace+"/candidate-transfers/"+path, body)
	if err != nil {
		return nil, err
	}
	req.ContentLength = length
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Suma-Operation-ID", operation)
	raw, _ := json.Marshal(ref)
	req.Header.Set("X-Suma-Assignment", base64.RawURLEncoding.EncodeToString(raw))
	client := http.Client{Timeout: 30 * time.Second}
	if c.HTTP != nil {
		client = *c.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("candidate bundle request unavailable")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body.Close()
		return nil, &runnerHTTPStatusError{Path: "candidate-transfers/" + path, Status: response.StatusCode}
	}
	return response, nil
}

// Only a single HEAD and the exact administrator base prerequisite are allowed.
// Git remains responsible for checking the pack, objects and prerequisites.
func candidateBundleHeader(path, base, head string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReader(io.LimitReader(f, 16<<10))
	version, err := r.ReadString('\n')
	if err != nil || (version != "# v2 git bundle\n" && version != "# v3 git bundle\n") {
		return fmt.Errorf("unsupported candidate bundle header")
	}
	prerequisites, heads, caps := 0, 0, 0
	for {
		line, e := r.ReadString('\n')
		if e != nil {
			return fmt.Errorf("incomplete candidate bundle header")
		}
		if line == "\n" {
			break
		}
		if strings.HasPrefix(line, "@") {
			if version != "# v3 git bundle\n" || caps != 0 || prerequisites != 0 || heads != 0 || line != "@object-format="+map[bool]string{true: "sha256", false: "sha1"}[len(head) == 64]+"\n" {
				return fmt.Errorf("unsupported candidate bundle capability")
			}
			caps++
			continue
		}
		if strings.HasPrefix(line, "-") {
			id, _, ok := strings.Cut(strings.TrimSuffix(line, "\n"), " ")
			if !ok || id != "-"+base || prerequisites != 0 || heads != 0 {
				return fmt.Errorf("candidate bundle base prerequisite mismatch")
			}
			prerequisites++
			continue
		}
		if line != head+" HEAD\n" || heads != 0 {
			return fmt.Errorf("candidate bundle must advertise only the fixed HEAD")
		}
		heads++
	}
	if prerequisites != 1 || heads != 1 || (len(head) == 64 && (version != "# v3 git bundle\n" || caps != 1)) {
		return fmt.Errorf("candidate bundle lacks fixed head or base")
	}
	return nil
}

type candidateLimitWriter struct {
	file   *os.File
	size   int64
	cancel context.CancelFunc
}

func (w *candidateLimitWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > cc.MaxBundleBytes-w.size {
		w.cancel()
		return 0, fmt.Errorf("candidate bundle exceeds 64 MiB")
	}
	n, e := w.file.Write(p)
	w.size += int64(n)
	return n, e
}

func candidateGit(ctx context.Context, repo string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.hooksPath=" + filepath.Join(repo, ".suma-disabled-hooks"), "-c", "transfer.fsckObjects=true", "-c", "fetch.fsckObjects=true"}, args...)...)
	cmd.Dir = repo
	environment, _, _ := commandEnvironment(CommandSpec{})
	cmd.Env = append(environment, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_NO_REPLACE_OBJECTS=1")
	return cmd
}

func createCandidateBundle(ctx context.Context, w *attemptWorkspace) (string, string, int64, error) {
	path := filepath.Join(w.Directory, "candidate.bundle")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", "", 0, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	limited := &candidateLimitWriter{file: f, cancel: cancel}
	cmd := candidateGit(ctx, w.Source, "bundle", "create", "-", "HEAD", "^"+w.AdminBaseCommit)
	cmd.Stdout = limited
	cmd.Stderr = newOutput(8192, nil)
	err = cmd.Run()
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", "", 0, fmt.Errorf("candidate bundle creation failed: %w", err)
	}
	if err = candidateBundleHeader(path, w.AdminBaseCommit, w.HeadCommit); err != nil {
		return "", "", 0, err
	}
	sha, size, err := candidateFileHash(path)
	return path, sha, size, err
}

func candidateFileHash(path string) (string, int64, error) {
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() < 1 || st.Size() > cc.MaxBundleBytes {
		return "", 0, fmt.Errorf("candidate bundle file missing, linked or oversized")
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil || !strings.EqualFold(filepath.Clean(real), filepath.Clean(path)) {
		return "", 0, fmt.Errorf("candidate bundle path traverses a link")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, cc.MaxBundleBytes+1))
	if err != nil || n != st.Size() {
		return "", 0, fmt.Errorf("candidate bundle changed while hashing")
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func candidateBindingMatches(b cc.Binding, q cc.StartRequest) bool {
	return b.SchemaVersion == 1 && cc.ValidHash(b.BundleSHA256) && cc.ValidHash(b.EvidenceSHA256) && b.TransferID != "" && len(b.TransferID) <= 64 && !strings.ContainsAny(b.TransferID, "/\\\x00\r\n") && b.Source == q.Assignment && b.ProjectID == q.ProjectID && b.BaseCommit == q.BaseCommit && b.HeadCommit == q.HeadCommit && b.EvidenceSHA256 == q.EvidenceSHA256 && b.BundleSHA256 == q.BundleSHA256 && b.SizeBytes == q.SizeBytes && (b.State == "uploading" || b.ValidSealed())
}

func uploadCandidate(ctx context.Context, c *Client, p candidateTransfer) (cc.Binding, error) {
	var b cc.Binding
	sha, size, err := candidateFileHash(p.BundlePath)
	if err != nil || sha != p.Request.BundleSHA256 || size != p.Request.SizeBytes {
		return b, fmt.Errorf("durable candidate bundle changed")
	}
	if err = p.Request.Validate(); err != nil {
		return b, err
	}
	if err = c.post(ctx, "candidate-transfers", p.Request, &b); err != nil {
		return b, err
	}
	if !candidateBindingMatches(b, p.Request) {
		return b, fmt.Errorf("candidate upload binding mismatch")
	}
	if b.ValidSealed() {
		return b, nil
	}
	f, err := os.Open(p.BundlePath)
	if err != nil {
		return b, err
	}
	response, err := c.candidateRequest(ctx, http.MethodPut, url.PathEscape(b.TransferID)+"/bundle?project_id="+url.QueryEscape(c.ProjectID), p.Assignment, f, size, candidateOperation(p.Assignment, "bundle"))
	f.Close()
	if err != nil {
		return b, err
	}
	response.Body.Close()
	err = c.post(ctx, "candidate-transfers/"+url.PathEscape(b.TransferID)+"/seal", cc.CommandRequest{ProjectID: c.ProjectID, Assignment: candidateRef(p.Assignment), OperationID: candidateOperation(p.Assignment, "seal")}, &b)
	if err != nil {
		return b, err
	}
	if !candidateBindingMatches(b, p.Request) || !b.ValidSealed() {
		return b, fmt.Errorf("sealed candidate binding mismatch")
	}
	return b, nil
}

func finishCandidate(ctx context.Context, a Assignment, cfg ExecConfig, w *attemptWorkspace, r *Result) error {
	path, sha, size, err := createCandidateBundle(ctx, w)
	if err != nil {
		return err
	}
	evidence, _ := r.WorkspaceEvidence["evidence_sha256"].(string)
	p := candidateTransfer{Assignment: a, Result: *r, BundlePath: path, Request: cc.StartRequest{ProjectID: cfg.Artifacts.ProjectID, Assignment: candidateRef(a), OperationID: candidateOperation(a, "start"), BaseCommit: w.AdminBaseCommit, HeadCommit: w.HeadCommit, EvidenceSHA256: evidence, BundleSHA256: sha, SizeBytes: size}}
	if p.Result.OperationID == "" {
		p.Result.OperationID = candidateOperation(a, "result")
		r.OperationID = p.Result.OperationID
	}
	if err = p.Request.Validate(); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(p, "", "  ")
	if err = writeEvidence(filepath.Join(w.Directory, "candidate-transfer.json"), raw); err != nil {
		return err
	}
	if cfg.Journal != nil {
		if err = cfg.Journal.checkpointCandidate(p); err != nil {
			return err
		}
		r.candidatePending = true
	}
	b, err := uploadCandidate(ctx, cfg.Artifacts, p)
	if err != nil {
		return err
	}
	r.WorkspaceEvidence["candidate_bundle"] = b
	r.candidatePending = false
	return nil
}

func importCandidate(ctx context.Context, repo string, a Assignment, cfg ExecConfig) (*cc.ImportReceipt, error) {
	if cfg.Artifacts == nil {
		return nil, fmt.Errorf("candidate commit is not available locally; no transfer client is configured")
	}
	var b cc.Binding
	c := cfg.Artifacts
	err := c.post(ctx, "candidate-transfers/resolve", cc.CommandRequest{ProjectID: c.ProjectID, Assignment: candidateRef(a), OperationID: candidateOperation(a, "resolve")}, &b)
	if err != nil {
		return nil, err
	}
	if !b.ValidSealed() || b.ProjectID != c.ProjectID || b.BaseCommit != cfg.BaseCommit || b.HeadCommit != a.CandidateCommit || b.Source.RunID != a.RunID || b.Source.NodeID != a.CandidateFrom || b.Source.Attempt != a.CandidateAttempt || b.Source.Fence != a.CandidateFence || b.EvidenceSHA256 != a.CandidateEvidenceSHA256 || (a.TenantID != "" && b.Source.TenantID != "" && a.TenantID != b.Source.TenantID) {
		return nil, fmt.Errorf("resolved candidate differs from frozen source")
	}
	dir := filepath.Join(repo, ".cache", "runner", "candidate-imports", attemptKey(a))
	if err = safeMkdir(dir); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dir, "bundle-*.tmp")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	defer os.Remove(path)
	response, err := c.candidateRequest(ctx, http.MethodGet, url.PathEscape(b.TransferID)+"/bundle?project_id="+url.QueryEscape(c.ProjectID), a, nil, 0, candidateOperation(a, "download"))
	if err != nil {
		f.Close()
		return nil, err
	}
	n, copyErr := io.Copy(f, io.LimitReader(response.Body, b.SizeBytes+1))
	response.Body.Close()
	syncErr := f.Sync()
	closeErr := f.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || n != b.SizeBytes {
		return nil, fmt.Errorf("candidate download length or persistence mismatch")
	}
	sha, size, err := candidateFileHash(path)
	if err != nil || sha != b.BundleSHA256 || size != b.SizeBytes {
		return nil, fmt.Errorf("candidate download hash mismatch")
	}
	if err = candidateBundleHeader(path, b.BaseCommit, b.HeadCommit); err != nil {
		return nil, err
	}
	verify := candidateGit(ctx, repo, "bundle", "verify", path)
	out := newOutput(8192, nil)
	verify.Stdout = out
	verify.Stderr = out
	if verify.Run() != nil {
		return nil, fmt.Errorf("candidate Git bundle verification failed")
	}
	pack, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer pack.Close()
	reader := bufio.NewReader(pack)
	for {
		line, e := reader.ReadString('\n')
		if e != nil {
			return nil, fmt.Errorf("candidate bundle header changed")
		}
		if line == "\n" {
			break
		}
	}
	index := candidateGit(ctx, repo, "index-pack", "--stdin", "--fix-thin", "--strict", "--max-input-size=67108864")
	index.Stdin = reader
	index.Stdout = out
	index.Stderr = out
	if index.Run() != nil {
		return nil, fmt.Errorf("candidate pack strict verification/import failed")
	}
	resolved, err := gitText(ctx, repo, "rev-parse", "--verify", a.CandidateCommit+"^{commit}")
	if err != nil || resolved != a.CandidateCommit {
		return nil, fmt.Errorf("import did not provide fixed candidate commit")
	}
	return &cc.ImportReceipt{Binding: b, Imported: true, TargetWorkerID: a.WorkerID}, nil
}
