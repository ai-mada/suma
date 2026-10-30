package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ai-mada/suma/internal/workbuddy"
)

func workBuddyHTTPFixture(t *testing.T) (*fixture, *WorkBuddyConnections, *atomic.Int32) {
	t.Helper()
	calls := &atomic.Int32{}
	f, component := workBuddyCommandFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/openapi/v2/token" || r.Method != http.MethodPost {
			t.Error("unexpected provider route")
		}
		workBuddyCommandResponse(w, "owned-open-id", "user.task.readable user.task.invokable", "private-refresh")
	})
	f.handler = (&Server{Service: f.s, Engine: f.e, WorkBuddy: component, AllowedOrigins: []string{"http://suma.test"}}).Handler()
	return f, component, calls
}

func workBuddyHTTPCallback(t *testing.T, f *fixture, state, code string, cookie, view bool) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(WorkBuddyCallbackRequest{State: state, Code: code})
	r := httptest.NewRequest(http.MethodPost, "http://suma.test/api/v1/workbuddy-oauth/callback", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://suma.test")
	if cookie {
		r.AddCookie(f.cookie)
	}
	if view {
		r.Header.Set("X-Suma-Expected-User", f.view.ID)
		r.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}

func TestWorkBuddyConnectionsHTTPStrictCookieBridgeAndOriginalPost(t *testing.T) {
	f, component, calls := workBuddyHTTPFixture(t)
	authorization, _ := workBuddyCommandStart(t, f, component)
	request := workBuddyCommandComplete(authorization)
	for _, diagnostic := range []string{fmt.Sprintf("%+v", request), fmt.Sprintf("%#v", WorkBuddyCallbackRequest{State: request.State, Code: request.Code})} {
		if strings.Contains(diagnostic, request.Code) || strings.Contains(diagnostic, request.State) {
			t.Fatal("callback request formatting exposed an ephemeral credential")
		}
	}
	if f.cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("the OAuth integration weakened the existing session cookie")
	}
	// Model the first cross-site redirect with no session cookie. The response
	// serves only a bootstrap and cannot consume a provider authorization code.
	u := "http://suma.test/api/v1/workbuddy-oauth/callback?" + url.Values{"code": {request.Code}, "state": {request.State}}.Encode()
	landing := httptest.NewRecorder()
	f.handler.ServeHTTP(landing, httptest.NewRequest(http.MethodGet, u, nil))
	body := landing.Body.String()
	if landing.Code != 200 || calls.Load() != 0 || landing.Header().Get("Referrer-Policy") != "no-referrer" || !strings.Contains(landing.Header().Get("Content-Security-Policy"), "default-src 'none'") || landing.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("unauthenticated callback GET did not stay an isolated read-only bridge", landing.Code)
	}
	if strings.Contains(body, request.Code) || strings.Contains(body, request.State) || strings.Contains(body, "localStorage") || strings.Contains(body, "sessionStorage") || strings.Contains(body, "postMessage") {
		t.Fatal("callback parameters were reflected or persisted")
	}
	clean, fetch := strings.Index(body, "history.replaceState"), strings.Index(body, "fetch(")
	if clean < 0 || fetch <= clean {
		t.Fatal("delivered bootstrap does not clean the callback URL before requests")
	}
	if got := workBuddyHTTPCallback(t, f, request.State, request.Code, false, true); got.Code != 401 {
		t.Fatal("callback POST accepted no current session", got.Code)
	}
	if got := workBuddyHTTPCallback(t, f, request.State, request.Code, true, false); got.Code != 428 {
		t.Fatal("callback POST bypassed the ordinary original-view guard", got.Code)
	}
	for attempt := 0; attempt < 2; attempt++ {
		response := workBuddyHTTPCallback(t, f, request.State, request.Code, true, true)
		var result WorkBuddyCallbackResult
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.ConnectionID != authorization.Connection.ID || result.ProjectID != f.project || result.State != "connected" || !strings.HasPrefix(result.RedirectURL, "/?") {
			t.Fatal("callback did not resolve the original safe connection", response.Code)
		}
		for _, secret := range []string{request.Code, request.State, "private-access", "private-refresh", "private-app-secret"} {
			if strings.Contains(response.Body.String(), secret) || strings.Contains(result.RedirectURL, secret) {
				t.Fatal("callback exposed provider credentials")
			}
		}
	}
	if calls.Load() != 1 {
		t.Fatal("callback replay issued another grant exchange", calls.Load())
	}
}

