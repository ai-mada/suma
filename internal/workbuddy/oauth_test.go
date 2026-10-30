package workbuddy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const oauthSecret = "private-client-secret"
const oauthAccess = "private-access-token"
const oauthRefresh = "private-refresh-token"
const oauthCallback = "https://suma.example/callback?binding=fixed%2Fvalue"

func oauthFixture(t *testing.T, handler http.HandlerFunc) (*OAuthClient, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, err := NewOAuthClient(OAuthConfig{BaseURL: server.URL, ClientID: "app-suma", ClientSecret: oauthSecret, RedirectURI: oauthCallback, Scopes: []string{"user.task.readable", "user.task.invokable"}, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal("valid OAuth fixture rejected:", err)
	}
	return c, server
}
func oauthResponse(w http.ResponseWriter, id, scope, extra string) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","open_id":%q,"scope":%q,"expires_in":3600%s}`, oauthAccess, id, scope, extra)
}
func TestOAuthAuthorizationExchangeAndExplicitRefresh(t *testing.T) {
	var calls atomic.Int32
	c, server := oauthFixture(t, func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if r.Method != "POST" || r.URL.RequestURI() != "/openapi/v2/token" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("unexpected route, auth style or cookie")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		want := url.Values{"client_id": {"app-suma"}, "client_secret": {oauthSecret}}
		switch n {
		case 1:
			want.Set("grant_type", "authorization_code")
			want.Set("code", "private-auth-code")
			want.Set("redirect_uri", oauthCallback)
		case 2, 3:
			want.Set("grant_type", "refresh_token")
			want.Set("refresh_token", oauthRefresh)
		default:
			t.Error("unexpected implicit call")
		}
		if !reflect.DeepEqual(r.PostForm, want) {
			t.Error("form differs from documented typed exchange")
		}
		extra := ""
		if n != 2 {
			extra = `,"refresh_token":"` + oauthRefresh + `"`
		}
		oauthResponse(w, "open-user-one", "user.task.readable", extra)
	})
	auth, err := c.AuthorizationURL("random-state-123")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	wantQuery := url.Values{"client_id": {"app-suma"}, "redirect_uri": {oauthCallback}, "response_type": {"code"}, "scope": {"user.task.readable user.task.invokable"}, "state": {"random-state-123"}}
	if u.Scheme+"://"+u.Host != server.URL || u.Path != "/openapi/v2/authorize" || !reflect.DeepEqual(u.Query(), wantQuery) || calls.Load() != 0 {
		t.Fatal("authorization URL does not preserve configured grant")
	}
	before := time.Now()
	grant, err := c.ExchangeCode(context.Background(), "private-auth-code")
	if err != nil || grant.AccessToken != oauthAccess || grant.RefreshToken != oauthRefresh || !grant.RefreshTokenPresent || grant.OpenID != "open-user-one" || !reflect.DeepEqual(grant.Scopes, []string{"user.task.readable"}) || grant.ExpiresAt.Before(before.Add(3599*time.Second)) || grant.ExpiresAt.After(time.Now().Add(3601*time.Second)) {
		t.Fatal("exchange failed exact grant projection", grant, err)
	}
	grant, err = c.Refresh(context.Background(), oauthRefresh)
	if err != nil || grant.RefreshTokenPresent || grant.RefreshToken != "" || grant.AccessToken != oauthAccess {
		t.Fatal("refresh absence was replaced by SDK inherited token", grant, err)
	}
	grant, err = c.Refresh(context.Background(), oauthRefresh)
	if err != nil || !grant.RefreshTokenPresent || grant.RefreshToken != oauthRefresh || calls.Load() != 3 {
		t.Fatal("explicit refresh did not preserve supplied token", grant, err)
	}
}

