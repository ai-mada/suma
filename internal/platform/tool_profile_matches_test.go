package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/ai-mada/suma/internal/toolcontract"
)

func toolProfileReuseSeed(t *testing.T, f *fixture, id, project, worker, state string, profile toolcontract.Profile) {
	t.Helper()
	_, err := f.s.DB.Exec(`INSERT INTO suma_tool_profiles(id,tenant_id,project_id,worker_id,name,version,state,profile,enabled_by,created_at,updated_at)VALUES(?,?,?,?,?,1,?,?,?,?,?)`, id, f.user.TenantID, project, worker, "Independent display "+id, state, stringify(profile), f.user.ID, now(), now())
	if err != nil {
		t.Fatal(err)
	}
}

func TestToolProfileReuseCompleteExactMatchesIgnoreNamesAndAvailability(t *testing.T) {
	f, _, original := toolFixture(t)
	profile := original.Profile
	profile.ReadOnly = true
	profile.Planning = &toolcontract.PlanningCapability{Protocol: toolcontract.PlanningProtocol, Model: "fixed-model", EndpointOrigin: "https://models.example"}
	for i := 0; i < 205; i++ {
		toolProfileReuseSeed(t, f, fmt.Sprintf("a-noise-%03d", i), f.project, original.WorkerID, "paused", original.Profile)
	}
	for _, state := range []string{"paused", "enabled", "revoked"} {
		toolProfileReuseSeed(t, f, "z-match-"+state, f.project, original.WorkerID, state, toolcontract.CloneProfile(profile))
	}
	different := toolcontract.CloneProfile(profile)
	different.Planning.Model = "other-model"
	toolProfileReuseSeed(t, f, "z-other-model", f.project, original.WorkerID, "paused", different)
	toolProfileReuseSeed(t, f, "z-other-worker", f.project, "other-worker", "paused", profile)
	toolProfileReuseSeed(t, f, "z-other-project", newID(), original.WorkerID, "paused", profile)
	if _, err := f.s.DB.Exec(`UPDATE suma_release_workers SET status='revoked' WHERE project_id=? AND worker_id=?`, f.project, original.WorkerID); err != nil {
		t.Fatal(err)
	}
	beforeProfiles, beforeReceipts := domainACount(t, f.s, "suma_tool_profiles"), domainACount(t, f.s, "suma_operations")
	request := ToolProfileMatchRequest{ProjectID: f.project, WorkerID: original.WorkerID, Profile: profile}
	value := f.call("POST", "/tool-profiles/matches", request, 200)
	raw, _ := json.Marshal(value)
	var matches ToolProfileMatchList
	if err := json.Unmarshal(raw, &matches); err != nil {
		t.Fatal(err)
	}
	if matches.ProjectID != f.project || matches.WorkerID != original.WorkerID || !toolcontract.EqualProfile(matches.Profile, profile) || matches.Total != 3 || len(matches.Items) != 3 {
		t.Fatalf("incomplete or inexact set: %+v", matches)
	}
	states := map[string]bool{}
	for _, item := range matches.Items {
		if item.WorkerID != request.WorkerID || item.ProjectID != request.ProjectID || !toolcontract.EqualProfile(item.Profile, profile) || item.Available {
			t.Fatalf("match became a grant or escaped predicate: %+v", item)
		}
		states[item.State] = true
	}
	if len(states) != 3 || domainACount(t, f.s, "suma_tool_profiles") != beforeProfiles || domainACount(t, f.s, "suma_operations") != beforeReceipts {
		t.Fatal("match lookup dropped a state or wrote business data")
	}
}

