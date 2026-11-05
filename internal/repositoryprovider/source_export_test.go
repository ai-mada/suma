package repositoryprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func TestRepositorySourceExportExactGitBundle(t *testing.T) {
	f := newGitFixture(t)
	const baseRef, sourceRef = "refs/heads/delivery-base", "refs/heads/delivery-candidate"
	git := func(repo, input string, args ...string) (string, error) {
		t.Helper()
		cmd := exec.Command(f.cfg.GitExecutable, args...)
		cmd.Dir = repo
		cmd.Stdin = strings.NewReader(input)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
			"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
		raw, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(raw)), err
	}
	mustGit := func(repo, input string, args ...string) string {
		t.Helper()
		out, err := git(repo, input, args...)
		if err != nil {
			t.Fatalf("fixture Git %v: %v: %s", args, err, out)
		}
		return out
	}
	// The registered source and baseline are two refs in the same bare
	// repository. Its HEAD deliberately points at the baseline; the independent
	// verifier also begins with the baseline only.
	mustGit(f.source, "", "update-ref", baseRef, f.base)
	mustGit(f.source, "", "update-ref", sourceRef, f.head)
	mustGit(f.source, "", "symbolic-ref", "HEAD", baseRef)
	consumerDir := t.TempDir()
	consumer := filepath.Join(consumerDir, "independent")
	mustGit(consumerDir, "", "clone", "--no-hardlinks", f.target, consumer)
	if _, err := git(consumer, "", "cat-file", "-e", f.head+"^{commit}"); err == nil {
		t.Fatal("independent verifier already has the candidate commit")
	}
	if err := f.s.Close(); err != nil {
		t.Fatal(err)
	}
	f.s = nil
	f.cfg.Targets = map[string]TargetConfig{
		"delivery": {BareRepository: f.source, Ref: sourceRef, DeliveryBaseAlias: "baseline"},
		"baseline": {BareRepository: f.source, Ref: baseRef},
	}
	const exportEnv = "REPO_SOURCE_EXPORT_TEST"
	exportToken := strings.Repeat("s", 48)
	t.Setenv(exportEnv, exportToken)
	f.cfg.SourceExportTokenEnv = exportEnv
	const inspectEnv = "REPO_SOURCE_INSPECT_FOR_EXPORT_TEST"
	inspectToken := strings.Repeat("i", 48)
	t.Setenv(inspectEnv, inspectToken)
	f.cfg.SourceReadTokenEnv = inspectEnv
	var err error
	f.s, err = New(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := f.s.Target(context.Background(), "delivery")
	if err != nil || observed.HeadCommit != f.head {
		t.Fatalf("fixture source unavailable: %+v, %v", observed, err)
	}
	request := ri.RepositorySourceExportRequest{
		Protocol: ri.RepositorySourceExportProtocol, TenantID: f.cfg.TenantID,
		ProjectID: f.cfg.ProjectID, ProviderID: f.cfg.ProviderID, TargetAlias: observed.Alias,
		TargetRef: observed.Ref, TargetConfigSHA256: observed.ConfigSHA256, ExpectedHead: f.head,
		ExpectedFenceOID: observed.FenceOID, ExpectedGeneration: observed.Generation, ExpectedBase: f.base,
	}
	call := func(token string, q ri.RepositorySourceExportRequest) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(q)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "/source-exports", bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		f.s.Handler().ServeHTTP(w, r)
		return w
	}
	assertStatus := func(token string, q ri.RepositorySourceExportRequest, code int) {
		t.Helper()
		w := call(token, q)
		if w.Code != code || w.Header().Get("X-Suma-Bundle-SHA256") != "" {
			t.Fatalf("expected export rejection %d, got %d: %s", code, w.Code, w.Body)
		}
	}
	for _, credential := range []string{"", os.Getenv(f.cfg.ControlTokenEnv), os.Getenv(f.cfg.RunnerTokenEnv), os.Getenv(f.cfg.ReconcileTokenEnv), inspectToken} {
		assertStatus(credential, request, http.StatusForbidden)
	}
	foreign := request
	foreign.ProjectID = "foreign"
	assertStatus(exportToken, foreign, http.StatusForbidden)
	func() {
		f.s.exportMu.Lock()
		defer f.s.exportMu.Unlock()
		assertStatus(exportToken, request, http.StatusTooManyRequests)
	}()

	state := func() string {
		t.Helper()
		return mustGit(f.source, "", "for-each-ref", "--format=%(refname) %(objectname)") + "\n" +
			mustGit(f.source, "", "symbolic-ref", "HEAD") + "\n" + mustGit(f.source, "", "count-objects", "-v")
	}
	before := state()
	response := call(exportToken, request)
	if response.Code != http.StatusOK {
		t.Fatalf("exact source export failed: %d %s", response.Code, response.Body)
	}
	bundle := response.Body.Bytes()
	requestHash, err := ri.RepositoryCanonicalDigest(request)
	if err != nil || response.Header().Get("Content-Type") != "application/x-git-bundle" || response.Header().Get("Cache-Control") != "no-store" ||
		response.Header().Get("Content-Length") != strconv.Itoa(len(bundle)) ||
		response.Header().Get("X-Suma-Bundle-SHA256") != hashBytes(bundle) ||
		response.Header().Get("X-Suma-Source-Request-SHA256") != requestHash {
		t.Fatal("export headers do not match the exact request and bytes")
	}
	header, pack, ok := bytes.Cut(bundle, []byte("\n\n"))
	if !ok || !bytes.HasPrefix(pack, []byte("PACK")) {
		t.Fatal("bundle did not contain a delimited Git pack")
	}
	var prerequisites []string
	for _, line := range strings.Split(string(header), "\n") {
		if strings.HasPrefix(line, "-") {
			oid, _, _ := strings.Cut(line[1:], " ")
			prerequisites = append(prerequisites, oid)
		}
	}
	if len(prerequisites) != 1 || prerequisites[0] != f.base {
		t.Fatalf("bundle prerequisite differs from the fixed baseline: %v", prerequisites)
	}
	bundleFile := filepath.Join(consumerDir, "candidate.bundle")
	if err = os.WriteFile(bundleFile, bundle, 0600); err != nil {
		t.Fatal(err)
	}
	mustGit(consumer, "", "bundle", "verify", bundleFile)
	if heads := mustGit(consumer, "", "bundle", "list-heads", bundleFile); heads != f.head+" HEAD" {
		t.Fatalf("bundle carries unexpected refs: %s", heads)
	}
	mustGit(consumer, "", "bundle", "unbundle", bundleFile)
	mustGit(consumer, "", "checkout", "--detach", f.head)
	content, err := os.ReadFile(filepath.Join(consumer, "file.txt"))
	if err != nil || strings.TrimSpace(string(content)) != "candidate" || mustGit(consumer, "", "rev-parse", "HEAD") != f.head {
		t.Fatalf("independent repository did not receive the exact candidate: %v", err)
	}
	mustGit(consumer, "", "merge-base", "--is-ancestor", f.base, f.head)
	if state() != before {
		t.Fatal("source export changed Git refs or object store")
	}

	mustGit(f.source, "", "update-ref", sourceRef, f.base, f.head)
	assertStatus(exportToken, request, http.StatusConflict)
	mustGit(f.source, "", "update-ref", sourceRef, f.head, f.base)
	mustGit(f.source, "", "update-ref", baseRef, f.head, f.base)
	assertStatus(exportToken, request, http.StatusConflict)
	mustGit(f.source, "", "update-ref", baseRef, f.base, f.head)
	baseTree := mustGit(f.source, "", "rev-parse", f.base+"^{tree}")
	diverged := mustGit(f.source, "unrelated baseline\n", "commit-tree", baseTree)
	mustGit(f.source, "", "update-ref", baseRef, diverged, f.base)
	wrongLineage := request
	wrongLineage.ExpectedBase = diverged
	assertStatus(exportToken, wrongLineage, http.StatusConflict)
	var operations int
	if err := f.s.db.QueryRow(`SELECT COUNT(*) FROM operations`).Scan(&operations); err != nil || operations != 0 {
		t.Fatalf("source export added an integration operation: %d, %v", operations, err)
	}
}
