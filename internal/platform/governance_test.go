package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/store"
)

// These cases exercise real browser sessions and SQLite transactions. Forging a
// Principal or inserting fake users would miss the invitation and grant boundary.
func governanceInvite(f *fixture, email, role string, grants []any) map[string]any {
	f.t.Helper()
	return f.call("POST", "/organization/invitations", map[string]any{"email": email, "role": role, "grants": grants, "idempotency_key": newID()}, 201)
}

func governanceAccept(f *fixture, inv map[string]any, email string) map[string]any {
	f.t.Helper()
	f.cookie = nil
	return f.call("POST", "/auth/invitations/accept", map[string]any{"token": inv["token"], "email": email, "name": "受邀成员", "password": "invited-password-strong-42", "idempotency_key": newID()}, 200)
}

func TestGovernanceInvitationCreatesRealAccountAndProjectGrant(t *testing.T) {
	f := newFixture(t)
	ownerCookie := f.cookie
	inv := governanceInvite(f, "new@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "developer"}})
	if inv["token"] == nil || inv["version"] != float64(1) {
		t.Fatalf("missing one-time invitation: %v", inv)
	}
	list := f.call("GET", "/organization/invitations", nil, 200)
	raw, _ := json.Marshal(list)
	if strings.Contains(string(raw), inv["token"].(string)) || strings.Contains(string(raw), "token_hash") {
		t.Fatal("invitation listing leaked token material")
	}
	f.cookie = nil
	f.call("POST", "/auth/invitations/accept", map[string]any{"token": inv["token"], "email": "other@example.test", "name": "wrong", "password": "invited-password-strong-42", "idempotency_key": newID()}, 400)
	accepted := governanceAccept(f, inv, "new@example.test")
	user := accepted["user"].(map[string]any)
	if user["role"] != "member" || len(accepted["projects"].([]any)) != 1 {
		t.Fatalf("invitation did not materialize account and access: %v", accepted)
	}
	f.call("POST", "/tasks", f.body(map[string]any{"title": "真实受邀账号创建任务"}), 201)
	f.call("POST", "/auth/invitations/accept", map[string]any{"token": inv["token"], "email": "new@example.test", "name": "again", "password": "invited-password-strong-42", "idempotency_key": newID()}, 409)
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	f.call("POST", "/auth/login", map[string]any{"email": "new@example.test", "password": "invited-password-strong-42"}, 200)
	f.cookie = ownerCookie
	members := f.call("GET", "/organization/members", nil, 200)
	if members["total"] != float64(2) {
		t.Fatalf("real membership missing: %v", members)
	}
	var stored string
	if err := f.s.DB.QueryRow(`SELECT token_hash FROM suma_invitations WHERE id=?`, inv["id"]).Scan(&stored); err != nil || stored != digest(inv["token"].(string)) {
		t.Fatalf("token was not stored hashed: %v", err)
	}
}

func TestGovernanceInvitationReplayRevocationExpiryAndNoSecretReceipt(t *testing.T) {
	f := newFixture(t)
	body := map[string]any{"email": "replay@example.test", "role": "member", "grants": []any{}, "idempotency_key": newID()}
	inv := f.call("POST", "/organization/invitations", body, 201)
	again := f.call("POST", "/organization/invitations", body, 201)
	if again["id"] != inv["id"] || again["token"] != nil || again["token_revealed"] != false {
		t.Fatalf("replay reissued secret: %v", again)
	}
	var receipts string
	rows, err := f.s.DB.Query(`SELECT response FROM suma_operations`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var item string
		if err = rows.Scan(&item); err != nil {
			t.Fatal(err)
		}
		receipts += item
	}
	rows.Close()
	if strings.Contains(receipts, inv["token"].(string)) {
		t.Fatal("receipt leaked invitation secret")
	}
	f.call("POST", "/organization/invitations/"+inv["id"].(string)+"/revoke", map[string]any{"expected_version": 99, "idempotency_key": newID()}, 409)
	f.call("POST", "/organization/invitations/"+inv["id"].(string)+"/revoke", map[string]any{"expected_version": 1, "idempotency_key": newID()}, 200)
	f.cookie = nil
	f.call("POST", "/auth/invitations/accept", map[string]any{"token": inv["token"], "email": "replay@example.test", "name": "replay", "password": "invited-password-strong-42", "idempotency_key": newID()}, 409)
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "test-password-strong-42"}, 200)
	expired := governanceInvite(f, "expired@example.test", "member", nil)
	if _, err = f.s.DB.Exec(`UPDATE suma_invitations SET expires_at=? WHERE id=?`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano), expired["id"]); err != nil {
		t.Fatal(err)
	}
	f.cookie = nil
	f.call("POST", "/auth/invitations/accept", map[string]any{"token": expired["token"], "email": "expired@example.test", "name": "expired", "password": "invited-password-strong-42", "idempotency_key": newID()}, 409)
}

