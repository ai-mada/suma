package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

// Code inputs have their own current-consumer authorization. Knowing a sealed
// transfer ID never relaxes the legacy same-run candidate resolver.
func (c *Client) codeInputBundle(ctx context.Context, a Assignment, kind, bindingID string, b cc.Binding, path string) error {
	if c == nil || !b.ValidSealed() || b.ProjectID != c.ProjectID {
		return fmt.Errorf("fixed code input client or binding is invalid")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) || strings.TrimSpace(c.Token) == "" {
		return fmt.Errorf("code input requires an authenticated secure control origin")
	}
	namespace := "runner"
	if c.Pool != nil {
		if c.Pool.Assignment == nil || !samePoolAssignmentIdentity(a, *c.Pool.Assignment) || a.PoolID != c.Pool.PoolID || a.PoolRunnerID != c.Pool.RunnerID || validPoolAssignment(a) != nil {
			return fmt.Errorf("code input differs from current pool assignment")
		}
		namespace = "pool-runner"
	} else if a.PoolID != "" {
		return fmt.Errorf("pool code input requires a scoped pool client")
	}
	q := ccc.CodeInputRequest{ProjectID: c.ProjectID, Assignment: candidateRef(a), InputKind: kind, BindingID: bindingID, TransferID: b.TransferID}
	if err = ccc.ValidateCodeInputRequest(q); err != nil {
		return err
	}
	raw, err := json.Marshal(q)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/api/v1/"+namespace+"/code-inputs/bundle", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.Token)
	request.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 30 * time.Second}
	if c.HTTP != nil {
		client = *c.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("authorized code input request unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return &runnerHTTPStatusError{Path: "code-inputs/bundle", Status: response.StatusCode}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(f, io.LimitReader(response.Body, b.SizeBytes+1))
	syncErr := f.Sync()
	closeErr := f.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || n != b.SizeBytes {
		return fmt.Errorf("code input download length or persistence mismatch")
	}
	sha, size, err := candidateFileHash(path)
	if err != nil || sha != b.BundleSHA256 || size != b.SizeBytes {
		return fmt.Errorf("code input bytes differ from frozen binding")
	}
	return nil
}

// Strict object import is shared; only its callers decide which authenticated
// binding may be read. A pack's anchor and the selected source base are distinct.
func importCandidateFile(ctx context.Context, repo, path string, b cc.Binding) error {
	sha, size, err := candidateFileHash(path)
	if err != nil || sha != b.BundleSHA256 || size != b.SizeBytes {
		return fmt.Errorf("candidate download hash mismatch")
	}
	if err = candidateBundleHeader(path, b.BundleBase(), b.HeadCommit); err != nil {
		return err
	}
	verify := nativeGitCommand(ctx, repo, "bundle", "verify", path)
	out := newOutput(8192, nil)
	verify.Stdout = out
	verify.Stderr = out
	if verify.Run() != nil {
		return fmt.Errorf("candidate Git bundle verification failed")
	}
	pack, err := os.Open(path)
	if err != nil {
		return err
	}
	defer pack.Close()
	reader := bufio.NewReader(pack)
	for {
		line, e := reader.ReadString('\n')
		if e != nil {
			return fmt.Errorf("candidate bundle header changed")
		}
		if line == "\n" {
			break
		}
	}
	index := nativeGitCommand(ctx, repo, "index-pack", "--stdin", "--fix-thin", "--strict", "--max-input-size=67108864")
	index.Stdin = reader
	index.Stdout = out
	index.Stderr = out
	if index.Run() != nil {
		return fmt.Errorf("candidate pack strict verification/import failed")
	}
	for _, relation := range [][2]string{{b.BundleBase(), b.BaseCommit}, {b.BaseCommit, b.HeadCommit}} {
		cmd := nativeGitCommand(ctx, repo, "merge-base", "--is-ancestor", relation[0], relation[1])
		cmd.Stdout = out
		cmd.Stderr = out
		if cmd.Run() != nil {
			return fmt.Errorf("candidate selected base or head is outside bundle anchor lineage")
		}
	}
	resolved, err := gitText(context.WithValue(ctx, nativeGitScope{}, true), repo, "rev-parse", "--verify", b.HeadCommit+"^{commit}")
	if err != nil || resolved != b.HeadCommit {
		return fmt.Errorf("import did not provide fixed candidate commit")
	}
	return nil
}

func importCodeBaseline(ctx context.Context, repo string, a Assignment, cfg ExecConfig) (string, *cc.ImportReceipt, error) {
	binding := a.CodeBaseline
	if cfg.Artifacts == nil || binding == nil || ccc.ValidateCodeBaselineBinding(*binding) != nil || a.CandidateComposition != nil || hasCandidate(a) || a.Type != "agent" || binding.ProjectID != cfg.Artifacts.ProjectID || binding.Source.BundleBase() != cfg.BaseCommit {
		return "", nil, fmt.Errorf("development requires a valid dedicated code baseline binding")
	}
	dir := filepath.Join(repo, ".cache", "runner", "code-inputs", attemptKey(a))
	if err := safeMkdir(dir); err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, "baseline.bundle")
	// Even a locally present commit requires this fresh source/current-lease read.
	if err := cfg.Artifacts.codeInputBundle(ctx, a, "code_baseline", binding.ID, binding.Source, path); err != nil {
		return "", nil, err
	}
	if err := importCandidateFile(ctx, repo, path, binding.Source); err != nil {
		return "", nil, err
	}
	return binding.Source.HeadCommit, &cc.ImportReceipt{Binding: binding.Source, Imported: true, TargetWorkerID: a.WorkerID}, nil
}
