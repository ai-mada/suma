package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
	"github.com/ai-mada/suma/internal/runner"
)

// Faults are confined to the owned HTTP transport. The real provider still
// downloads, activates, persists, and checks the original sealed bytes. No
// positive observation or deployment result is manufactured by this fixture.
type rolloutRuntimeFaults struct {
	mu                                         sync.Mutex
	libraryArtifact, webArtifact, webOperation string
	started                                    chan struct{}
	release                                    chan struct{}
	startOnce, releaseOnce                     sync.Once
	dropInspect, dropReports                   bool
	posts                                      map[string]int
	webBeforeACKLoss                           rc.DeploymentObservation
}

func (f *rolloutRuntimeFaults) control(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		library := f.libraryArtifact
		f.mu.Unlock()
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/provider/artifacts/"+library+"/blobs/") && library != "" {
			f.startOnce.Do(func() { close(f.started) })
			select {
			case <-f.release:
			case <-r.Context().Done():
				return
			}
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/provider/deployment-observations" {
			raw, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
			if err != nil || len(raw) > 1<<20 {
				http.Error(w, "owned_transport_body_invalid", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))
			var observation rc.DeploymentObservation
			if json.Unmarshal(raw, &observation) != nil {
				http.Error(w, "owned_transport_body_invalid", http.StatusBadRequest)
				return
			}
			f.mu.Lock()
			drop := f.dropReports && observation.OperationID == f.webOperation && f.webOperation != ""
			f.mu.Unlock()
			if drop {
				http.Error(w, "owned_observation_transport_unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (f *rolloutRuntimeFaults) management(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		blocked := f.dropInspect && f.webOperation != "" && r.URL.Path == "/v2/operations/"+f.webOperation
		f.mu.Unlock()
		if blocked {
			http.Error(w, "owned_inspection_transport_unavailable", http.StatusServiceUnavailable)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v2/deployments" {
			next.ServeHTTP(w, r)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
		if err != nil || len(raw) > 1<<20 {
			http.Error(w, "owned_transport_body_invalid", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var request rc.DeploymentRequest
		if json.Unmarshal(raw, &request) != nil {
			http.Error(w, "owned_transport_body_invalid", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.posts[request.OperationID]++
		drop := request.ArtifactID == f.webArtifact && f.webArtifact != ""
		if drop {
			f.webOperation = request.OperationID
			f.dropInspect, f.dropReports = true, true
		}
		f.mu.Unlock()
		response := httptest.NewRecorder()
		next.ServeHTTP(response, r)
		if drop && (response.Code == http.StatusOK || response.Code == http.StatusAccepted) {
			// Keep the original Runner running until the actual provider obtains
			// its current effect grant and reaches a terminal fact. Dropping the
			// ACK earlier correctly blocks a not-yet-authorized effect when the
			// Runner enters reconciliation and the parent pauses.
			deadline := time.NewTimer(10 * time.Second)
			defer deadline.Stop()
			for {
				inspect := httptest.NewRequest(http.MethodGet, "/v2/operations/"+request.OperationID, nil)
				inspect.Header.Set("Authorization", r.Header.Get("Authorization"))
				actual := httptest.NewRecorder()
				next.ServeHTTP(actual, inspect)
				var observation rc.DeploymentObservation
				if actual.Code == http.StatusOK && json.Unmarshal(actual.Body.Bytes(), &observation) == nil {
					f.mu.Lock()
					f.webBeforeACKLoss = observation
					f.mu.Unlock()
					if observation.State == "succeeded" || observation.State == "failed" || observation.State == "cancelled" || observation.State == "reconciling" {
						break
					}
				}
				select {
				case <-deadline.C:
					http.Error(w, "owned_terminal_before_ack_timeout", http.StatusServiceUnavailable)
					return
				case <-r.Context().Done():
					return
				case <-time.After(25 * time.Millisecond):
				}
			}
			// Only transport is lost; no actual observation is changed or sent
			// to the control plane by this fault injector.
			http.Error(w, "owned_dispatch_ack_lost", http.StatusServiceUnavailable)
			return
		}
		for key, values := range response.Header() {
			w.Header()[key] = append([]string{}, values...)
		}
		w.WriteHeader(response.Code)
		_, _ = w.Write(response.Body.Bytes())
	})
}

func (f *rolloutRuntimeFaults) unblockLibrary() {
	f.releaseOnce.Do(func() { close(f.release) })
}

func rolloutRuntimeItem(t *testing.T, view ApplicationReleaseRolloutView, app string) ApplicationReleaseRolloutItem {
	t.Helper()
	for _, item := range view.Items {
		if item.ApplicationID == app {
			return item
		}
	}
	t.Fatal("rollout lost its original application member", app)
	return ApplicationReleaseRolloutItem{}
}

func TestApplicationReleaseRolloutActualOrderedPauseAndUnknownRecovery(t *testing.T) {
	faults := &rolloutRuntimeFaults{started: make(chan struct{}), release: make(chan struct{}), posts: map[string]int{}}
	t.Cleanup(faults.unblockLibrary)
	p := newActualReleaseSetFixture(t, faults.control, faults.management)
	// LIFO cleanup releases downloads before closing the fixture servers.
	t.Cleanup(faults.unblockLibrary)
	h, f := p.application, p.application.f
	faults.mu.Lock()
	faults.libraryArtifact, faults.webArtifact = p.candidates["library"].ArtifactID, p.candidates["web"].ArtifactID
	faults.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	approved := map[string]ReleasePlan{}
	for _, binding := range p.group.Group.Bindings {
		plan, err := f.s.ReleasePlan(ctx, f.actor, f.project, binding.PlanID)
		if err != nil {
			t.Fatal(err)
		}
		plan, err = f.s.ApproveReleasePlan(ctx, f.actor, f.project, plan.ID, f.body(map[string]any{"expected_version": plan.Version, "plan_revision": plan.Revision, "plan_sha256": plan.SHA256, "decision": "approved", "reason": "Fixture owner separately approves this original exact plan before authorizing an ordered rollout.", "idempotency_key": newID()}))
		if err != nil {
			t.Fatal(err)
		}
		approved[binding.ApplicationID] = plan
	}
	// Switch only ordinary work to the shared Pool after preparing the
	// original approved plans. The ordered release family must stay private.
	pool, grant := dispatchAuthorizedPool(t, f)
	dispatchSetRoute(t, f, 0, &grant)
	defaultRoute := testProjectExecutionRoute(t, f)
	if defaultRoute.PoolID != pool.ID || defaultRoute.GrantID != grant.ID {
		t.Fatal("fixture did not configure an authorized project Pool default", defaultRoute)
	}
	base := "/application-release-sets/" + p.registered.Set.ID + "/rollouts"
	request := ApplicationReleaseRolloutPreviewRequest{ProjectID: f.project, PlanGroupID: p.group.Group.ID, ExpectedSetSHA256: p.registered.Set.SnapshotSHA256, ExpectedPlanGroupSHA256: p.group.Group.SnapshotSHA256, Reason: "Explicit fixture authorization to execute original approved plans in dependency order; unknown outcomes must pause and retain the same operations."}
	preview := issueRepairRuntimeDecode[ApplicationReleaseRolloutPreview](t, f.call("POST", base+"/preview", request, 200))
	if !preview.CanStart || len(preview.Rollout.Members) != 2 {
		t.Fatal("ordered rollout preview rejected current original approvals", preview.Blockers)
	}
	view := issueRepairRuntimeDecode[ApplicationReleaseRolloutView](t, f.call("POST", base, ApplicationReleaseRolloutStartRequest{ApplicationReleaseRolloutPreviewRequest: request, ExpectedPreviewSHA256: preview.PreviewSHA256, IdempotencyKey: newID()}, 201))
	assertDedicatedReleaseRun(t, ctx, f, view.Rollout.ParentRunID)
	url := base + "/" + view.Rollout.ID
	read := func() ApplicationReleaseRolloutView {
		t.Helper()
		if err := f.e.Tick(ctx); err != nil {
			t.Fatal("advance the existing Go DAG", err)
		}
		return issueRepairRuntimeDecode[ApplicationReleaseRolloutView](t, f.call("GET", url+"?project_id="+f.project, nil, 200))
	}
	command := func(current ApplicationReleaseRolloutView, action, previewSHA string) ApplicationReleaseRolloutView {
		t.Helper()
		return issueRepairRuntimeDecode[ApplicationReleaseRolloutView](t, f.call("POST", url+"/commands", ApplicationReleaseRolloutCommandRequest{ProjectID: f.project, ExpectedRunVersion: current.RunVersion, ExpectedRolloutSHA256: current.Rollout.SnapshotSHA256, Action: action, ExpectedPreviewSHA256: previewSHA, Reason: "Fixture owner explicitly " + action + "s the original rollout; no approval is fabricated.", IdempotencyKey: newID()}, 200))
	}
	resume := func(current ApplicationReleaseRolloutView) ApplicationReleaseRolloutView {
		t.Helper()
		preview := issueRepairRuntimeDecode[ApplicationReleaseRolloutResumePreview](t, f.call("POST", url+"/resume-preview", ApplicationReleaseRolloutResumePreviewRequest{ProjectID: f.project, ExpectedRunVersion: current.RunVersion, ExpectedRolloutSHA256: current.Rollout.SnapshotSHA256}, 200))
		if !preview.CanResume {
			t.Fatal("current original operation could not resume", preview.Blockers)
		}
		return command(current, "resume", preview.PreviewSHA256)
	}
	capabilities := runner.ExternalCapabilities(p.config.External)
	claim := func(appID string) *runner.Assignment {
		t.Helper()
		for {
			view = read()
			item := rolloutRuntimeItem(t, view, appID)
			if item.DeploymentOperationID != "" && item.DeploymentState == "queued" {
				deployment, err := f.s.ReleaseDeployment(ctx, f.actor, f.project, item.DeploymentOperationID)
				if err != nil {
					t.Fatal("read queued release child", err)
				}
				wrong := []string{"external_workflow:" + p.providerID, rc.ReleaseProviderCapabilityPrefix + "wrong-provider"}
				if a, err := p.client.Claim(ctx, "release-set-deployer124", wrong); err != nil || a != nil {
					t.Fatal("wrong provider marker acquired an actual ordered child", err)
				}
				assertReleaseNodeUnleased(t, f, deployment.RunID, deployment.NodeID)
				faults.mu.Lock()
				posts := faults.posts[deployment.OperationID]
				faults.mu.Unlock()
				if posts != 0 {
					t.Fatal("provider called before exact runner capability", posts)
				}
				a, err := p.client.Claim(ctx, "release-set-deployer124", capabilities)
				if err != nil || a == nil || a.RunID != deployment.RunID {
					t.Fatal("claim actual original release child", err)
				}
				return a
			}
			select {
			case <-ctx.Done():
				t.Fatal("original rollout child never became ready", view)
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	library := claim(h.library.ID)
	assertDedicatedReleaseRun(t, ctx, f, library.RunID)
	if library.ReleaseExecution == nil || library.ReleaseExecution.ArtifactID != p.candidates["library"].ArtifactID {
		t.Fatal("dependent application started before its library")
	}
	libraryResult := make(chan runner.Result, 1)
	go func() { libraryResult <- runner.Execute(ctx, *library, p.config) }()
	select {
	case <-faults.started:
	case <-ctx.Done():
		t.Fatal("real library artifact download did not start")
	}
	view = read()
	if item := rolloutRuntimeItem(t, view, h.web.ID); item.DeploymentOperationID != "" {
		t.Fatal("downstream original plan acquired an operation before library health", item)
	}
	webEnvironment, err := f.s.ReleaseEnvironment(ctx, f.actor, f.project, p.environments["web"].ID)
	if err != nil || webEnvironment.Generation != 0 || webEnvironment.BarrierOperationID != "" {
		t.Fatal("unstarted downstream environment was reserved", err, webEnvironment)
	}
	if a, err := p.client.Claim(ctx, "release-set-deployer124", capabilities); err != nil || a != nil {
		t.Fatal("downstream child was claimable before prerequisite health", err)
	}
	view = command(view, "pause", "")
	if view.State != "paused" {
		t.Fatal("explicit pause did not stop future child starts", view.State)
	}
	faults.unblockLibrary()
	var result runner.Result
	select {
	case result = <-libraryResult:
	case <-ctx.Done():
		t.Fatal("paused rollout did not let its running child drain")
	}
	if result.ExitCode != 0 || result.OutcomeUnknown || result.ReleaseEvidence == nil {
		t.Fatal("actual library deployment failed", result.Error)
	}
	if result.OperationID == "" {
		result.OperationID = newID()
	}
	if err := p.client.Complete(ctx, result); err != nil {
		t.Fatal("actual library final evidence was rejected", err)
	}
	view = read()
	if view.State != "paused" || rolloutRuntimeItem(t, view, h.library.ID).DeploymentState != "deployed_healthy" || rolloutRuntimeItem(t, view, h.web.ID).DeploymentOperationID != "" {
		t.Fatal("paused rollout did not retain healthy first member and unstarted dependent", view)
	}
	paused := view
	view = resume(view)
	web := claim(h.web.ID)
	assertDedicatedReleaseRun(t, ctx, f, web.RunID)
	if web.ReleaseExecution == nil || web.ReleaseExecution.ArtifactID != p.candidates["web"].ArtifactID {
		t.Fatal("resume did not use the original dependent plan")
	}
	unknown := runner.Execute(ctx, *web, p.config)
	faults.mu.Lock()
	beforeLoss := faults.webBeforeACKLoss
	faults.mu.Unlock()
	if beforeLoss.State != "succeeded" || beforeLoss.Effect.State != "applied" || beforeLoss.Health.State != "passed" || beforeLoss.OperationID != web.ReleaseExecution.OperationID {
		t.Fatalf("owned ACK-loss boundary lacked actual provider success: state=%s effect=%s health=%s error=%v", beforeLoss.State, beforeLoss.Effect.State, beforeLoss.Health.State, beforeLoss.Error)
	}
	if !unknown.OutcomeUnknown || unknown.ExitCode == 0 {
		t.Fatal("lost real POST acknowledgement was turned into success", unknown)
	}
	if unknown.OperationID == "" {
		unknown.OperationID = newID()
	}
	if err := p.client.Complete(ctx, unknown); err != nil {
		t.Fatal("original unknown runner outcome was not retained", err)
	}
	// Capture the original stored history before reading the current summary.
	// A read-side unknown projection must not invent a provider observation.
	var storedBeforeUnknownView, observationBeforeUnknownView string
	var observationCountBeforeUnknownView int
	if err := f.s.DB.QueryRowContext(ctx, f.s.q(`SELECT data FROM suma_release_deployments WHERE tenant_id=? AND project_id=? AND operation_id=?`), f.user.TenantID, f.project, web.ReleaseExecution.OperationID).Scan(&storedBeforeUnknownView); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.QueryRowContext(ctx, f.s.q(`SELECT COUNT(*) FROM suma_release_observations WHERE tenant_id=? AND project_id=? AND operation_id=?`), f.user.TenantID, f.project, web.ReleaseExecution.OperationID).Scan(&observationCountBeforeUnknownView); err != nil {
		t.Fatal(err)
	}
	if observationCountBeforeUnknownView > 0 {
		if err := f.s.DB.QueryRowContext(ctx, f.s.q(`SELECT data FROM suma_release_observations WHERE tenant_id=? AND project_id=? AND operation_id=? ORDER BY version DESC LIMIT 1`), f.user.TenantID, f.project, web.ReleaseExecution.OperationID).Scan(&observationBeforeUnknownView); err != nil {
			t.Fatal(err)
		}
	}
	for {
		view = read()
		if view.State == "paused" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("unknown child failed to pause ordered rollout", view)
		case <-time.After(10 * time.Millisecond):
		}
	}
	unknownView := view
	unknownItem, healthySibling := rolloutRuntimeItem(t, unknownView, h.web.ID), rolloutRuntimeItem(t, unknownView, h.library.ID)
	if unknownItem.DeploymentState != "reconciling" || unknownItem.EffectState != "unknown" || unknownItem.HealthState != "unknown" || healthySibling.DeploymentState != "deployed_healthy" || healthySibling.EffectState != "applied" || healthySibling.HealthState != "passed" {
		t.Fatal("current unknown effect was reported as not started or replaced the healthy sibling", unknownView)
	}
	var storedAfterUnknownView, observationAfterUnknownView string
	var observationCountAfterUnknownView int
	if err := f.s.DB.QueryRowContext(ctx, f.s.q(`SELECT data FROM suma_release_deployments WHERE tenant_id=? AND project_id=? AND operation_id=?`), f.user.TenantID, f.project, web.ReleaseExecution.OperationID).Scan(&storedAfterUnknownView); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.QueryRowContext(ctx, f.s.q(`SELECT COUNT(*) FROM suma_release_observations WHERE tenant_id=? AND project_id=? AND operation_id=?`), f.user.TenantID, f.project, web.ReleaseExecution.OperationID).Scan(&observationCountAfterUnknownView); err != nil {
		t.Fatal(err)
	}
	if observationCountAfterUnknownView > 0 {
		if err := f.s.DB.QueryRowContext(ctx, f.s.q(`SELECT data FROM suma_release_observations WHERE tenant_id=? AND project_id=? AND operation_id=? ORDER BY version DESC LIMIT 1`), f.user.TenantID, f.project, web.ReleaseExecution.OperationID).Scan(&observationAfterUnknownView); err != nil {
			t.Fatal(err)
		}
	}
	if storedAfterUnknownView != storedBeforeUnknownView || observationCountAfterUnknownView != observationCountBeforeUnknownView || observationAfterUnknownView != observationBeforeUnknownView {
		t.Fatal("read-side unknown projection rewrote original deployment or provider history")
	}
	if rolloutRuntimeItem(t, view, h.library.ID).DeploymentOperationID != library.ReleaseExecution.OperationID || rolloutRuntimeItem(t, view, h.web.ID).DeploymentOperationID != web.ReleaseExecution.OperationID {
		t.Fatal("pause replaced original healthy or unknown operation")
	}
	// Observe the actual provider's persisted terminal facts independently of
	// the intentionally unavailable transport; retain its exact real bytes.
	for {
		request := httptest.NewRequest(http.MethodGet, "/v2/operations/"+web.ReleaseExecution.OperationID, nil)
		request.Header.Set("Authorization", "Bearer "+p.transportKey)
		response := httptest.NewRecorder()
		p.provider.Handler().ServeHTTP(response, request)
		var observed rc.DeploymentObservation
		if response.Code == 200 && json.Unmarshal(response.Body.Bytes(), &observed) == nil && observed.State == "succeeded" && observed.Effect.State == "applied" && observed.Health.State == "passed" {
			break
		}
		if observed.State == "failed" || observed.State == "cancelled" || observed.State == "reconciling" {
			t.Fatalf("actual web provider terminated without healthy effect: state=%s effect=%s health=%s error=%v", observed.State, observed.Effect.State, observed.Health.State, observed.Error)
		}
		select {
		case <-ctx.Done():
			t.Fatal("real web provider never completed its persisted effect and health")
		case <-time.After(10 * time.Millisecond):
		}
	}
	faults.mu.Lock()
	faults.dropInspect = false
	faults.mu.Unlock()
	current, err := f.s.ReleaseDeployment(ctx, f.actor, f.project, web.ReleaseExecution.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := f.s.ReconcileReleaseDeployment(ctx, f.actor, f.project, current.OperationID, f.body(map[string]any{"expected_version": current.Version, "idempotency_key": newID()}), f.e)
	if err != nil || reconciled.State != "deployed_healthy" || reconciled.OperationID != current.OperationID {
		t.Fatal("recovery failed to reconcile original real provider operation", err)
	}
	faults.mu.Lock()
	faults.dropReports = false
	faults.mu.Unlock()
	view = read()
	if view.State != "paused" {
		t.Fatal("fact reconciliation implicitly resumed a paused rollout", view.State)
	}
	view = resume(view)
	for {
		view = read()
		if view.State == "succeeded" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("healthy original children did not finish parent rollout", view)
		case <-time.After(10 * time.Millisecond):
		}
	}
	for _, item := range view.Items {
		if item.DeploymentState != "deployed_healthy" || item.EffectState != "applied" || item.HealthState != "passed" || item.ChildRunState != "succeeded" {
			t.Fatal("parent success omitted an original child fact", item)
		}
	}
	for _, plan := range approved {
		current, err := f.s.ReleasePlan(ctx, f.actor, f.project, plan.ID)
		if err != nil || current.Approval.Decision != "approved" || len(current.ApprovalHistory) != 1 || current.SHA256 != plan.SHA256 {
			t.Fatal("rollout manufactured another original-plan approval", err)
		}
	}
	faults.mu.Lock()
	postCounts := map[string]int{}
	for operation, count := range faults.posts {
		postCounts[operation] = count
	}
	faults.mu.Unlock()
	if len(postCounts) != 2 || postCounts[library.ReleaseExecution.OperationID] != 1 || postCounts[web.ReleaseExecution.OperationID] != 1 {
		t.Fatal("recovery redispatched an already accepted provider operation", postCounts)
	}
	var original string
	if err := f.s.DB.QueryRow(`SELECT original_result FROM dag_managed_resolutions WHERE run_id=? AND node_id=? AND attempt=?`, web.RunID, web.NodeID, web.Attempt).Scan(&original); err != nil {
		t.Fatal("original unknown result was not retained", err)
	}
	var preserved dag.Result
	if json.Unmarshal([]byte(original), &preserved) != nil || !preserved.OutcomeUnknown || preserved.Error != unknown.Error {
		t.Fatal("reconciliation erased original unknown execution facts")
	}
	for app, candidate := range p.candidates {
		response, err := http.Get(p.provider.SiteURL(app+"-static124") + "/")
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
		response.Body.Close()
		env, envErr := f.s.ReleaseEnvironment(ctx, f.actor, f.project, p.environments[app].ID)
		if err != nil || response.StatusCode != 200 || string(body) != releaseSetRuntimeHTML(app) || envErr != nil || env.Generation != 1 || env.ActiveGeneration != 1 || env.ActiveArtifactID != candidate.ArtifactID || env.BarrierOperationID != "" {
			t.Fatal("final actual bytes, generation or original environment barrier differs", app, err, envErr)
		}
	}
	if route := testProjectExecutionRoute(t, f); route != defaultRoute {
		t.Fatal("ordered release family changed the project's ordinary Pool default", route)
	}
	t.Logf("ordered_release_dedicated_runs=%s,%s,%s default_pool=%s default_route_version=%d",
		view.Rollout.ParentRunID, library.RunID, web.RunID, defaultRoute.PoolID, defaultRoute.Version)
	receipt := map[string]any{"stage": "T-ROLLOUT-125", "release_set_id": p.registered.Set.ID, "release_set_sha256": p.registered.Set.SnapshotSHA256, "plan_group_id": p.group.Group.ID, "rollout_id": view.Rollout.ID, "parent_run_id": view.Rollout.ParentRunID, "candidate_commit": h.delivery.CandidateCommit, "paused_after_library": paused, "unknown_after_web": unknownView, "final": view, "provider_post_counts": postCounts, "model_calls": 0, "static_demo_only": true, "customer_human_signoff": false}
	t.Logf("application_release_rollout_actual_receipt=%s", stringify(receipt))
	actualApplicationRuntimeHold(t, f, h.delivery, h.manifests, receipt, actualApplicationHoldOptions{CacheDirectory: "rollout-125", DirectoryEnvironment: "SUMA_ROLLOUT_UI_DIR", WebEnvironment: "SUMA_ROLLOUT_WEB_DIR", Hostname: "rollout125.localhost", Kind: "actual_application_ordered_rollout", AdditionalTables: []string{"suma_application_release_sets", "suma_application_release_set_plan_groups", "suma_application_release_set_plan_bindings", "suma_application_release_rollouts", "suma_application_release_rollout_members", "suma_release_candidates", "suma_release_artifacts", "suma_artifact_proofs", "suma_release_plans", "suma_release_deployments", "suma_release_environments"}, ProtectedQueries: []string{`SELECT payload FROM suma_application_release_sets ORDER BY id`, `SELECT payload FROM suma_application_release_set_plan_groups ORDER BY id`, `SELECT payload FROM suma_application_release_rollouts ORDER BY id`, `SELECT data FROM suma_release_plans ORDER BY id`, `SELECT data FROM suma_release_deployments ORDER BY operation_id`, `SELECT data FROM suma_release_environments ORDER BY id`}})
}