func TestGovernanceOrganizationSwitchExistingPasswordAndForeignProject(t *testing.T) {
	f := newFixture(t)
	ownerCookie := f.cookie
	created := f.call("POST", "/organizations", map[string]any{"name": "第二个组织", "idempotency_key": newID()}, 201)
	if created["id"] == f.user.TenantID {
		t.Fatal("organization create reused active organization")
	}
	f.call("POST", "/auth/switch", map[string]any{"organization_id": created["id"], "idempotency_key": newID()}, 200)
	secondProject := f.call("POST", "/projects", map[string]any{"name": "第二组织项目", "idempotency_key": newID()}, 201)
	f.call("POST", "/organization/invitations", map[string]any{"email": "foreign@example.test", "role": "member", "grants": []any{map[string]any{"project_id": f.project, "role": "admin"}}, "idempotency_key": newID()}, 400)
	inv := governanceInvite(f, "owner@example.test", "member", []any{map[string]any{"project_id": secondProject["id"], "role": "viewer"}})
	f.cookie = nil
	f.call("POST", "/auth/invitations/accept", map[string]any{"token": inv["token"], "email": "owner@example.test", "name": "replace owner", "password": "wrong-password-strong-42", "idempotency_key": newID()}, 401)
	f.call("POST", "/auth/invitations/accept", map[string]any{"token": inv["token"], "email": "owner@example.test", "name": "replace owner", "password": "test-password-strong-42", "idempotency_key": newID()}, 409)
	// Existing membership is preserved; it cannot be downgraded by invitation.
	f.cookie = ownerCookie
	f.call("POST", "/auth/switch", map[string]any{"organization_id": "foreign-org", "idempotency_key": newID()}, 403)
	orgs := f.call("GET", "/organizations", nil, 200)
	if orgs["total"] != float64(2) {
		t.Fatalf("organization memberships: %v", orgs)
	}
	if _, _, err := f.s.Login(context.Background(), "owner@example.test", "test-password-strong-42"); err != nil {
		t.Fatal("existing account password overwritten", err)
	}
}

