package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOriginalViewIsExplicitAndStableAcrossRequests(t *testing.T) {
	user, organization := strings.Repeat("a", 32), strings.Repeat("b", 32)
	var seen atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		if r.URL.Path != "/api/v1/tasks" {
			t.Error("unexpected identity-refresh request", r.URL.Path)
		}
		if r.Header.Get("X-Suma-Expected-User") != user || r.Header.Get("X-Suma-Expected-Organization") != organization {
			t.Error("original view changed")
		}
		w.Write([]byte(`{"id":"task"}`))
	}))
	defer server.Close()
	c, err := newClient(server.URL, "project", "fixture-token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.setOriginalView(user, organization); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = c.request(context.Background(), "POST", "/api/v1/tasks", map[string]any{"idempotency_key": "same-key"}, true); err != nil {
			t.Fatal(err)
		}
	}
	if seen.Load() != 2 {
		t.Fatal("unexpected request count", seen.Load())
	}
	if err = c.setOriginalView("", organization); err == nil {
		t.Fatal("accepted organization without original account")
	}
	// Invalid replacement must preserve the existing captured view.
	if c.expectedUser != user || c.expectedOrganization != organization {
		t.Fatal("invalid view replaced valid context")
	}
}
