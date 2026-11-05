package repositoryprovider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func TestRepositorySourceExportBasisUsesOnlyReconcileAuthority(t *testing.T) {
	f := newGitFixture(t)
	updateRef := func(args ...string) {
		t.Helper()
		cmd := exec.Command(f.cfg.GitExecutable, append([]string{"update-ref"}, args...)...)
		cmd.Dir = f.source
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("update fixture ref: %s, %v", output, err)
		}
	}
	updateRef("refs/heads/delivery", f.head)
	updateRef("refs/heads/baseline", f.base)
	if err := f.s.Close(); err != nil {
		t.Fatal(err)
	}
	f.s = nil
	f.cfg.Targets = map[string]TargetConfig{
		"delivery": {BareRepository: f.source, Ref: "refs/heads/delivery", DeliveryBaseAlias: "baseline"},
		"baseline": {BareRepository: f.source, Ref: "refs/heads/baseline"},
	}
	const exportEnv = "REPO_SOURCE_EXPORT_BASIS_TEST"
	exportToken := strings.Repeat("e", 48)
	t.Setenv(exportEnv, exportToken)
	f.cfg.SourceExportTokenEnv = exportEnv
	var err error
	f.s, err = New(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	handler := f.s.Handler()
	call := func(token, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, token := range []string{exportToken, os.Getenv(f.cfg.RunnerTokenEnv), os.Getenv(f.cfg.ControlTokenEnv)} {
		if r := call(token, "/source-export-basis/delivery"); r.Code != http.StatusForbidden {
			t.Fatalf("metadata used the wrong credential class: %d", r.Code)
		}
	}
	reconcile := os.Getenv(f.cfg.ReconcileTokenEnv)
	if r := call(reconcile, "/source-export-basis/baseline"); r.Code != http.StatusNotFound {
		t.Fatalf("an unpaired alias became a source: %d", r.Code)
	}
	if r := call(reconcile, "/source-export-basis/delivery?baseline=other"); r.Code != http.StatusBadRequest {
		t.Fatalf("accepted caller-selected baseline: %d", r.Code)
	}
	r := call(reconcile, "/source-export-basis/delivery")
	var basis ri.RepositorySourceExportBasis
	if r.Code != http.StatusOK || r.Header().Get("Cache-Control") != "no-store" || json.NewDecoder(r.Body).Decode(&basis) != nil ||
		ri.ValidateRepositorySourceExportBasis("delivery", basis) != nil || basis.Source.HeadCommit != f.head || basis.Baseline.HeadCommit != f.base {
		t.Fatal("basis did not return the configured live pair", r.Code)
	}
	updateRef("refs/heads/baseline", f.head, f.base)
	r = call(reconcile, "/source-export-basis/delivery")
	if r.Code != http.StatusOK || json.NewDecoder(r.Body).Decode(&basis) != nil || basis.Baseline.HeadCommit != f.head {
		t.Fatal("metadata hid a live baseline advance", r.Code)
	}
}
