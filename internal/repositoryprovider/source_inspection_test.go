package repositoryprovider

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

// Reuse the real atomic integration fixture. Only its source commit is expanded
// to a two-package workspace; original prepare/apply and Git receipts run fully.
func TestRepositorySourceInspectionAppliedWorkspaceAndReadAuthority(t *testing.T) {
	f := newGitFixture(t)
	git := func(input string, args ...string) string {
		t.Helper()
		cmd := exec.Command(f.cfg.GitExecutable, args...)
		cmd.Dir = f.source
		cmd.Stdin = strings.NewReader(input)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=source@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=source@example.invalid", "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
		raw, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("owned source fixture Git failed: %v: %s", err, raw)
		}
		return strings.TrimSpace(string(raw))
	}
	blob := func(content string) string { return git(content, "hash-object", "-w", "--stdin") }
	a := blob(`{"name":"@fixture/a","version":"1.0.0","dependencies":{"@fixture/b":"workspace:^2.0.0"}}`)
	b := blob(`{"name":"@fixture/b","version":"2.1.0"}`)
	aTree := git("100644 blob "+a+"\tpackage.json\n", "mktree")
	bTree := git("100644 blob "+b+"\tpackage.json\n", "mktree")
	packages := git("040000 tree "+aTree+"\ta\n040000 tree "+bTree+"\tb\n", "mktree")
	rootPkg := blob(`{"name":"fixture-root","version":"1.0.0","private":true,"workspaces":["packages/*"]}`)
	workspace := blob("packages:\n  - 'packages/*'\n")
	candidate := blob("candidate\n")
	tree := git("100644 blob "+candidate+"\tfile.txt\n100644 blob "+rootPkg+"\tpackage.json\n040000 tree "+packages+"\tpackages\n100644 blob "+workspace+"\tpnpm-workspace.yaml\n", "mktree")
	f.head = git("owned workspace source\n", "commit-tree", tree, "-p", f.base)
	git("", "update-ref", "refs/heads/main", f.head)
	bundle := filepath.Join(t.TempDir(), "workspace.bundle")
	git("", "bundle", "create", bundle, "HEAD", "^"+f.base)
	var err error
	f.bundle, err = os.ReadFile(bundle)
	if err != nil {
		t.Fatal(err)
	}
	prepared := prepareFixture(t, f)
	apply := f.request(t, "apply", "apply-source-123", prepared)
	code, proof := providerCall(t, f.s, "POST", "/operations", apply, false)
	if code != 200 || proof.State != "applied" || proof.Receipt == nil {
		t.Fatal("actual source integration failed", code, proof.State)
	}
	q := ri.RepositorySourceInspectionRequest{Protocol: ri.RepositorySourceInspectionProtocol, TenantID: f.cfg.TenantID, ProjectID: f.cfg.ProjectID, ProviderID: f.cfg.ProviderID, TargetID: apply.TargetID, TargetAlias: apply.TargetAlias, TargetRef: apply.TargetRef, TargetConfigSHA256: apply.TargetConfigSHA256, OperationID: apply.OperationID, RequestSHA256: proof.RequestSHA256, ReceiptSHA256: proof.ReceiptSHA256, ExactCommit: proof.Receipt.ResultCommit, RelativeRoots: []string{""}}
	call := func(token string, q ri.RepositorySourceInspectionRequest) (int, ri.ProviderSourceInspection) {
		t.Helper()
		raw, _ := json.Marshal(q)
		request := httptest.NewRequest(http.MethodPost, "/source-inspections", bytes.NewReader(raw))
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		f.s.Handler().ServeHTTP(response, request)
		var out ri.ProviderSourceInspection
		if response.Code == 200 {
			if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
		}
		return response.Code, out
	}
	for _, token := range []string{strings.Repeat("r", 32), strings.Repeat("x", 32), strings.Repeat("s", 32)} {
		if status, _ := call(token, q); status != 403 {
			t.Fatal("source content capability was not opt-in", status)
		}
	}
	if err = f.s.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REPO_SOURCE_TEST", strings.Repeat("s", 32))
	f.cfg.SourceReadTokenEnv = "REPO_SOURCE_TEST"
	f.s, err = New(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{strings.Repeat("r", 32), strings.Repeat("x", 32)} {
		if status, _ := call(token, q); status != 403 {
			t.Fatal("ordinary execution credential read source", status)
		}
	}
	status, out := call(strings.Repeat("s", 32), q)
	if status != 200 || ri.ValidateProviderSourceInspection(q, out) != nil || out.ExactCommit != f.head || out.TreeOID != tree || len(out.Manifests) != 4 || out.Coverage.State != "complete" {
		t.Fatalf("actual fixed workspace snapshot failed: status=%d manifests=%d state=%s", status, len(out.Manifests), out.Coverage.State)
	}
	_, again := call(strings.Repeat("s", 32), q)
	if again.SnapshotSHA256 != out.SnapshotSHA256 {
		t.Fatal("immutable source snapshot was not stable")
	}
	scoped := q
	scoped.RelativeRoots = []string{"packages/a"}
	status, subset := call(strings.Repeat("s", 32), scoped)
	if status != 200 || len(subset.Manifests) != 1 || subset.Manifests[0].Path != "packages/a/package.json" || subset.Scopes[0].TreeOID != aTree {
		t.Fatal("source scope escaped the exact Git subtree", status)
	}
	for _, root := range []string{"../packages", "packages/../b", "/packages", "packages\\a", ".git"} {
		bad := q
		bad.RelativeRoots = []string{root}
		if status, _ := call(strings.Repeat("s", 32), bad); status != 400 {
			t.Fatal("unsafe root accepted", status)
		}
	}
	foreign := q
	foreign.ProjectID = "foreign"
	if status, _ := call(strings.Repeat("s", 32), foreign); status != 403 {
		t.Fatal("foreign source scope accepted", status)
	}
	stale := q
	stale.ReceiptSHA256 = strings.Repeat("0", 64)
	if status, _ := call(strings.Repeat("s", 32), stale); status != 409 {
		t.Fatal("unproven source accepted", status)
	}
	if f.effects != 1 {
		t.Fatal("read-only inspection repeated an integration effect", f.effects)
	}
}
