package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/store"
)

// These requests cross a real loopback HTTP connection and the SQLite-backed
// handler, so cookie identity, strict JSON and committed replay are exercised.
type preferenceHTTP struct {
	t                  *testing.T
	server             *httptest.Server
	inferExpectedScope bool
}

func newPreferenceHTTP(t *testing.T, handler http.Handler) *preferenceHTTP {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &preferenceHTTP{t: t, server: server, inferExpectedScope: true}
}

func (h *preferenceHTTP) request(method, path, body string, cookie *http.Cookie, headers map[string]string) (int, http.Header, map[string]any) {
	h.t.Helper()
	req, err := http.NewRequest(method, h.server.URL+"/api/v1"+path, strings.NewReader(body))
	if err != nil {
		h.t.Fatal(err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	// Existing fixture calls bind writes to their known cookie identity. Tests of
	// shared-cookie drift disable this and submit the original view's headers.
	if h.inferExpectedScope && method == "PATCH" && (path == "/account/preferences" || path == "/organization/preferences") {
		_, _, session := h.request("GET", "/auth/session", "", cookie, headers)
		if user, ok := session["user"].(map[string]any); ok {
			req.Header.Set("X-Suma-Expected-User", user["id"].(string))
		}
		if path == "/organization/preferences" {
			if organization, ok := session["organization"].(map[string]any); ok {
				req.Header.Set("X-Suma-Expected-Organization", organization["id"].(string))
			}
		}
	}
	response, err := h.server.Client().Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	var result map[string]any
	if err = json.Unmarshal(raw, &result); err != nil {
		h.t.Fatalf("invalid response %s: %v", raw, err)
	}
	return response.StatusCode, response.Header, result
}

func (h *preferenceHTTP) expect(method, path, body string, cookie *http.Cookie, want int) map[string]any {
	h.t.Helper()
	status, _, result := h.request(method, path, body, cookie, nil)
	if status != want {
		h.t.Fatalf("%s %s => %d, want %d: %v", method, path, status, want, result)
	}
	return result
}

func preferenceBody(locale string, version int, key string) string {
	return stringify(map[string]any{"locale": locale, "expected_version": version, "idempotency_key": key})
}

func TestLocalePreferencesPersistenceInheritanceAndScopedReplay(t *testing.T) {
	f := newFixture(t)
	h := newPreferenceHTTP(t, f.handler)
	for _, path := range []string{"/account/preferences", "/organization/preferences"} {
		initial := h.expect("GET", path, "", f.cookie, 200)
		if initial["locale"] != "" || initial["version"] != float64(0) || initial["updated_at"] != "" {
			t.Fatalf("unset preference: %v", initial)
		}
	}
	accountBody := preferenceBody("en_GB", 0, "account-global-key")
	first := h.expect("PATCH", "/account/preferences", accountBody, f.cookie, 200)
	if first["locale"] != "en-US" || first["user_id"] != f.user.ID || first["version"] != float64(1) {
		t.Fatalf("canonical account: %v", first)
	}
	if _, err := time.Parse(time.RFC3339Nano, first["updated_at"].(string)); err != nil {
		t.Fatal(err)
	}
	replay := h.expect("PATCH", "/account/preferences", accountBody, f.cookie, 200)
	if stringify(first) != stringify(replay) {
		t.Fatalf("replay changed: %v %v", first, replay)
	}
	h.expect("PATCH", "/account/preferences", preferenceBody("en-US", 0, "account-global-key"), f.cookie, 409)
	h.expect("PATCH", "/account/preferences", preferenceBody("zh-CN", 0, newID()), f.cookie, 409)
	org := h.expect("PATCH", "/organization/preferences", preferenceBody("zh-Hans", 0, "account-global-key"), f.cookie, 200)
	if org["organization_id"] != f.user.TenantID || org["locale"] != "zh-CN" {
		t.Fatalf("organization scope: %v", org)
	}
	oldCookie := f.cookie
	created := f.call("POST", "/organizations", map[string]any{"name": "另一语言组织", "idempotency_key": newID()}, 201)
	f.call("POST", "/auth/switch", map[string]any{"organization_id": created["id"], "idempotency_key": newID()}, 200)
	if got := h.expect("GET", "/account/preferences", "", f.cookie, 200); got["locale"] != "en-US" || got["version"] != float64(1) {
		t.Fatalf("account failed to follow organization switch: %v", got)
	}
	if got := h.expect("GET", "/organization/preferences", "", f.cookie, 200); got["locale"] != "" || got["version"] != float64(0) {
		t.Fatalf("organization preference leaked: %v", got)
	}
	if got := h.expect("PATCH", "/account/preferences", accountBody, f.cookie, 200); stringify(got) != stringify(first) {
		t.Fatal("global replay was partitioned by organization")
	}
	h.expect("PATCH", "/organization/preferences", preferenceBody("en", 0, "account-global-key"), f.cookie, 200)
	h.expect("PATCH", "/account/preferences", preferenceBody("", 1, newID()), f.cookie, 200)
	for _, test := range []struct {
		cookie *http.Cookie
		want   string
	}{{oldCookie, "zh-CN"}, {f.cookie, "en-US"}} {
		_, head, _ := h.request("GET", "/auth/session", "", test.cookie, map[string]string{"Accept-Language": "zh-CN"})
		if head.Get("Content-Language") != test.want {
			t.Fatalf("clear did not inherit current organization: %v", head)
		}
	}
	if err := f.s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	restarted := newPreferenceHTTP(t, (&Server{Service: f.s, Engine: f.e}).Handler())
	if got := restarted.expect("GET", "/account/preferences", "", f.cookie, 200); got["version"] != float64(2) || got["locale"] != "" {
		t.Fatalf("restart lost clear tombstone: %v", got)
	}
	var count int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_activity WHERE kind='account.preferences.update'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("replay duplicated account audit: %d %v", count, err)
	}
}

func TestLocalePreferencesStrictJSONAndBilingualDiagnostics(t *testing.T) {
	f := newFixture(t)
	h := newPreferenceHTTP(t, f.handler)
	for _, path := range []string{"/account/preferences", "/organization/preferences"} {
		for name, body := range map[string]string{
			"missing_version":  `{"locale":"en","idempotency_key":"strict-key"}`,
			"missing_locale":   `{"expected_version":0,"idempotency_key":"strict-key"}`,
			"missing_key":      `{"locale":"en","expected_version":0}`,
			"null_locale":      `{"locale":null,"expected_version":0,"idempotency_key":"strict-key"}`,
			"number_locale":    `{"locale":1,"expected_version":0,"idempotency_key":"strict-key"}`,
			"unsupported":      preferenceBody("zh-Hant", 0, "strict-key"),
			"control_locale":   preferenceBody("en\n", 0, "strict-key"),
			"null_version":     `{"locale":"en","expected_version":null,"idempotency_key":"strict-key"}`,
			"string_version":   `{"locale":"en","expected_version":"0","idempotency_key":"strict-key"}`,
			"fractional":       `{"locale":"en","expected_version":0.0,"idempotency_key":"strict-key"}`,
			"fractional_large": `{"locale":"en","expected_version":2147483646.0000000001,"idempotency_key":"strict-key"}`,
			"exponent":         `{"locale":"en","expected_version":0e0,"idempotency_key":"strict-key"}`,
			"negative":         `{"locale":"en","expected_version":-1,"idempotency_key":"strict-key"}`,
			"overflow":         `{"locale":"en","expected_version":2147483647,"idempotency_key":"strict-key"}`,
			"null_key":         `{"locale":"en","expected_version":0,"idempotency_key":null}`,
			"short_key":        preferenceBody("en", 0, "short"),
			"control_key":      preferenceBody("en", 0, "strict-key\n"),
			"duplicate":        `{"locale":"en","locale":"zh","expected_version":0,"idempotency_key":"strict-key"}`,
			"unknown":          `{"locale":"en","expected_version":0,"idempotency_key":"strict-key","organization_id":"foreign"}`,
			"trailing":         preferenceBody("en", 0, "strict-key") + ` {}`,
			"array":            `[]`,
			"null":             `null`,
		} {
			t.Run(strings.Trim(path, "/")+"/"+name, func(t *testing.T) {
				status, head, result := h.request("PATCH", path, body, f.cookie, map[string]string{"X-Suma-Language": "en-US"})
				if status != 400 {
					t.Fatalf("strict body accepted: %d %v", status, result)
				}
				detail := result["error"].(map[string]any)
				if detail["localized"] != true || detail["locale"] != "en-US" || head.Get("Content-Language") != "en-US" || !strings.HasPrefix(detail["message_key"].(string), "preferences.") {
					t.Fatalf("specific diagnostic untranslated: %v", result)
				}
			})
		}
		h.expect("POST", path, preferenceBody("en", 0, newID()), f.cookie, 405)
	}
	if got := h.expect("GET", "/account/preferences", "", f.cookie, 200); got["version"] != float64(0) {
		t.Fatal("invalid writes created preference")
	}
}

func TestLocalePreferencesCurrentAuthorityBeforeReplay(t *testing.T) {
	f := newFixture(t)
	h := newPreferenceHTTP(t, f.handler)
	owner := f.cookie
	inv := governanceInvite(f, "locale-admin@example.test", "admin", nil)
	accepted := governanceAccept(f, inv, "locale-admin@example.test")
	admin := f.cookie
	adminID := accepted["user"].(map[string]any)["id"].(string)
	body := preferenceBody("en", 0, newID())
	h.expect("PATCH", "/organization/preferences", body, admin, 200)
	f.cookie = owner
	f.call("PATCH", "/organization/members/"+adminID, map[string]any{"role": "member", "expected_version": 1, "idempotency_key": newID()}, 200)
	h.expect("GET", "/organization/preferences", "", admin, 200)
	h.expect("PATCH", "/organization/preferences", body, admin, 403)
	h.expect("PATCH", "/organization/preferences", preferenceBody("zh", 1, newID()), admin, 403)
	h.expect("PATCH", "/account/preferences", preferenceBody("zh", 0, newID()), admin, 200)
	for _, kind := range []string{"api", "runner", "agent"} {
		key := f.call("POST", "/auth/tokens", map[string]any{"project_id": f.project, "scope": kind, "label": "browser"}, 201)["token"].(string)
		for _, path := range []string{"/account/preferences", "/organization/preferences"} {
			for _, method := range []string{"GET", "PATCH"} {
				status, _, _ := h.request(method, path, body, owner, map[string]string{"Authorization": "Bearer " + key})
				if status != 403 {
					t.Fatalf("scoped %s read/wrote preferences: %s %s %d", kind, method, path, status)
				}
			}
		}
	}
	custom, err := f.s.CreateToken(context.Background(), f.user, "unscoped-cli", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/account/preferences", "/organization/preferences"} {
		status, _, _ := h.request("GET", path, "", nil, map[string]string{"Authorization": "Bearer " + custom})
		if status != 403 {
			t.Fatalf("nonbrowser preference read: %d", status)
		}
	}
	f.call("PATCH", "/organization/members/"+adminID, map[string]any{"action": "remove", "expected_version": 2, "idempotency_key": newID()}, 200)
	for _, path := range []string{"/account/preferences", "/organization/preferences"} {
		h.expect("GET", path, "", admin, 401)
	}
	h.expect("PATCH", "/organization/preferences", body, admin, 401)
}

func TestLocalePreferencesPrecedenceIndependentAccountsAndConcurrentResponses(t *testing.T) {
	f := newFixture(t)
	h := newPreferenceHTTP(t, f.handler)
	owner := f.cookie
	h.expect("PATCH", "/organization/preferences", preferenceBody("en", 0, newID()), owner, 200)
	inv := governanceInvite(f, "locale-other@example.test", "member", nil)
	governanceAccept(f, inv, "locale-other@example.test")
	member := f.cookie
	h.expect("PATCH", "/account/preferences", preferenceBody("zh", 0, newID()), owner, 200)
	if got := h.expect("GET", "/account/preferences", "", member, 200); got["locale"] != "" || got["version"] != float64(0) {
		t.Fatal("account preference leaked", got)
	}
	for _, test := range []struct {
		name    string
		cookie  *http.Cookie
		headers map[string]string
		want    string
	}{
		{"account", owner, map[string]string{"Accept-Language": "en-US"}, "zh-CN"},
		{"organization", member, map[string]string{"Accept-Language": "zh-CN"}, "en-US"},
		{"explicit", owner, map[string]string{"X-Suma-Language": "en_GB"}, "en-US"},
		{"invalid_explicit", owner, map[string]string{"X-Suma-Language": "zh-Hant", "Accept-Language": "en"}, "zh-CN"},
		{"unauthenticated", nil, map[string]string{"Accept-Language": "zh-CN"}, "zh-CN"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, head, _ := h.request("GET", "/auth/session", "", test.cookie, test.headers)
			if head.Get("Content-Language") != test.want {
				t.Fatalf("precedence: %v", head)
			}
		})
	}
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		cookie, want := owner, "zh-CN"
		if i%2 == 1 {
			cookie, want = member, "en-US"
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, head, _ := h.request("GET", "/organization/preferences", "", cookie, nil)
			if status != 200 || head.Get("Content-Language") != want {
				t.Errorf("concurrent language leaked: %d %v", status, head)
			}
		}()
	}
	wg.Wait()
	// Clearing both stored layers restores negotiation without deleting versions.
	h.expect("PATCH", "/account/preferences", preferenceBody("", 1, newID()), owner, 200)
	h.expect("PATCH", "/organization/preferences", preferenceBody("", 1, newID()), owner, 200)
	_, head, _ := h.request("GET", "/auth/session", "", owner, map[string]string{"Accept-Language": "en"})
	if head.Get("Content-Language") != "en-US" {
		t.Fatal("request negotiation did not return after clear")
	}
	_, head, _ = h.request("GET", "/auth/session", "", owner, nil)
	if head.Get("Content-Language") != "zh-CN" {
		t.Fatal("product default did not return after clear")
	}
}

