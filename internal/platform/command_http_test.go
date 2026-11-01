package platform

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthorityHTTPOriginalView(t *testing.T) {
	p := Principal{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", TenantID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	for _, tc := range []struct {
		name        string
		users, orgs []string
		account     bool
		status      int
	}{
		{"bound", []string{p.ID}, []string{p.TenantID}, false, 0},
		{"missing", nil, nil, false, 428},
		{"duplicate", []string{p.ID, p.ID}, []string{p.TenantID}, false, 428},
		{"joined", []string{p.ID + "," + p.ID}, []string{p.TenantID}, false, 428},
		{"other-user", []string{"cccccccccccccccccccccccccccccccc"}, []string{p.TenantID}, false, 403},
		{"other-org", []string{p.ID}, []string{"cccccccccccccccccccccccccccccccc"}, false, 403},
		{"global-account", []string{p.ID}, nil, true, 0},
		{"account-cannot-rebind", []string{p.ID}, []string{p.TenantID}, true, 428},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", nil)
			for _, v := range tc.users {
				r.Header.Add("X-Suma-Expected-User", v)
			}
			for _, v := range tc.orgs {
				r.Header.Add("X-Suma-Expected-Organization", v)
			}
			err := validateOriginalView(r, p, tc.account)
			if tc.status == 0 {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var ae *APIError
			if !errors.As(err, &ae) || ae.Status != tc.status {
				t.Fatalf("got %v, want HTTP %d", err, tc.status)
			}
		})
	}
}

func TestAuthorityHTTPCommandClassification(t *testing.T) {
	for _, path := range []string{"search", "workflows/validate", "agent/tools/invoke", "automations/preview", "release-plans/preview", "deployments/id/rollback-preview", "auth/logout", "tool-profiles/matches"} {
		if BrowserCommandRequiresView(http.MethodPost, path) {
			t.Errorf("semantic read/logout requires command view: %s", path)
		}
	}
	for _, path := range []string{"tasks", "teams", "auth/tokens", "auth/switch", "scheduling/policy-previews", "execution-pools/id/policy-previews", "future/preview"} {
		if !BrowserCommandRequiresView(http.MethodPost, path) {
			t.Errorf("write bypassed view: %s", path)
		}
	}
}

func TestAuthorityHTTPOptionalReadView(t *testing.T) {
	p := Principal{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", TenantID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	for _, tc := range []struct {
		name, header string
		values       []string
		status       int
	}{
		{"omitted", "", nil, 0},
		{"account-only", "X-Suma-Expected-User", []string{p.ID}, 0},
		{"organization-only", "X-Suma-Expected-Organization", []string{p.TenantID}, 0},
		{"duplicate", "X-Suma-Expected-User", []string{p.ID, p.ID}, 428},
		{"changed", "X-Suma-Expected-User", []string{"cccccccccccccccccccccccccccccccc"}, 403},
		{"invalid", "X-Suma-Expected-Organization", []string{""}, 428},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/tool-profiles/id/diagnostics", nil)
			for _, value := range tc.values {
				r.Header.Add(tc.header, value)
			}
			err := validateOptionalReadView(r, p)
			if tc.status == 0 {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var api *APIError
			if !errors.As(err, &api) || api.Status != tc.status {
				t.Fatalf("optional read assertion: got %v, want %d", err, tc.status)
			}
		})
	}
}
