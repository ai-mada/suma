package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func notificationPreferencesHTTP(t *testing.T, f *fixture, method, path, body, token string, want int) map[string]any {
	t.Helper()
	r := httptest.NewRequest(method, "http://suma.test/api/v1"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token == "" {
		r.AddCookie(f.cookie)
		r.Header.Set("X-Suma-Expected-User", f.view.ID)
		r.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
	} else {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s => %d want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("preferences response may be cached")
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestProjectNotificationPreferencesHTTPClosedBodyAndScope(t *testing.T) {
	f := newFixture(t)
	path := "/notification-preferences?project_id=" + f.project
	commands := "/notification-preferences/commands?project_id=" + f.project
	before := domainACount(t, f.s, "suma_operations")
	initial := notificationPreferencesHTTP(t, f, http.MethodGet, path, "", "", 200)
	if initial["version"] != float64(0) || initial["followed_discussions_muted"] != false || initial["updated_at"] != "" || domainACount(t, f.s, "suma_operations") != before {
		t.Fatal("HTTP GET not pure default", initial)
	}
	req := ProjectNotificationPreferencesCommand{ProjectID: f.project, ExpectedVersion: 0, FollowedDiscussionsMuted: false, IdempotencyKey: "preferences-http167"}
	raw := stringify(req)
	for _, candidate := range []string{
		strings.TrimSuffix(raw, "}") + `,"user_id":"someone-else"}`,
		strings.TrimSuffix(raw, "}") + `,"followed_discussions_muted":true}`,
		strings.Replace(raw, `"expected_version":0`, `"expected_version":null`, 1),
		strings.Replace(raw, `"expected_version":0`, `"expected_version":0.0`, 1),
		strings.Replace(raw, `"expected_version":0`, `"expected_version":2147483648`, 1),
		strings.Replace(raw, `"expected_version":0,`, "", 1),
		strings.Replace(raw, `"followed_discussions_muted":false`, `"followed_discussions_muted":null`, 1),
		strings.Replace(raw, `"followed_discussions_muted":false,`, "", 1),
		strings.Replace(raw, `"project_id":"`+f.project+`"`, `"project_id":"different-project"`, 1),
		strings.Replace(raw, `"project_id"`, `"Project_ID"`, 1),
		strings.Replace(raw, `"preferences-http167"`, `"short"`, 1),
		strings.Replace(raw, `"preferences-http167"`, `"control\u0000key"`, 1),
		strings.Repeat(" ", 16<<10) + raw,
	} {
		notificationPreferencesHTTP(t, f, http.MethodPost, commands, candidate, "", 400)
	}
	for _, query := range []string{"", "?project_id=" + f.project + "&project_id=" + f.project, "?project_id=" + f.project + "&user_id=other", "?project_id=.."} {
		notificationPreferencesHTTP(t, f, http.MethodGet, "/notification-preferences"+query, "", "", 400)
	}
	if domainACount(t, f.s, "suma_operations") != before || domainACount(t, f.s, projectNotificationPreferencesTable) != 0 {
		t.Fatal("invalid command wrote state")
	}
	ack := notificationPreferencesHTTP(t, f, http.MethodPost, commands, raw, "", 200)
	if ack["preferences"].(map[string]any)["version"] != float64(0) || domainACount(t, f.s, projectNotificationPreferencesTable) != 0 {
		t.Fatal("false/zero presence rejected or noop materialized")
	}
	notificationPreferencesHTTP(t, f, http.MethodPost, path, raw, "", 405)
	notificationPreferencesHTTP(t, f, http.MethodGet, commands, "", "", 405)
	issued, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	notificationPreferencesHTTP(t, f, http.MethodGet, path, "", issued.Token, 200)
	notificationPreferencesHTTP(t, f, http.MethodGet, "/notification-preferences?project_id=other-project", "", issued.Token, 403)
}