func TestLocalePreferencesConcurrentGlobalAccountCASAcrossOrganizations(t *testing.T) {
	f := newFixture(t)
	firstCookie := f.cookie
	created := f.call("POST", "/organizations", map[string]any{"name": "并发语言组织", "idempotency_key": newID()}, 201)
	f.call("POST", "/auth/switch", map[string]any{"organization_id": created["id"], "idempotency_key": newID()}, 200)
	var sequence int
	var name, path string
	if err := f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	other := &Service{DB: db, Dialect: "sqlite"}
	servers := []*preferenceHTTP{newPreferenceHTTP(t, f.handler), newPreferenceHTTP(t, (&Server{Service: other, Engine: f.e}).Handler())}
	cookies := []*http.Cookie{firstCookie, f.cookie}
	start := make(chan struct{})
	results := make(chan int, 2)
	for i := range servers {
		go func(i int) {
			<-start
			status, _, _ := servers[i].request("PATCH", "/account/preferences", preferenceBody([]string{"zh", "en"}[i], 0, fmt.Sprintf("global-race-%d", i)), cookies[i], nil)
			results <- status
		}(i)
	}
	close(start)
	a, b := <-results, <-results
	if !(a == 200 && b == 409 || a == 409 && b == 200) {
		t.Fatalf("global CAS was not atomic: %d %d", a, b)
	}
	for i := range servers {
		if got := servers[i].expect("GET", "/account/preferences", "", cookies[i], 200); got["version"] != float64(1) {
			t.Fatalf("CAS incremented twice: %v", got)
		}
	}
}

