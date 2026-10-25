package platform

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaintenanceFreezesBusinessBeforeParsingButAllowsReadAndAuthentication(t *testing.T) {
	f := newFixture(t)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Read during upgrade", "description": "Business state must remain unchanged."}), 201)
	f.handler = (&Server{Service: f.s, Engine: f.e, Maintenance: true}).Handler()
	for _, route := range []struct{ method, path string }{
		{"POST", "/requirements"},
		{"PUT", "/requirements/unknown/design"},
		{"POST", "/requirements/unknown/design/preview"},
		{"POST", "/runner/results"},
		{"PUT", "/runner/artifact-uploads/unknown/blobs/unknown"},
		{"POST", "/automation-ingress/unknown"},
		{"POST", "/auth/setup"},
	} {
		r := httptest.NewRequest(route.method, "http://suma.test/api/v1"+route.path, strings.NewReader("not valid JSON or a file"))
		r.Header.Set("Accept-Language", "en-US")
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != http.StatusLocked || !strings.Contains(w.Body.String(), "maintenance_mode") || !strings.Contains(w.Body.String(), "has not started") || w.Header().Get("Retry-After") != "5" {
			t.Fatalf("maintenance did not reject before dispatch: %s %s %d %s", route.method, route.path, w.Code, w.Body.String())
		}
	}
	view := f.call("GET", "/tasks/"+task["id"].(string)+"/design-context?project_id="+f.project, nil, 200)
	if view["bound"] != false || view["eligibility"] != "none" {
		t.Fatal("unbound read became adopted during maintenance", view)
	}
	f.call("GET", "/tasks/"+task["id"].(string)+"/design-context?project_id="+f.project+"&project_id="+f.project, nil, 400)
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "test-password-strong-42"}, 200)
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	var tasks, requirements int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_tasks`).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_requirements`).Scan(&requirements); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 || requirements != 0 {
		t.Fatal("maintenance changed business state", tasks, requirements)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, httptest.NewRequest("GET", "http://suma.test/healthz", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"maintenance":true`) {
		t.Fatal("liveness must disclose paused business mode", w.Code, w.Body.String())
	}
}