func TestGovernanceOwnerProtectionRemovalRevokesSessionAndGrants(t *testing.T) {
	f := newFixture(t)
	ownerCookie := f.cookie
	f.call("PATCH", "/organization/members/"+f.user.ID, map[string]any{"role": "member", "expected_version": 1, "idempotency_key": newID()}, 409)
	inv := governanceInvite(f, "admin@example.test", "admin", []any{map[string]any{"project_id": f.project, "role": "admin"}})
	accepted := governanceAccept(f, inv, "admin@example.test")
	adminCookie := f.cookie
	adminID := accepted["user"].(map[string]any)["id"].(string)
	f.call("PATCH", "/organization/members/"+f.user.ID, map[string]any{"role": "member", "expected_version": 1, "idempotency_key": newID()}, 403)
	f.call("PATCH", "/organization/members/"+adminID, map[string]any{"role": "owner", "expected_version": 1, "idempotency_key": newID()}, 403)
	f.cookie = ownerCookie
	removed := f.call("PATCH", "/organization/members/"+adminID, map[string]any{"action": "remove", "expected_version": 1, "idempotency_key": newID()}, 200)
	if removed["status"] != "removed" {
		t.Fatalf("membership not removed: %v", removed)
	}
	f.cookie = adminCookie
	f.call("GET", "/auth/session", nil, 401)
	var count int
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_project_members WHERE user_id=?`, adminID).Scan(&count)
	if count != 0 {
		t.Fatal("membership removal left project grants")
	}
}

func TestGovernanceTeamsUseRealIdentitiesAndDoNotGrantAccess(t *testing.T) {
	f := newFixture(t)
	ownerCookie := f.cookie
	inv := governanceInvite(f, "team@example.test", "member", nil)
	accepted := governanceAccept(f, inv, "team@example.test")
	memberCookie := f.cookie
	memberID := accepted["user"].(map[string]any)["id"]
	f.cookie = ownerCookie
	team := f.call("POST", "/teams", map[string]any{"name": "研发组", "description": "真实身份", "idempotency_key": newID()}, 201)
	path := "/teams/" + team["id"].(string) + "/members"
	f.call("POST", path, map[string]any{"resource_kind": "human", "resource_id": "fake", "role": "member", "expected_version": 0, "idempotency_key": newID()}, 400)
	human := f.call("POST", path, map[string]any{"resource_kind": "human", "resource_id": memberID, "role": "member", "expected_version": 0, "idempotency_key": newID()}, 200)
	if human["name"] != "受邀成员" || human["version"] != float64(1) {
		t.Fatalf("stable identity missing: %v", human)
	}
	agent := f.call("POST", "/members", f.body(map[string]any{"title": "项目代理", "kind": "agent"}), 201)
	f.call("POST", path, map[string]any{"resource_kind": "tool", "resource_id": agent["id"], "project_id": f.project, "role": "member", "expected_version": 0, "idempotency_key": newID()}, 400)
	f.call("POST", path, map[string]any{"resource_kind": "agent", "resource_id": agent["id"], "project_id": f.project, "role": "member", "expected_version": 0, "idempotency_key": newID()}, 200)
	f.call("PATCH", "/teams/"+team["id"].(string), map[string]any{"name": "陈旧写入", "expected_version": 99, "idempotency_key": newID()}, 409)
	f.cookie = memberCookie
	f.call("GET", "/tasks?project_id="+f.project, nil, 403)
	f.call("POST", path, map[string]any{"resource_kind": "human", "resource_id": memberID, "role": "lead", "expected_version": 1, "idempotency_key": newID()}, 403)
}

func TestGovernanceProjectGrantAuthorityCASAndRealAccess(t *testing.T) {
	f := newFixture(t)
	ownerCookie := f.cookie
	inv := governanceInvite(f, "viewer@example.test", "member", nil)
	accepted := governanceAccept(f, inv, "viewer@example.test")
	memberCookie := f.cookie
	memberID := accepted["user"].(map[string]any)["id"]
	f.cookie = ownerCookie
	body := map[string]any{"project_id": f.project, "user_id": memberID, "role": "viewer", "expected_version": 0, "idempotency_key": newID()}
	grant := f.call("POST", "/project-access", body, 200)
	if grant["version"] != float64(1) {
		t.Fatalf("missing grant version %v", grant)
	}
	f.call("POST", "/project-access", body, 200)
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": memberID, "role": "admin", "expected_version": 0, "idempotency_key": newID()}, 409)
	f.cookie = memberCookie
	f.call("GET", "/tasks?project_id="+f.project, nil, 200)
	f.call("POST", "/tasks", f.body(map[string]any{"title": "viewer write"}), 403)
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": memberID, "role": "admin", "expected_version": 1, "idempotency_key": newID()}, 403)
	f.cookie = ownerCookie
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": memberID, "role": "developer", "expected_version": 1, "idempotency_key": newID()}, 200)
	f.cookie = memberCookie
	f.call("POST", "/tasks", f.body(map[string]any{"title": "developer write"}), 201)
	f.cookie = ownerCookie
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": memberID, "action": "remove", "expected_version": 2, "idempotency_key": newID()}, 200)
	f.cookie = memberCookie
	f.call("GET", "/tasks?project_id="+f.project, nil, 403)
}

func TestGovernanceScopedTokensCannotUseOrganizationNamespaces(t *testing.T) {
	f := newFixture(t)
	inv := governanceInvite(f, "scoped-accept@example.test", "member", nil)
	for _, kind := range []string{"api", "runner", "agent"} {
		key := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": kind}), 201)["token"].(string)
		for _, path := range []string{"/organizations", "/organization/members", "/organization/invitations", "/teams", "/project-access?project_id=" + f.project} {
			f.bearer("GET", path, key, nil, 403)
		}
		f.bearer("POST", "/auth/switch", key, map[string]any{"organization_id": f.user.TenantID, "idempotency_key": newID()}, 403)
		f.bearer("POST", "/auth/invitations/accept", key, map[string]any{"token": inv["token"], "email": "scoped-accept@example.test", "name": "禁止凭证治理", "password": "scoped-password-strong-42", "idempotency_key": newID()}, 403)
	}
}

func TestGovernanceConcurrentInvitationAcceptanceConsumesOnce(t *testing.T) {
	f := newFixture(t)
	inv := governanceInvite(f, "race@example.test", "member", nil)
	var sequence int
	var schema, path string
	if err := f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&sequence, &schema, &path); err != nil {
		t.Fatal(err)
	}
	other, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.Close() })
	services := []*Service{f.s, {DB: other, Dialect: "sqlite"}}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := services[i].AcceptInvitation(context.Background(), map[string]any{"token": inv["token"], "email": "race@example.test", "name": "并发", "password": "race-password-strong-42", "idempotency_key": fmt.Sprintf("race-operation-%d", i)})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else {
			var api *APIError
			if !errors.As(err, &api) || api.Status != 409 {
				t.Fatalf("concurrent acceptance returned non-conflict error: %v", err)
			}
		}
	}
	if success != 1 {
		t.Fatalf("one-time invitation accepted %d times", success)
	}
}

func TestGovernanceExistingAccountCanJoinAnotherOrganizationWithoutPasswordOverwrite(t *testing.T) {
	f := newFixture(t)
	ownerCookie := f.cookie
	first := governanceInvite(f, "existing@example.test", "member", nil)
	joined := governanceAccept(f, first, "existing@example.test")
	memberCookie := f.cookie
	memberID := joined["user"].(map[string]any)["id"]
	f.cookie = ownerCookie
	org := f.call("POST", "/organizations", map[string]any{"name": "受邀第二组织", "idempotency_key": newID()}, 201)
	f.call("POST", "/auth/switch", map[string]any{"organization_id": org["id"], "idempotency_key": newID()}, 200)
	project := f.call("POST", "/projects", map[string]any{"name": "权限隔离项目", "idempotency_key": newID()}, 201)
	inv := governanceInvite(f, "existing@example.test", "member", []any{map[string]any{"project_id": project["id"], "role": "viewer"}})
	f.cookie = nil
	f.call("POST", "/auth/invitations/accept", map[string]any{"token": inv["token"], "email": "existing@example.test", "name": "覆盖姓名尝试", "password": "wrong-password-strong-42", "idempotency_key": newID()}, 401)
	session := governanceAccept(f, inv, "existing@example.test")
	if session["user"].(map[string]any)["id"] != memberID || session["user"].(map[string]any)["name"] != "受邀成员" {
		t.Fatal("existing identity overwritten")
	}
	if len(session["projects"].([]any)) != 1 {
		t.Fatal("second organization project grant missing")
	}
	f.call("GET", "/tasks?project_id="+f.project, nil, 404)
	f.cookie = memberCookie
	f.call("GET", "/tasks?project_id="+project["id"].(string), nil, 404)
	f.call("POST", "/auth/switch", map[string]any{"organization_id": org["id"], "idempotency_key": newID()}, 200)
	f.call("GET", "/tasks?project_id="+project["id"].(string), nil, 200)
	var count int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_users WHERE email='existing@example.test'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("account duplicated: count %d %v", count, err)
	}
}

func TestGovernanceProjectAdminCanManageOnlyOwnProjects(t *testing.T) {
	f := newFixture(t)
	ownerCookie := f.cookie
	other := f.call("POST", "/projects", map[string]any{"name": "另一个受限项目", "idempotency_key": newID()}, 201)
	adminInvite := governanceInvite(f, "project-admin@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "admin"}})
	admin := governanceAccept(f, adminInvite, "project-admin@example.test")
	adminCookie := f.cookie
	adminID := admin["user"].(map[string]any)["id"]
	f.cookie = ownerCookie
	memberInvite := governanceInvite(f, "project-target@example.test", "member", nil)
	member := governanceAccept(f, memberInvite, "project-target@example.test")
	memberID := member["user"].(map[string]any)["id"]
	f.cookie = adminCookie
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": memberID, "role": "developer", "expected_version": 0, "idempotency_key": newID()}, 200)
	f.call("GET", "/project-access?project_id="+f.project, nil, 200)
	f.call("POST", "/project-access", map[string]any{"project_id": other["id"], "user_id": adminID, "role": "admin", "expected_version": 0, "idempotency_key": newID()}, 403)
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": f.user.ID, "role": "viewer", "expected_version": 0, "idempotency_key": newID()}, 403)
	f.call("POST", "/organization/invitations", map[string]any{"email": "forbidden@example.test", "role": "admin", "idempotency_key": newID()}, 403)
}

func TestGovernanceGrantTombstonePreventsStaleRegrantAndFractionalCAS(t *testing.T) {
	f := newFixture(t)
	ownerCookie := f.cookie
	inv := governanceInvite(f, "tombstone@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	member := governanceAccept(f, inv, "tombstone@example.test")
	memberID := member["user"].(map[string]any)["id"]
	f.cookie = ownerCookie
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": memberID, "role": "admin", "expected_version": 1.25, "idempotency_key": newID()}, 400)
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": memberID, "action": "remove", "expected_version": 1, "idempotency_key": newID()}, 200)
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": memberID, "role": "developer", "expected_version": 0, "idempotency_key": newID()}, 409)
	list := f.call("GET", "/project-access?project_id="+f.project, nil, 200)
	found := false
	for _, raw := range list["items"].([]any) {
		item := raw.(map[string]any)
		if item["user_id"] == memberID {
			found = true
			if item["status"] != "removed" || item["version"] != float64(2) {
				t.Fatalf("invalid tombstone %v", item)
			}
		}
	}
	if !found {
		t.Fatal("grant tombstone unavailable for CAS regrant")
	}
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": memberID, "role": "developer", "expected_version": 2, "idempotency_key": newID()}, 200)
}

func TestGovernanceOwnerTransferAndConcurrentDemotionRetainsOneOwner(t *testing.T) {
	f := newFixture(t)
	ownerCookie := f.cookie
	inv := governanceInvite(f, "co-owner@example.test", "member", nil)
	member := governanceAccept(f, inv, "co-owner@example.test")
	id := member["user"].(map[string]any)["id"].(string)
	f.cookie = ownerCookie
	f.call("PATCH", "/organization/members/"+id, map[string]any{"action": "transfer_owner", "role": "owner", "expected_version": 1, "idempotency_key": newID()}, 200)
	var sequence int
	var schema, path string
	if err := f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&sequence, &schema, &path); err != nil {
		t.Fatal(err)
	}
	other, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.Close() })
	services := []*Service{f.s, {DB: other, Dialect: "sqlite"}}
	principals := []Principal{f.user, {ID: id, TenantID: f.user.TenantID, Role: "owner"}}
	versions := []float64{1, 2}
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := services[i].UpdateOrganizationMember(context.Background(), principals[i], principals[i].ID, map[string]any{"role": "member", "expected_version": versions[i], "idempotency_key": newID()})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else {
			var api *APIError
			if !errors.As(err, &api) || api.Status != 409 {
				t.Fatalf("concurrent owner demotion returned non-conflict error: %v", err)
			}
		}
	}
	var owners int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_org_members WHERE tenant_id=? AND role='owner'`, f.user.TenantID).Scan(&owners); err != nil {
		t.Fatal(err)
	}
	if owners != 1 || success != 1 {
		t.Fatalf("concurrent owner demotion: successes=%d remaining=%d", success, owners)
	}
}

