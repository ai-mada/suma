package platform

import "testing"

// Exercise the real HTTP session, invitations and versioned grant changes. A
// hand-built Session with projects[].role hid this missing projection in the UI.
func TestIdentitySessionCurrentProjectRoles(t *testing.T) {
	f := newFixture(t)
	ownerCookie := f.cookie
	projectRole := func(session map[string]any, project string) string {
		t.Helper()
		for _, raw := range session["projects"].([]any) {
			item := raw.(map[string]any)
			if item["id"] == project {
				role, _ := item["role"].(string)
				return role
			}
		}
		return ""
	}
	if role := projectRole(f.call("GET", "/auth/session", nil, 200), f.project); role != "admin" {
		t.Fatalf("owner effective project role: %q", role)
	}
	other := f.call("POST", "/projects", map[string]any{"name": "Not granted", "idempotency_key": newID()}, 201)
	inv := governanceInvite(f, "session-role@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "developer"}})
	member := governanceAccept(f, inv, "session-role@example.test")
	memberCookie := f.cookie
	memberID := member["user"].(map[string]any)["id"]
	if projectRole(member, f.project) != "developer" || projectRole(member, other["id"].(string)) != "" {
		t.Fatal("acceptance must project only the actual current grant")
	}
	for i, role := range []string{"reviewer", "viewer"} {
		f.cookie = ownerCookie
		f.call("GET", "/auth/session", nil, 200) // Select the owner's current view before writing.
		f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": memberID, "role": role, "expected_version": i + 1, "idempotency_key": newID()}, 200)
		f.cookie = memberCookie
		session := f.call("GET", "/auth/session", nil, 200)
		if projectRole(session, f.project) != role || session["user"].(map[string]any)["role"] != "member" {
			t.Fatalf("session did not reread current project role %q", role)
		}
	}
	f.cookie = ownerCookie
	f.call("GET", "/auth/session", nil, 200)
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": memberID, "action": "remove", "expected_version": 3, "idempotency_key": newID()}, 200)
	f.cookie = memberCookie
	if projectRole(f.call("GET", "/auth/session", nil, 200), f.project) != "" {
		t.Fatal("revoked project remained in the session")
	}
}
