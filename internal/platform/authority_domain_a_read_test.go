package platform

import (
	"context"
	"testing"
)

func TestAuthorityDomainAReadsRejectDetachedRevokedCredential(t *testing.T) {
	for _, invalidation := range []string{"logout", "expiry"} {
		t.Run(invalidation, func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()
			team := f.call("POST", "/teams", map[string]any{"name": "private team", "expected_version": 0, "idempotency_key": "read-authority-team"}, 201)
			task := f.call("POST", "/tasks", f.body(map[string]any{"title": "private task"}), 201)
			baseline := f.call("POST", "/planning/baselines", f.body(map[string]any{"title": "private baseline"}), 201)
			actor := domainAActor(t, f.s, f.cookie.Value)
			reads := []struct {
				name string
				read func() error
			}{
				{"organizations", func() error { _, e := f.s.Organizations(ctx, actor); return e }},
				{"organization_members", func() error { _, e := f.s.OrganizationMembers(ctx, actor); return e }},
				{"invitations", func() error { _, e := f.s.Invitations(ctx, actor); return e }},
				{"teams", func() error { _, e := f.s.Teams(ctx, actor); return e }},
				{"team_members", func() error { _, e := f.s.TeamMembers(ctx, actor, team["id"].(string)); return e }},
				{"project_access", func() error { _, e := f.s.ProjectAccess(ctx, actor, f.project); return e }},
				{"records", func() error { _, e := f.s.List(ctx, actor, f.project, "tasks"); return e }},
				{"record", func() error {
					_, e := f.s.Get(ctx, actor, f.project, "tasks", task["id"].(string))
					return e
				}},
				{"record_page", func() error {
					_, _, _, e := f.s.ListPage(ctx, actor, f.project, "tasks", 10, "")
					return e
				}},
				{"overview", func() error { _, e := f.s.Overview(ctx, actor, f.project); return e }},
				{"periods", func() error { _, e := f.s.PlanningPeriods(ctx, actor, f.project, "iterations"); return e }},
				{"planning", func() error {
					_, e := f.s.Planning(ctx, actor, f.project, 10, "", "2026-10-19", task["id"].(string))
					return e
				}},
				{"capacity", func() error { _, e := f.s.PlanningCapacity(ctx, actor, f.project, "2026-10-19"); return e }},
				{"baselines", func() error { _, e := f.s.Baselines(ctx, actor, f.project); return e }},
				{"baseline", func() error {
					_, e := f.s.Baseline(ctx, actor, f.project, baseline["id"].(string), "", 10)
					return e
				}},
			}
			for _, read := range reads {
				if e := read.read(); e != nil {
					t.Fatalf("valid %s read failed: %v", read.name, e)
				}
			}
			if invalidation == "logout" {
				if e := logoutCredentialForTest(ctx, f.s, f.cookie.Value); e != nil {
					t.Fatal(e)
				}
			} else {
				for _, change := range []struct{ query, key string }{
					{`UPDATE suma_sessions SET expires_at='2000-01-01T00:00:00Z' WHERE token_hash=?`, actor.credential.digest},
					{`UPDATE suma_session_families SET expires_at='2000-01-01T00:00:00Z' WHERE id=?`, actor.credential.familyID},
					{`UPDATE suma_credential_slots SET expires_at='2000-01-01T00:00:00Z' WHERE id=?`, actor.credential.slotID},
				} {
					if _, e := f.s.DB.ExecContext(ctx, change.query, change.key); e != nil {
						t.Fatal(e)
					}
				}
			}
			for _, read := range reads {
				t.Run(read.name, func(t *testing.T) { requireAuthorityStatus(t, read.read(), 401) })
			}
		})
	}
}

func TestAuthorityDomainAReadKindsAndCurrentProjectRoles(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "scoped read task"}), 201)
	other := f.call("POST", "/projects", map[string]any{"title": "Other read scope", "idempotency_key": "other-read-project"}, 201)["id"].(string)
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, Label: "browser", IdempotencyKey: "domain-a-read-" + scope})
		if err != nil {
			t.Fatal(err)
		}
		actor := domainAActor(t, f.s, issued.Token)
		items, total, next, err := f.s.ListPage(ctx, actor, f.project, "tasks", 10, "")
		if scope == "api" {
			if err != nil || len(items) != 1 || total != 1 || next != "" || items[0].ID != task["id"] {
				t.Fatalf("API project read changed page: %v %d %q %v", items, total, next, err)
			}
			empty, err := f.s.List(ctx, actor, f.project, "issues")
			if err != nil || empty == nil || len(empty) != 0 {
				t.Fatalf("empty authorized list changed: %v %v", empty, err)
			}
			_, _, _, err = f.s.ListPage(ctx, actor, other, "tasks", 10, "")
			requireAuthorityStatus(t, err, 403)
		} else {
			requireAuthorityStatus(t, err, 403)
		}
		_, err = f.s.Teams(ctx, actor)
		requireAuthorityStatus(t, err, 403)
	}
	ownerCookie := f.cookie
	invit := governanceInvite(f, "read-viewer@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	accepted := governanceAccept(f, invit, "read-viewer@example.test")
	viewer := domainAActor(t, f.s, f.cookie.Value)
	if _, err := f.s.Get(ctx, viewer, f.project, "tasks", task["id"].(string)); err != nil {
		t.Fatal(err)
	}
	f.cookie = ownerCookie
	f.call("GET", "/auth/session", nil, 200)
	user := accepted["user"].(map[string]any)["id"].(string)
	if _, err := f.s.SaveProjectAccess(ctx, f.actor, map[string]any{"project_id": f.project, "user_id": user, "action": "remove", "expected_version": float64(1), "idempotency_key": "withdraw-read-viewer"}); err != nil {
		t.Fatal(err)
	}
	_, err := f.s.Get(ctx, viewer, f.project, "tasks", task["id"].(string))
	requireAuthorityStatus(t, err, 403)
}