func TestLocalePreferencesPublicCatalogExactAndMachineLanguageUnchanged(t *testing.T) {
	f := newFixture(t)
	h := newPreferenceHTTP(t, f.handler)
	h.expect("PATCH", "/account/preferences", preferenceBody("en", 0, newID()), f.cookie, 200)
	key := f.call("POST", "/auth/tokens", map[string]any{"project_id": f.project, "scope": "api"}, 201)["token"].(string)
	_, head, _ := h.request("GET", "/projects", "", nil, map[string]string{"Authorization": "Bearer " + key, "Accept-Language": "zh-CN"})
	if head.Get("Content-Language") != "zh-CN" {
		t.Fatal("machine inherited human account locale")
	}
	catalog := h.expect("GET", "/localization/catalog", "", nil, 200)
	found := false
	for _, item := range catalog["messages"].([]any) {
		if item.(map[string]any)["key"] == "preferences.locale_invalid" {
			found = true
		}
	}
	if !found {
		t.Fatal("public static catalog omitted preferences messages")
	}
}

func TestLocalePreferencesSessionResponsesUseReturnedHumanAndOrganization(t *testing.T) {
	f := newFixture(t)
	h := newPreferenceHTTP(t, f.handler)
	owner := f.cookie
	h.expect("PATCH", "/organization/preferences", preferenceBody("en", 0, newID()), owner, 200)
	created := f.call("POST", "/organizations", map[string]any{"name": "切换后中文默认", "idempotency_key": newID()}, 201)
	status, headers, _ := h.request("POST", "/auth/switch", stringify(map[string]any{"organization_id": created["id"], "idempotency_key": newID()}), owner, nil)
	if status != 200 || headers.Get("Content-Language") != "zh-CN" {
		t.Fatalf("switch response used old organization: %d %v", status, headers)
	}
	h.expect("PATCH", "/account/preferences", preferenceBody("en", 0, newID()), owner, 200)
	status, headers, _ = h.request("POST", "/auth/login", `{"email":"owner@example.test","password":"test-password-strong-42"}`, nil, map[string]string{"Accept-Language": "zh-CN"})
	if status != 200 || headers.Get("Content-Language") != "en-US" {
		t.Fatalf("login response lost persisted account preference: %d %v", status, headers)
	}
	f.cookie = owner
	inv := governanceInvite(f, "locale-accept@example.test", "member", nil)
	status, headers, _ = h.request("POST", "/auth/invitations/accept", stringify(map[string]any{"token": inv["token"], "email": "locale-accept@example.test", "name": "新语言成员", "password": "invited-password-strong-42", "idempotency_key": newID()}), nil, map[string]string{"Accept-Language": "zh-CN"})
	if status != 200 || headers.Get("Content-Language") != "en-US" {
		t.Fatalf("accept response ignored returned organization: %d %v", status, headers)
	}
}