func TestOAuthRejectsMalformedGrantWithoutRetry(t *testing.T) {
	valid := `{"access_token":"private-access-token","token_type":"Bearer","open_id":"open-one","scope":"user.task.readable","expires_in":3600,"refresh_token":"private-refresh-token"}`
	cases := []struct {
		name, body string
		status     int
	}{
		{"duplicate", strings.Replace(valid, `"expires_in":3600`, `"expires_in":3600,"expires_in":1`, 1), 200},
		{"trailing", valid + ` {}`, 200},
		{"invalid_utf8", strings.Replace(valid, "open-one", string([]byte{0xff}), 1), 200},
		{"invalid_unicode", strings.Replace(valid, "open-one", `\ud800`, 1), 200},
		{"oversized", valid + strings.Repeat(" ", 1<<20), 200},
		{"missing_identity", strings.Replace(valid, `"open_id":"open-one",`, "", 1), 200},
		{"empty_identity", strings.Replace(valid, "open-one", "", 1), 200},
		{"null_identity", strings.Replace(valid, `"open-one"`, `null`, 1), 200},
		{"scope_escalation", strings.Replace(valid, "user.task.readable", "user.profile.readable", 1), 200},
		{"scope_missing", strings.Replace(valid, `"scope":"user.task.readable",`, "", 1), 200},
		{"scope_empty", strings.Replace(valid, "user.task.readable", "", 1), 200},
		{"scope_duplicate", strings.Replace(valid, "user.task.readable", "user.task.readable user.task.readable", 1), 200},
		{"wrong_token_type", strings.Replace(valid, "Bearer", "Basic", 1), 200},
		{"missing_token_type", strings.Replace(valid, `"token_type":"Bearer",`, "", 1), 200},
		{"bad_access", strings.Replace(valid, oauthAccess, "invalid access", 1), 200},
		{"padding_without_token", strings.Replace(valid, oauthAccess, "===", 1), 200},
		{"zero_expiry", strings.Replace(valid, "3600", "0", 1), 200},
		{"negative_expiry", strings.Replace(valid, "3600", "-1", 1), 200},
		{"fraction_expiry", strings.Replace(valid, "3600", "1.1", 1), 200},
		{"string_expiry", strings.Replace(valid, "3600", `"3600"`, 1), 200},
		{"overflow_expiry", strings.Replace(valid, "3600", "9223372036854775807", 1), 200},
		{"missing_refresh", strings.Replace(valid, `,"refresh_token":"private-refresh-token"`, "", 1), 200},
		{"empty_refresh", strings.Replace(valid, oauthRefresh, "", 1), 200},
		{"null_refresh", strings.Replace(valid, `"private-refresh-token"`, `null`, 1), 200},
		{"error_object", `{"error":"invalid_grant","error_description":"private-client-secret private-refresh-token"}`, 400},
		{"success_with_error", strings.Replace(valid, `"expires_in":3600`, `"error":"invalid_grant","expires_in":3600`, 1), 200},
		{"unexpected_success_status", valid, 201},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			c, _ := oauthFixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})
			grant, err := c.ExchangeCode(context.Background(), "private-auth-code")
			var e *Error
			if !errors.As(err, &e) || !e.OutcomeUnknown || calls.Load() != 1 || grant.AccessToken != "" {
				t.Fatal("invalid grant accepted or replayed", grant, err, calls.Load())
			}
			requireOAuthSafe(t, err)
		})
	}
}