func TestWorkBuddyConnectionsHTTPClosedCommandsAndSessionReplacement(t *testing.T) {
	f, component, calls := workBuddyHTTPFixture(t)
	f.call("POST", "/workbuddy-authorizations", map[string]any{"project_id": f.project, "scopes": []string{"user.task.readable"}, "idempotency_key": newID(), "token_url": "https://untrusted.example/token"}, 400)
	f.call("POST", "/workbuddy-authorizations", map[string]any{"project_id": f.project, "scopes": []string{"user.task.readable"}, "idempotency_key": newID(), "expected_version": 0.5}, 400)
	f.call("GET", "/workbuddy-connections?project_id="+f.project+"&limit=01", nil, 400)
	f.call("GET", "/workbuddy-connections?project_id="+f.project+"&owner_id=another-user", nil, 400)
	authorization, _ := workBuddyCommandStart(t, f, component)
	request := workBuddyCommandComplete(authorization)
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "test-password-strong-42"}, 200)
	response := workBuddyHTTPCallback(t, f, request.State, request.Code, true, true)
	if response.Code != 403 || calls.Load() != 0 {
		t.Fatal("replacement browser session consumed the original nonce", response.Code, calls.Load())
	}
	page := f.call("GET", "/workbuddy-connections?project_id="+f.project, nil, 200)
	if page["configured"] != true || page["can_authorize"] != true || len(page["items"].([]any)) != 1 {
		t.Fatal("current owner could not reread safe connection metadata")
	}
	f.call("GET", "/workbuddy-authorizations/"+authorization.ID+"?project_id="+f.project, nil, 403)
}

func TestWorkBuddyConnectionsReadPrivatePagesAndCurrentRole(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	if err := f.s.InitializeWorkBuddyConnections(context.Background()); err != nil {
		t.Fatal(err)
	}
	component, err := NewWorkBuddyConnections(f.s, workbuddy.OAuthConfig{ClientID: "app-test", ClientSecret: "private-app-secret", RedirectURI: "https://suma.example/oauth", Scopes: []string{"user.task.readable", "user.task.invokable"}}, []byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	for count := 0; count < 3; count++ {
		workBuddyCommandStart(t, f, component)
	}
	first, err := component.List(context.Background(), f.actor, f.project, 2, "")
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatal("first owner page", err)
	}
	second, err := component.List(context.Background(), f.actor, f.project, 2, first.NextCursor)
	if err != nil || len(second.Items) != 1 || second.NextCursor != "" || second.Items[0].ID <= first.Items[1].ID {
		t.Fatal("stable owner pagination", err)
	}
	other, err := component.List(context.Background(), h.receiver, f.project, 2, "")
	if err != nil || len(other.Items) != 0 {
		t.Fatal("another current writer saw private connections", err)
	}
	_, err = component.Get(context.Background(), h.receiver, f.project, first.Items[0].ID)
	requireAuthorityStatus(t, err, 404)
	_, err = component.List(context.Background(), h.receiver, f.project, 2, first.NextCursor)
	requireAuthorityStatus(t, err, 400)
	if _, err = f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	other, err = component.List(context.Background(), h.receiver, f.project, 2, "")
	if err != nil || other.CanAuthorize {
		t.Fatal("current viewer retained authorize permission", err)
	}
	_, err = component.StartAuthorization(context.Background(), h.receiver, f.project, WorkBuddyAuthorizationRequest{ProjectID: f.project, Scopes: []string{"user.task.readable"}, IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 403)
}