func TestLocalePreferencesStalePrincipalAndBrowserCredentialRechecked(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	request := LocalePreferenceUpdateRequest{Locale: "en", ExpectedVersion: 0, IdempotencyKey: newID()}
	if _, err := f.s.UpdateOrganizationPreferences(ctx, f.user, f.cookie.Value, request); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.UpdateOrganizationPreferences(ctx, f.user, f.cookie.Value, request); err == nil {
		t.Fatal("stale owner Principal replayed administrator receipt")
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_sessions SET expires_at='2000-01-01T00:00:00Z' WHERE token_hash=?`, digest(f.cookie.Value)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AccountPreferences(ctx, f.user, f.cookie.Value); err == nil {
		t.Fatal("expired browser read preferences through stale Principal")
	}
	if _, err := f.s.UpdateAccountPreferences(ctx, f.user, f.cookie.Value, request); err == nil {
		t.Fatal("expired browser wrote preferences through stale Principal")
	}
	if _, err := f.s.OrganizationPreferences(ctx, Principal{internal: true, ID: f.user.ID, TenantID: f.user.TenantID}, f.cookie.Value); err == nil {
		t.Fatal("internal Principal acquired browser preference authority")
	}
}

func TestLocalePreferencesAuditAndReceiptCommitAtomically(t *testing.T) {
	f := newFixture(t)
	h := newPreferenceHTTP(t, f.handler)
	if _, err := f.s.DB.Exec(`CREATE TRIGGER fail_locale_audit BEFORE INSERT ON suma_activity WHEN NEW.kind='account.preferences.update' BEGIN SELECT RAISE(ABORT,'fixture audit storage failure'); END`); err != nil {
		t.Fatal(err)
	}
	body := preferenceBody("en", 0, newID())
	h.expect("PATCH", "/account/preferences", body, f.cookie, 500)
	var count int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_locale_preference_operations`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed audit retained receipt: %d %v", count, err)
	}
	if got := h.expect("GET", "/account/preferences", "", f.cookie, 200); got["version"] != float64(0) {
		t.Fatal("failed audit committed account preference", got)
	}
	if _, err := f.s.DB.Exec(`DROP TRIGGER fail_locale_audit`); err != nil {
		t.Fatal(err)
	}
	first := h.expect("PATCH", "/account/preferences", body, f.cookie, 200)
	for _, locale := range []string{"zh-CN", "en-US"} {
		status, _, reply := h.request("PATCH", "/account/preferences", body, f.cookie, map[string]string{"X-Suma-Language": locale})
		if status != 200 || stringify(reply) != stringify(first) {
			t.Fatalf("presentation locale changed receipt: %d %v", status, reply)
		}
	}
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_locale_preference_operations`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("replay duplicated receipt: %d %v", count, err)
	}
}

func preferenceExistingTableHashes(t *testing.T, s *Service) map[string]string {
	t.Helper()
	rows, err := s.DB.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var table string
		if err = rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		if table != "suma_account_preferences" && table != "suma_organization_preferences" && table != "suma_account_preference_locks" && table != "suma_locale_preference_operations" {
			tables = append(tables, table)
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	out := map[string]string{}
	for _, table := range tables {
		entries, err := s.DB.Query(`SELECT * FROM "` + table + `"`)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := entries.Columns()
		if err != nil {
			t.Fatal(err)
		}
		var encoded []string
		for entries.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err = entries.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			encoded = append(encoded, stringify(values))
		}
		if err = entries.Err(); err != nil {
			t.Fatal(err)
		}
		entries.Close()
		sort.Strings(encoded)
		out[table] = digest(stringify(columns) + stringify(encoded))
	}
	return out
}

func TestLocalePreferencesMigrationPreservesExistingTablesAndUnsetReads(t *testing.T) {
	f := newFixture(t)
	for _, table := range []string{"suma_account_preferences", "suma_organization_preferences", "suma_account_preference_locks", "suma_locale_preference_operations"} {
		if _, err := f.s.DB.Exec(`DROP TABLE ` + table); err != nil {
			t.Fatal(err)
		}
	}
	before := preferenceExistingTableHashes(t, f.s)
	for i := 0; i < 2; i++ {
		if err := f.s.MigrateLocalePreferences(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if after := preferenceExistingTableHashes(t, f.s); stringify(before) != stringify(after) {
		t.Fatal("preference migration changed existing table contents")
	}
	h := newPreferenceHTTP(t, f.handler)
	h.expect("GET", "/account/preferences", "", f.cookie, 200)
	h.expect("GET", "/organization/preferences", "", f.cookie, 200)
	for _, table := range []string{"suma_account_preferences", "suma_organization_preferences", "suma_account_preference_locks", "suma_locale_preference_operations"} {
		var count int
		if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("unset read mutated %s: %d %v", table, count, err)
		}
	}
	t.Logf("preserved %d existing table content hashes", len(before))
}

func TestLocalePreferencesPoolMachineCannotReadOrWriteHumanDefaults(t *testing.T) {
	pool := newPoolMachineFixture(t)
	h := newPreferenceHTTP(t, pool.f.handler)
	for _, path := range []string{"/account/preferences", "/organization/preferences"} {
		for _, method := range []string{"GET", "PATCH"} {
			status, _, _ := h.request(method, path, preferenceBody("en", 0, newID()), nil, map[string]string{"Authorization": "Bearer " + pool.credential.Token})
			if status != 401 {
				t.Fatalf("pool machine preference access: %s %s %d", method, path, status)
			}
		}
	}
}

func TestLocalePreferencesExpectedScopeRejectsSharedCookieDriftBeforeMutationOrReplay(t *testing.T) {
	f := newFixture(t)
	h := newPreferenceHTTP(t, f.handler)
	h.inferExpectedScope = false
	originalCookie := f.cookie
	user, organizationA := f.user.ID, f.user.TenantID
	organization := f.call("POST", "/organizations", map[string]any{"name": "共享 Cookie 的第二组织", "idempotency_key": newID()}, 201)
	f.call("POST", "/auth/switch", map[string]any{"organization_id": organization["id"], "idempotency_key": newID()}, 200)
	newCookie := f.cookie
	body := preferenceBody("en-US", 0, "shared-cookie-original-op")
	oldHeaders := map[string]string{"X-Suma-Expected-User": user, "X-Suma-Expected-Organization": organizationA}
	status, _, result := h.request("PATCH", "/organization/preferences", body, newCookie, oldHeaders)
	if status != 409 {
		t.Fatalf("stale organization view mutated the shared cookie's new organization: %d %v", status, result)
	}
	for _, table := range []string{"suma_organization_preferences", "suma_locale_preference_operations"} {
		var count int
		if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("scope mismatch persisted %s: %d %v", table, count, err)
		}
	}
	status, _, _ = h.request("PATCH", "/organization/preferences", body, originalCookie, oldHeaders)
	if status != 200 {
		t.Fatalf("correct original organization rejected: %d", status)
	}
	status, _, _ = h.request("PATCH", "/organization/preferences", body, newCookie, oldHeaders)
	if status != 409 {
		t.Fatalf("old receipt was applied/replayed under a different organization: %d", status)
	}
	var count int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_organization_preferences`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("foreign cookie created another preference: %d %v", count, err)
	}
	// Account recovery intentionally follows this same user across organizations.
	accountHeaders := map[string]string{"X-Suma-Expected-User": user}
	status, _, _ = h.request("PATCH", "/account/preferences", body, originalCookie, accountHeaders)
	if status != 200 {
		t.Fatalf("account original write rejected: %d", status)
	}
	status, _, _ = h.request("PATCH", "/account/preferences", body, newCookie, accountHeaders)
	if status != 200 {
		t.Fatalf("same-account cross-organization recovery rejected: %d", status)
	}
	f.cookie = originalCookie
	invite := governanceInvite(f, "scope-drift-other@example.test", "member", nil)
	accepted := governanceAccept(f, invite, "scope-drift-other@example.test")
	otherUser := accepted["user"].(map[string]any)["id"].(string)
	status, _, _ = h.request("PATCH", "/account/preferences", body, f.cookie, accountHeaders)
	if status != 409 {
		t.Fatalf("stale account view wrote the newly signed-in account: %d", status)
	}
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_account_preferences WHERE user_id=?`, otherUser).Scan(&count); err != nil || count != 0 {
		t.Fatalf("foreign account was mutated: %d %v", count, err)
	}
}

func TestLocalePreferencesExpectedScopeHeadersAreStrictAndCurrentRoleWins(t *testing.T) {
	f := newFixture(t)
	h := newPreferenceHTTP(t, f.handler)
	h.inferExpectedScope = false
	body := preferenceBody("en-US", 0, newID())
	for name, headers := range map[string]map[string]string{
		"missing_both":         {},
		"missing_user":         {"X-Suma-Expected-Organization": f.user.TenantID},
		"missing_organization": {"X-Suma-Expected-User": f.user.ID},
		"bad_user":             {"X-Suma-Expected-User": "not-a-user-id", "X-Suma-Expected-Organization": f.user.TenantID},
		"bad_organization":     {"X-Suma-Expected-User": f.user.ID, "X-Suma-Expected-Organization": "foreign"},
		"joined_duplicate":     {"X-Suma-Expected-User": f.user.ID + "," + f.user.ID, "X-Suma-Expected-Organization": f.user.TenantID},
	} {
		t.Run(name, func(t *testing.T) {
			status, _, result := h.request("PATCH", "/organization/preferences", body, f.cookie, headers)
			if status != 400 {
				t.Fatalf("bad scope headers accepted: %d %v", status, result)
			}
		})
	}
	status, _, _ := h.request("PATCH", "/account/preferences", body, f.cookie, map[string]string{"X-Suma-Expected-User": f.user.ID, "X-Suma-Expected-Organization": f.user.TenantID})
	if status != 400 {
		t.Fatalf("account write accepted organization-scoped header: %d", status)
	}
	// Separate duplicate header lines must not be collapsed into a trusted value.
	req, err := http.NewRequest("PATCH", h.server.URL+"/api/v1/account/preferences", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(f.cookie)
	req.Header.Add("X-Suma-Expected-User", f.user.ID)
	req.Header.Add("X-Suma-Expected-User", f.user.ID)
	response, err := h.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 400 {
		t.Fatalf("duplicate scope header accepted: %d", response.StatusCode)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	status, _, _ = h.request("PATCH", "/organization/preferences", body, f.cookie, map[string]string{})
	if status != 403 {
		t.Fatalf("scope validation hid the current role rejection: %d", status)
	}
}
