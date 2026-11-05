package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	rp "github.com/ai-mada/suma/internal/repositoryprovider"
)

func TestTaskDeliveryRepositorySourceHTTPReadsExactLiveGitHead(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git is required for repository source verification")
	}
	git, err = filepath.EvalSymlinks(git)
	if err != nil {
		t.Fatal(err)
	}
	h := newWorkBuddyTaskDispatchFixture(t)
	f := h.f
	f.handler = (&Server{Service: f.s, Engine: f.e, WorkBuddy: h.w}).Handler()
	a := terminalCandidateWaitingWorkBuddy(t, h)
	ctx := context.Background()
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", h.task.ID)
	if err != nil {
		t.Fatal(err)
	}

	checkout := t.TempDir()
	workspaceGitTestCommand(t, checkout, "init", "--initial-branch=delivery", "--object-format=sha1")
	workspaceGitTestCommand(t, checkout, "commit", "--allow-empty", "-m", "Initial fixture revision")
	before := workspaceGitTestCommand(t, checkout, "rev-parse", "HEAD")
	workspaceGitTestWrite(t, checkout, "candidate.txt", "Delivery source fixture\n")
	workspaceGitTestCommand(t, checkout, "add", "--", "candidate.txt")
	workspaceGitTestCommand(t, checkout, "commit", "-m", "Review this exact source")
	head := workspaceGitTestCommand(t, checkout, "rev-parse", "HEAD")
	bare := filepath.Join(t.TempDir(), "delivery.git")
	workspaceGitTestCommand(t, checkout, "clone", "--bare", "--no-hardlinks", checkout, bare)
	const ref = "refs/heads/delivery"
	workspaceGitTestCommand(t, bare, "update-ref", ref, before, head)

	const providerID, alias = "delivery-repository-provider", "incoming"
	for name, token := range map[string]string{
		"SUMA_TEST_DELIVERY_CONTROL":   strings.Repeat("c", 48),
		"SUMA_TEST_DELIVERY_RUNNER":    strings.Repeat("r", 48),
		"SUMA_TEST_DELIVERY_RECONCILE": strings.Repeat("x", 48),
	} {
		t.Setenv(name, token)
	}
	provider, err := rp.New(rp.Config{
		ProviderID: providerID, TenantID: f.user.TenantID, ProjectID: f.project,
		ControlURL: "http://127.0.0.1:1", ControlTokenEnv: "SUMA_TEST_DELIVERY_CONTROL",
		RunnerTokenEnv: "SUMA_TEST_DELIVERY_RUNNER", ReconcileTokenEnv: "SUMA_TEST_DELIVERY_RECONCILE",
		GitExecutable: git, StateDir: filepath.Join(t.TempDir(), "provider"),
		Targets: map[string]rp.TargetConfig{alias: {BareRepository: bare, Ref: ref}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = provider.Close() })

	var probes atomic.Int32
	var moveFence atomic.Bool
	handler := provider.Handler()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/targets/"+alias {
			t.Errorf("unexpected repository request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		probes.Add(1)
		observed := httptest.NewRecorder()
		handler.ServeHTTP(observed, r)
		if moveFence.Swap(false) {
			// Change the original execution generation after observing the Git head,
			// before the control plane can receive this provider response.
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			if _, err := f.s.DB.ExecContext(ctx, `UPDATE dag_nodes SET fence=fence+1 WHERE run_id=? AND id=?`, a.RunID, a.NodeID); err != nil {
				t.Errorf("change original generation during provider I/O: %v", err)
			}
		}
		for name, values := range observed.Header() {
			w.Header()[name] = values
		}
		w.WriteHeader(observed.Code)
		_, _ = w.Write(observed.Body.Bytes())
	}))
	t.Cleanup(remote.Close)
	f.s.RepositoryProviders = []RepositoryProviderConfig{{
		ID: providerID, TenantID: f.user.TenantID, ProjectID: f.project, TargetAliases: []string{alias},
		TokenHash: digest(os.Getenv("SUMA_TEST_DELIVERY_CONTROL")), Origin: remote.URL,
		ReconcileToken: os.Getenv("SUMA_TEST_DELIVERY_RECONCILE"),
	}}
	target := issueRepairRuntimeDecode[RepositoryIntegrationTarget](t, f.call("POST", "/repository-integration-targets",
		RepositoryIntegrationTargetCreateRequest{ProjectID: f.project, Name: "Incoming fixture delivery", ProviderID: providerID, TargetAlias: alias, IdempotencyKey: newID()}, 201))
	if probes.Load() != 1 || target.Observation == nil || target.Observation.HeadCommit != before {
		t.Fatal("fixture did not register the initial Git head")
	}
	workspaceGitTestCommand(t, bare, "update-ref", ref, head, before)

	path := "/tasks/" + task.ID + "/delivery/repository-source"
	query := func(taskVersion int, run, targetID string, targetVersion int) string {
		return path + "?" + (url.Values{
			"project_id": {f.project}, "task_version": {strconv.Itoa(taskVersion)}, "run_id": {run},
			"target_id": {targetID}, "target_version": {strconv.Itoa(targetVersion)},
		}).Encode()
	}
	current := query(task.Version, a.RunID, target.ID, target.Version)
	f.call("GET", current+"&target_id="+target.ID, nil, 400)
	f.call("GET", current+"&unexpected=1", nil, 400)
	f.call("GET", query(task.Version+1, a.RunID, target.ID, target.Version), nil, 409)
	f.call("GET", query(task.Version, newID(), target.ID, target.Version), nil, 409)
	f.call("GET", query(task.Version, a.RunID, target.ID, target.Version+1), nil, 409)
	f.call("GET", query(task.Version, a.RunID, newID(), target.Version), nil, 404)

	ownerCookie, ownerView := f.cookie, f.view
	inv := governanceInvite(f, "repository-source-viewer@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	governanceAccept(f, inv, "repository-source-viewer@example.test")
	f.call("GET", "/tasks/"+task.ID+"/delivery?project_id="+f.project, nil, 200)
	f.call("GET", current, nil, 403)
	f.cookie, f.view = ownerCookie, ownerView
	if probes.Load() != 1 {
		t.Fatal("invalid or unauthorized reads reached the repository provider")
	}

	beforeEvents, beforeRuns := domainACount(t, f.s, "dag_events"), domainACount(t, f.s, "dag_runs")
	out := f.call("GET", current, nil, 200)
	if len(out) != 11 || out["task_id"] != task.ID || out["task_version"] != float64(task.Version) ||
		out["run_id"] != a.RunID || out["node_id"] != a.NodeID || out["attempt"] != float64(a.Attempt) ||
		out["target_id"] != target.ID || out["target_version"] != float64(target.Version) ||
		out["ref"] != ref || out["head_commit"] != head || out["object_format"] != "sha1" {
		t.Fatalf("unexpected repository source projection: %+v", out)
	}
	// The observation timestamp is part of the projection, so verify it explicitly
	// and reject accidentally exposing any additional provider internals.
	if stamp, ok := out["observed_at"].(string); !ok || stamp == "" {
		t.Fatal("missing provider observation timestamp")
	} else if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
		t.Fatal(err)
	}
	if probes.Load() != 2 || domainACount(t, f.s, "dag_events") != beforeEvents ||
		domainACount(t, f.s, "dag_runs") != beforeRuns || domainACount(t, f.s, "suma_repository_operations") != 0 ||
		workspaceGitTestCommand(t, bare, "rev-parse", ref) != head || h.creates.Load() != 1 || h.queries.Load() != 0 {
		t.Fatal("read-only repository observation changed execution or Git state")
	}

	moveFence.Store(true)
	f.call("GET", current, nil, 409)
	if probes.Load() != 3 || moveFence.Load() {
		t.Fatal("fixture did not race an authorized observation")
	}
	f.call("GET", current, nil, 409)
	if probes.Load() != 3 {
		t.Fatal("stale original assignment reached the repository provider")
	}
}
