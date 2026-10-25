package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthorityAnonymousSessionRetainsBootstrapHint(t *testing.T) {
	s := authorityEmptyFixture(t)
	handler := (&Server{Service: s}).Handler()
	check := func(token string, want bool) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if w.Code != 401 || out["setup_required"] != want {
			t.Fatalf("bootstrap hint: status=%d hint=%v want=%v", w.Code, out["setup_required"], want)
		}
	}
	check("", true)
	_, token, err := s.Setup(context.Background(), map[string]any{"email": "bootstrap@example.test", "name": "Bootstrap", "password": "bootstrap-test-password-42"})
	if err != nil {
		t.Fatal(err)
	}
	check("", false)
	if err = logoutCredentialForTest(context.Background(), s, token); err != nil {
		t.Fatal(err)
	}
	check(token, false)
}
