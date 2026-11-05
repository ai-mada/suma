package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	"github.com/ai-mada/suma/internal/repositoryprovider"
)

func TestFixedGitBundleImportsExactProviderSource(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git is required to verify real provider bundle import")
	}
	git, err = filepath.EvalSymlinks(git)
	if err != nil {
		t.Fatal(err)
	}
	source, base := gitFixture(t)
	dir := t.TempDir()
	managed := filepath.Join(dir, "source.git")
	isolated := filepath.Join(dir, "recipient.git")
	gitTest(t, dir, "clone", "--bare", "--no-hardlinks", source, managed)
	gitTest(t, dir, "clone", "--bare", "--no-local", source, isolated)
	gitTest(t, managed, "update-ref", "refs/heads/baseline", base)
	gitTest(t, source, "checkout", "--detach", base)
	const content = "provider candidate\n"
	if err = os.WriteFile(filepath.Join(source, "provider.txt"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, source, "add", "provider.txt")
	gitTest(t, source, "commit", "-m", "provider delivery fixture")
	head := strings.TrimSpace(gitTest(t, source, "rev-parse", "HEAD"))
	gitTest(t, source, "push", managed, "HEAD:refs/heads/delivery")

	const exportEnv = "SUMA_TEST_RUNNER_PROVIDER_EXPORT"
	exportToken := strings.Repeat("e", 40)
	t.Setenv(exportEnv, exportToken)
	t.Setenv("SUMA_TEST_RUNNER_PROVIDER_CONTROL", strings.Repeat("c", 40))
	t.Setenv("SUMA_TEST_RUNNER_PROVIDER_RUNNER", strings.Repeat("r", 40))
	t.Setenv("SUMA_TEST_RUNNER_PROVIDER_RECONCILE", strings.Repeat("q", 40))
	provider, err := repositoryprovider.New(repositoryprovider.Config{
		ProviderID: "provider", TenantID: "tenant", ProjectID: "project", ControlURL: "http://127.0.0.1:1",
		ControlTokenEnv: "SUMA_TEST_RUNNER_PROVIDER_CONTROL", RunnerTokenEnv: "SUMA_TEST_RUNNER_PROVIDER_RUNNER",
		ReconcileTokenEnv: "SUMA_TEST_RUNNER_PROVIDER_RECONCILE", SourceExportTokenEnv: exportEnv,
		GitExecutable: git, StateDir: filepath.Join(dir, "provider-state"),
		Targets: map[string]repositoryprovider.TargetConfig{
			"delivery": {BareRepository: managed, Ref: "refs/heads/delivery", DeliveryBaseAlias: "baseline"},
			"baseline": {BareRepository: managed, Ref: "refs/heads/baseline"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	ctx := context.Background()
	target, err := provider.Target(ctx, "delivery")
	if err != nil || target.HeadCommit != head {
		t.Fatalf("provider fixture did not observe source commit: %v", err)
	}
	request := ri.RepositorySourceExportRequest{
		Protocol: ri.RepositorySourceExportProtocol, TenantID: "tenant", ProjectID: "project", ProviderID: "provider",
		TargetAlias: target.Alias, TargetRef: target.Ref, TargetConfigSHA256: target.ConfigSHA256,
		ExpectedHead: head, ExpectedBase: base, ExpectedFenceOID: target.FenceOID, ExpectedGeneration: target.Generation,
	}
	requestSHA, err := ri.RepositorySourceExportDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest := httptest.NewRequest(http.MethodPost, "/source-exports", bytes.NewReader(data))
	httpRequest.Header.Set("Authorization", "Bearer "+exportToken)
	response := httptest.NewRecorder()
	provider.Handler().ServeHTTP(response, httpRequest)
	if response.Code != http.StatusOK || response.Header().Get("X-Suma-Source-Request-SHA256") != requestSHA {
		t.Fatalf("source export failed: status %d", response.Code)
	}
	path := filepath.Join(dir, "provider.bundle")
	if err = os.WriteFile(path, response.Body.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	bundle := fixedGitBundle{anchor: base, base: base, head: head, sha256: response.Header().Get("X-Suma-Bundle-SHA256"), size: int64(response.Body.Len())}
	refs := gitTest(t, isolated, "show-ref")
	wrong := bundle
	wrong.sha256 = strings.Repeat("0", 64)
	if err = importFixedGitBundle(ctx, isolated, path, wrong); err == nil {
		t.Fatal("import accepted a different bundle digest")
	}
	wrong = bundle
	wrong.head = base
	if err = importFixedGitBundle(ctx, isolated, path, wrong); err == nil {
		t.Fatal("import accepted a different advertised commit")
	}
	if err = nativeGitCommand(ctx, isolated, "cat-file", "-e", head+"^{commit}").Run(); err == nil {
		t.Fatal("baseline-only recipient unexpectedly contained source commit")
	}
	if err = importFixedGitBundle(ctx, isolated, path, bundle); err != nil {
		t.Fatal(err)
	}
	if gitTest(t, isolated, "show", head+":provider.txt") != content || gitTest(t, isolated, "show-ref") != refs {
		t.Fatal("strict import did not reproduce the exact source without changing refs")
	}
}