func TestOAuthRejectsInvalidConfigAndInputBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		oauthResponse(w, "open-one", "user.task.readable", `,"refresh_token":"private-refresh-token"`)
	}))
	defer server.Close()
	base := OAuthConfig{BaseURL: server.URL, ClientID: "app-one", ClientSecret: oauthSecret, RedirectURI: oauthCallback, Scopes: []string{"user.task.readable"}}
	for _, mutate := range []func(*OAuthConfig){
		func(c *OAuthConfig) { c.BaseURL = "https://evil.example" },
		func(c *OAuthConfig) { c.BaseURL = "http://localhost:12345" },
		func(c *OAuthConfig) { c.BaseURL = "http://192.168.1.1" },
		func(c *OAuthConfig) { c.BaseURL = "https://www.workbuddy.cn/path" },
		func(c *OAuthConfig) { c.BaseURL = "https://www.workbuddy.cn?secret=x" },
		func(c *OAuthConfig) { c.BaseURL = "https://user@www.workbuddy.cn" },
		func(c *OAuthConfig) { c.ClientID = "" },
		func(c *OAuthConfig) { c.ClientSecret = "" },
		func(c *OAuthConfig) { c.ClientSecret = "secret\nvalue" },
		func(c *OAuthConfig) { c.RedirectURI = "http://suma.example/callback" },
		func(c *OAuthConfig) { c.RedirectURI = "https://user:secret@suma.example/callback" },
		func(c *OAuthConfig) { c.RedirectURI = "https://suma.example/callback#fragment" },
		func(c *OAuthConfig) { c.RedirectURI = "/callback" },
		func(c *OAuthConfig) { c.Scopes = nil },
		func(c *OAuthConfig) { c.Scopes = []string{"user.profile.readable"} },
		func(c *OAuthConfig) { c.Scopes = []string{"user.task.readable", "user.task.readable"} },
	} {
		cfg := base
		mutate(&cfg)
		if _, err := NewOAuthClient(cfg); err == nil {
			t.Error("invalid config accepted")
		}
	}
	c, err := NewOAuthClient(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "a\nb", strings.Repeat("a", 8193)} {
		if _, err := c.AuthorizationURL(value); err == nil {
			t.Error("invalid state accepted")
		}
		if _, err := c.ExchangeCode(context.Background(), value); err == nil {
			t.Error("invalid code accepted")
		}
		if _, err := c.Refresh(context.Background(), value); err == nil {
			t.Error("invalid refresh accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.ExchangeCode(ctx, "private-auth-code")
	requireError(t, err, "cancelled", false)
	_, err = c.Refresh(ctx, oauthRefresh)
	requireError(t, err, "cancelled", false)
	if calls.Load() != 0 {
		t.Fatal("local rejection made external request")
	}
	base.Scopes[0] = "user.task.invokable"
	auth, err := c.AuthorizationURL("state-one")
	if err != nil || !strings.Contains(auth, "user.task.readable") {
		t.Fatal("client scope changed via caller slice")
	}
	official := base
	official.BaseURL = ""
	official.Scopes = []string{"user.task.readable"}
	oc, err := NewOAuthClient(official)
	if err != nil {
		t.Fatal(err)
	}
	auth, err = oc.AuthorizationURL("state-two")
	if err != nil || !strings.HasPrefix(auth, "https://www.workbuddy.cn/openapi/v2/authorize?") {
		t.Fatal("wrong official endpoint")
	}
}

func TestOAuthRedirectCookiesAndPostTimeout(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	var sourceCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sourceCalls.Add(1)
		if r.Header.Get("Cookie") != "" {
			t.Error("ambient cookie leaked")
		}
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(server.URL)
	jar.SetCookies(u, []*http.Cookie{{Name: "private-session", Value: "private-cookie"}})
	supplied := server.Client()
	supplied.Jar = jar
	supplied.CheckRedirect = func(*http.Request, []*http.Request) error { t.Error("caller redirect handler used"); return nil }
	c, err := NewOAuthClient(OAuthConfig{BaseURL: server.URL, ClientID: "app-one", ClientSecret: oauthSecret, RedirectURI: oauthCallback, Scopes: []string{"user.task.readable"}, HTTPClient: supplied})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.ExchangeCode(context.Background(), "private-auth-code")
	requireError(t, err, "http_status", true)
	if sourceCalls.Load() != 1 || targetCalls.Load() != 0 || supplied.Jar != jar {
		t.Fatal("redirect leaked exchange or input client changed")
	}
	var timed atomic.Int32
	slow, _ := oauthFixture(t, func(w http.ResponseWriter, r *http.Request) {
		timed.Add(1)
		time.Sleep(70 * time.Millisecond)
		oauthResponse(w, "open-one", "user.task.readable", `,"refresh_token":"private-refresh-token"`)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err = slow.Refresh(ctx, oauthRefresh)
	var e *Error
	if !errors.As(err, &e) || !e.OutcomeUnknown || timed.Load() != 1 {
		t.Fatal("sent timeout was not unknown", err, timed.Load())
	}
}

func TestOAuthConcurrentExchangesKeepGrantIsolation(t *testing.T) {
	var calls atomic.Int32
	c, _ := oauthFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		r.ParseForm()
		id := r.Form.Get("code")
		oauthResponse(w, id, "user.task.readable", `,"refresh_token":"refresh-`+id+`"`)
	})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("identity-%d", i)
			grant, err := c.ExchangeCode(context.Background(), id)
			if err != nil || grant.OpenID != id || grant.RefreshToken != "refresh-"+id {
				t.Errorf("concurrent grant crossed identity: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if calls.Load() != 16 {
		t.Fatal("unexpected concurrent resend", calls.Load())
	}
}

func requireOAuthSafe(t *testing.T, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{string(raw), fmt.Sprintf("%v", value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value)} {
		for _, secret := range []string{oauthSecret, oauthAccess, oauthRefresh, "private-auth-code", "private-cookie"} {
			if strings.Contains(text, secret) {
				t.Fatal("OAuth diagnostic leaked secret")
			}
		}
	}
}
func TestOAuthGrantConfigAndErrorDiagnosticsAreSecretSafe(t *testing.T) {
	c, _ := oauthFixture(t, func(w http.ResponseWriter, r *http.Request) {
		oauthResponse(w, "open-one", "user.task.readable", `,"refresh_token":"private-refresh-token","private":"private-client-secret"`)
	})
	grant, err := c.ExchangeCode(context.Background(), "private-auth-code")
	if err != nil {
		t.Fatal(err)
	}
	requireOAuthSafe(t, grant)
	requireOAuthSafe(t, &grant)
	requireOAuthSafe(t, c)
	cfg := OAuthConfig{ClientSecret: oauthSecret}
	requireOAuthSafe(t, cfg)
	requireOAuthSafe(t, &cfg)
}

func TestOAuthRefreshAndRequestedScopeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, scope, extra, content string
		good                        bool
	}{
		{"missing_refresh_is_preserved", "user.task.readable", "", "application/json", true},
		{"invalid_present_refresh", "user.task.readable", `,"refresh_token":null`, "application/json", false},
		{"requested_scope_ceiling", "user.task.readable user.task.invokable", "", "application/json", false},
		{"non_json_token_response", "user.task.readable", "", "text/plain", false},
		{"case_variant_does_not_override", "user.task.readable", `,"ACCESS_TOKEN":"attacker-token","OPEN_ID":"other-identity"`, "application/json", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", tc.content)
				fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","open_id":"exact-identity","scope":%q,"expires_in":3600%s}`, oauthAccess, tc.scope, tc.extra)
			}))
			defer server.Close()
			c, err := NewOAuthClient(OAuthConfig{BaseURL: server.URL, ClientID: "app-one", ClientSecret: oauthSecret, RedirectURI: oauthCallback, Scopes: []string{"user.task.readable"}})
			if err != nil {
				t.Fatal(err)
			}
			grant, err := c.Refresh(context.Background(), oauthRefresh)
			if tc.good {
				if err != nil || grant.AccessToken != oauthAccess || grant.OpenID != "exact-identity" || grant.RefreshTokenPresent || grant.RefreshToken != "" {
					t.Fatal("refresh grant was overwritten", grant, err)
				}
			} else {
				requireError(t, err, "invalid_response", true)
			}
			if calls.Load() != 1 {
				t.Fatal("refresh was retried")
			}
		})
	}
}
