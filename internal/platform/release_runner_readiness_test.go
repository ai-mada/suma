package platform

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	rc "github.com/ai-mada/suma/internal/releasecontract"
	"github.com/ai-mada/suma/internal/runner"
)

func TestReleaseRunnerReadinessUsesCurrentDedicatedClaimAndDoesNotGateDeployment(t *testing.T) {
	f, candidate, env, plan, builderToken, _ := verifiedReleaseDeploymentPlan(t)
	const workerID, providerID = "readiness-deployer", "local-static-test"
	const tokenEnv = "SUMA_RELEASE_READINESS_WORKER_TOKEN"
	t.Setenv(tokenEnv, "fixture-transport-token")
	configured := runner.ExternalSpec{URL: "http://127.0.0.1:1", TokenEnv: tokenEnv, Protocol: rc.ReleaseProtocol}
	legacy := configured
	legacy.Protocol = ""
	unconfigured := configured
	unconfigured.TokenEnv = tokenEnv + "_EMPTY"
	t.Setenv(unconfigured.TokenEnv, "")
	insecure := configured
	insecure.URL = "http://not-loopback.example.test"
	otherCaps := runner.ExternalCapabilities(map[string]runner.ExternalSpec{providerID: legacy, "other-static": configured})
	matchingCaps := runner.ExternalCapabilities(map[string]runner.ExternalSpec{
		providerID: configured, "missing-token": unconfigured, "insecure-url": insecure,
	})
	if !slices.Equal(otherCaps, []string{"external_workflow:local-static-test", "external_workflow:other-static", "release_provider:other-static"}) ||
		!slices.Equal(matchingCaps, []string{"external_workflow:local-static-test", "release_provider:local-static-test"}) {
		t.Fatal("Runner must distinguish release protocol support from a generic external workflow capability")
	}

	path := "/release-plans/" + plan.ID + "?project_id=" + f.project
	previewBody := map[string]any{
		"project_id": f.project, "candidate_id": candidate.ID, "environment_id": env.ID,
		"action": "deploy", "expected_environment_generation": 0,
	}
	readiness := func(view map[string]any, state string, count int) map[string]any {
		t.Helper()
		hint, ok := view["execution_readiness"].(map[string]any)
		if !ok || hint["state"] != state || hint["registered_count"] != float64(count) {
			t.Fatalf("want readiness %s and %d dedicated registrations, got %v", state, count, hint)
		}
		if checkedAt, _ := hint["checked_at"].(string); checkedAt == "" {
			t.Fatal("readiness must identify when it was evaluated")
		}
		return hint
	}
	detail := func(state string, count int) map[string]any {
		t.Helper()
		return readiness(f.call("GET", path, nil, 200), state, count)
	}
	claim := func(token, worker string, caps []string, operation string, status int) map[string]any {
		t.Helper()
		return f.bearer("POST", "/runner/claim", token, map[string]any{
			"project_id": f.project, "worker_id": worker, "capabilities": caps, "idempotency_key": operation,
		}, status)
	}
	age := func(registration any) {
		t.Helper()
		past := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339Nano)
		result, err := f.s.DB.Exec(`UPDATE suma_release_runner_claims SET observed_at=? WHERE worker_registration_id=?`, past, registration)
		if err != nil {
			t.Fatal(err)
		}
		if n, err := result.RowsAffected(); n != 1 || err != nil {
			t.Fatalf("could not age the actual claim observation: %d, %v", n, err)
		}
	}

	detail("not_registered", 0)
	before := f.call("POST", "/release-plans/preview", previewBody, 200)
	readiness(before, "not_registered", 0)
	if before["eligibility"] != true {
		t.Fatal("a missing Runner must leave the valid deployment plan eligible")
	}

	pool, grant := dispatchAuthorizedPool(t, f)
	dispatchSetRoute(t, f, testProjectExecutionRoute(t, f).Version, &grant)
	machine, err := f.s.RegisterExecutionPoolRunner(context.Background(), f.actor, pool.ID, ExecutionPoolRunnerCreateRequest{
		Name: "Release readiness isolation", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	poolClaim := func() {
		t.Helper()
		out := f.bearer("POST", "/pool-runner/claim", machine.Token, runner.PoolClaimRequest{
			PoolID: pool.ID, RunnerID: machine.Runner.ID, WorkerID: machine.Runner.WorkerID, SlotID: "0", OperationID: newID(),
			Scopes: []runner.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: matchingCaps}},
		}, 200)
		if out["assignment"] != nil {
			t.Fatal("a shared Pool poll received a dedicated release assignment")
		}
	}
	registered := f.call("POST", "/release-workers", f.body(map[string]any{
		"worker_id": workerID, "profiles": []any{}, "deployment": true,
	}), 201)
	oldToken := registered["token"].(string)
	claim(builderToken, "release-fixture-worker", matchingCaps, newID(), 200)
	claim(oldToken, "wrong-readiness-worker", matchingCaps, newID(), 403)
	poolClaim()
	detail("unobserved", 1)

	claim(oldToken, workerID, otherCaps, newID(), 200)
	detail("capability_missing", 1)
	age(registered["id"])
	matchOperation := newID()
	claim(oldToken, workerID, matchingCaps, matchOperation, 200)
	detail("capability_observed", 1)
	after := f.call("POST", "/release-plans/preview", previewBody, 200)
	readiness(after, "capability_observed", 1)
	if after["preview_sha256"] != before["preview_sha256"] || after["eligibility"] != true {
		t.Fatal("a Runner claim changed the authoritative release preview")
	}

	// A valid project viewer may read the advisory, while the same user has no access to a separate project.
	otherProject := f.call("POST", "/projects", map[string]any{"name": "Unrelated project", "idempotency_key": newID()}, 201)
	ownerCookie := f.cookie
	invite := governanceInvite(f, "release-readiness-viewer@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	governanceAccept(f, invite, "release-readiness-viewer@example.test")
	detail("capability_observed", 1)
	f.call("GET", "/release-plans/"+plan.ID+"?project_id="+otherProject["id"].(string), nil, 403)
	f.cookie = ownerCookie
	f.call("GET", "/auth/session", nil, 200)
	f.bearer("GET", path, oldToken, nil, 403)

	age(registered["id"])
	stale := detail("stale", 1)
	claim(oldToken, workerID, matchingCaps, matchOperation, 200)
	if replayed := detail("stale", 1); replayed["observed_at"] != stale["observed_at"] {
		t.Fatal("replaying the original claim refreshed expired evidence")
	}
	rotated := f.call("POST", manualRotationPath(registered["id"]), f.body(map[string]any{
		"expected_version": registered["version"], "acknowledge_stopped": true,
	}), 200)
	newToken := rotated["token"].(string)
	claim(oldToken, workerID, matchingCaps, newID(), 401)
	detail("unobserved", 1)

	executePath := "/release-plans/" + plan.ID + "/execute"
	executeBody := releaseExecuteBody(f, plan)
	queued := f.call("POST", executePath, executeBody, 201)
	if queued["state"] != "queued" || queued["effect_state"] != "not_started" || queued["assignment"] != nil {
		t.Fatal("an offline deployment did not stay queued without effects")
	}
	assertDedicatedReleaseRun(t, context.Background(), f, queued["run_id"].(string))
	poolClaim()
	detail("unobserved", 1)
	assignment := claim(newToken, workerID, matchingCaps, newID(), 200)["assignment"]
	if assignment, ok := assignment.(map[string]any); !ok || assignment["run_id"] != queued["run_id"] {
		t.Fatal("current dedicated Runner could not claim the queued deployment")
	}
	detail("capability_observed", 1)
	if replayed := f.call("POST", executePath, executeBody, 201); !reflect.DeepEqual(queued, replayed) {
		t.Fatal("readiness and claim changes altered the original execution receipt")
	}
}