func TestGovernanceHumanResourceBindingRequiresRealProjectIdentity(t *testing.T) {
	f := newFixture(t)
	ownerCookie := f.cookie
	inv := governanceInvite(f, "human-binding@example.test", "member", nil)
	member := governanceAccept(f, inv, "human-binding@example.test")
	id := member["user"].(map[string]any)["id"].(string)
	f.cookie = ownerCookie
	check := func(actor Principal, user, project string, want bool) {
		t.Helper()
		tx, err := f.s.DB.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		err = f.s.BindHumanResourceTx(context.Background(), tx, actor, project, user)
		tx.Rollback()
		if (err == nil) != want {
			t.Fatalf("binding user=%s project=%s success=%v want=%v error=%v", user, project, err == nil, want, err)
		}
	}
	check(f.user, f.user.ID, f.project, true)
	check(f.user, "", f.project, false)
	check(f.user, "nonexistent-user", f.project, false)
	check(f.user, id, f.project, false)
	check(f.user, f.user.ID, "foreign-project", false)
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": id, "role": "viewer", "expected_version": 0, "idempotency_key": newID()}, 200)
	check(f.user, id, f.project, true)
	check(Principal{ID: id, TenantID: f.user.TenantID, Role: "owner"}, f.user.ID, f.project, false)
}