func TestToolProfileReuseRequiresCurrentReadAndClosedBody(t *testing.T) {
	f, token, original := toolFixture(t)
	request := ToolProfileMatchRequest{ProjectID: f.project, WorkerID: original.WorkerID, Profile: original.Profile}
	for _, tokenScope := range []string{"api", "agent", "runner"} {
		t.Run(tokenScope, func(t *testing.T) {
			key := token
			if tokenScope != "runner" {
				key = f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": tokenScope}), 201)["token"].(string)
			}
			f.bearer("POST", "/tool-profiles/matches", key, request, 403)
		})
	}
	t.Run("closed", func(t *testing.T) {
		f.call("POST", "/tool-profiles/matches", map[string]any{"project_id": f.project, "worker_id": original.WorkerID, "profile": original.Profile, "idempotency_key": newID()}, 400)
	})
	t.Run("null_boolean", func(t *testing.T) {
		var profile map[string]any
		_ = json.Unmarshal([]byte(stringify(original.Profile)), &profile)
		profile["read_only"] = nil
		f.call("POST", "/tool-profiles/matches", map[string]any{"project_id": f.project, "worker_id": original.WorkerID, "profile": profile}, 400)
	})
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role)VALUES(?,?,'developer')`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/tool-profiles/matches", request, 200)
	if _, err := f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/tool-profiles/matches", request, 403)
}

func TestToolProfileReuseCreateReceiptPrecedesDuplicateAndRevocation(t *testing.T) {
	f, _, original := toolFixture(t)
	ctx := context.Background()
	request := ToolProfileCreateRequest{ProjectID: f.project, WorkerID: original.WorkerID, Name: "New exact profile", Profile: original.Profile, IdempotencyKey: newID()}
	request.Profile.ToolVersion = "reuse receipt fixture"
	created, err := f.s.CreateToolProfile(ctx, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	other := request
	other.Name, other.IdempotencyKey = "Different name cannot duplicate identity", newID()
	_, err = f.s.CreateToolProfile(ctx, f.actor, f.project, other)
	var api *APIError
	if !errors.As(err, &api) || api.Status != 409 || api.Code != "tool_profile_match_exists" {
		t.Fatalf("duplicate create did not conflict: %v", err)
	}
	if _, err = f.s.CommandToolProfile(ctx, f.actor, f.project, created.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "revoke", ExpectedVersion: created.Version, IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	replacement, err := f.s.CreateToolProfile(ctx, f.actor, f.project, other)
	if err != nil || replacement.ID == created.ID || replacement.State != "paused" {
		t.Fatal("explicit replacement of revoked history failed", replacement, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_release_workers SET status='revoked' WHERE project_id=? AND worker_id=?`, f.project, original.WorkerID); err != nil {
		t.Fatal(err)
	}
	replayed, err := f.s.CreateToolProfile(ctx, f.actor, f.project, request)
	if err != nil || replayed.ID != created.ID || replayed.Version != 1 || replayed.State != "paused" {
		t.Fatal("original receipt was replaced by current match/state", replayed, err)
	}
	if domainACount(t, f.s, "suma_tool_profiles") != 3 {
		t.Fatal("duplicate attempt wrote another profile")
	}
}

func TestToolProfileReuseConcurrentCreatesSerializeExactIdentity(t *testing.T) {
	f, _, original := toolFixture(t)
	profile := original.Profile
	profile.ToolVersion = "concurrent exact identity"
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			_, err := f.s.CreateToolProfile(context.Background(), f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: original.WorkerID, Name: fmt.Sprintf("Caller %d", i), Profile: profile, IdempotencyKey: newID()})
			results <- err
		}(i)
	}
	close(start)
	workers.Wait()
	close(results)
	created, conflicts := 0, 0
	for err := range results {
		if err == nil {
			created++
			continue
		}
		var api *APIError
		if errors.As(err, &api) && api.Status == 409 && api.Code == "tool_profile_match_exists" {
			conflicts++
		} else {
			t.Fatalf("unexpected race error: %v", err)
		}
	}
	if created != 1 || conflicts != 1 || domainACount(t, f.s, "suma_tool_profiles") != 2 {
		t.Fatalf("concurrent duplicate persisted: created=%d conflicts=%d", created, conflicts)
	}
}