func TestGovernanceRemovedMembershipInvalidatesEarlierGrantTombstone(t *testing.T) {
	f := newFixture(t)
	ownerCookie := f.cookie
	inv := governanceInvite(f, "rejoin@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	member := governanceAccept(f, inv, "rejoin@example.test")
	id := member["user"].(map[string]any)["id"].(string)
	f.cookie = ownerCookie
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": id, "action": "remove", "expected_version": 1, "idempotency_key": newID()}, 200)
	f.call("PATCH", "/organization/members/"+id, map[string]any{"action": "remove", "expected_version": 1, "idempotency_key": newID()}, 200)
	rejoin := governanceInvite(f, "rejoin@example.test", "member", nil)
	governanceAccept(f, rejoin, "rejoin@example.test")
	f.cookie = ownerCookie
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": id, "role": "developer", "expected_version": 2, "idempotency_key": newID()}, 409)
}

func TestGovernanceAcceptanceReceiptDoesNotCreateFastPasswordVerifier(t *testing.T) {
	f := newFixture(t)
	inv := governanceInvite(f, "password-receipt@example.test", "member", nil)
	key := newID()
	password := "receipt-password-strong-42"
	f.cookie = nil
	session := f.call("POST", "/auth/invitations/accept", map[string]any{"token": inv["token"], "email": "password-receipt@example.test", "name": "回执成员", "password": password, "idempotency_key": key}, 200)
	user := session["user"].(map[string]any)["id"].(string)
	var stored string
	if err := f.s.DB.QueryRow(`SELECT request_hash FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`, f.user.TenantID, organizationScope, key).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	// A password-derived request digest plus public metadata would bypass the
	// account's expensive password KDF during an offline database attack.
	fastVerifier := digest(user + stringify(map[string]any{"action": "invitation.accept", "invitation_id": inv["id"], "email": "password-receipt@example.test", "name": "回执成员", "password_proof": digest(password)}))
	if stored == fastVerifier {
		t.Fatal("acceptance receipt created a fast offline password verifier")
	}
}
