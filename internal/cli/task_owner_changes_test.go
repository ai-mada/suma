package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestTaskOwnersCLIRoutesAndOriginalBody(t *testing.T) {
	preview := `{"owner_id":"human-1","expected_owner_version":3,"tasks":[{"task_id":"task-b","expected_version":2},{"task_id":"task-a","expected_version":1}]}`
	apply := strings.TrimSuffix(preview, "}") + `,"preview_sha256":"` + strings.Repeat("a", 64) + `","acknowledge_invalidation":true,"idempotency_key":"owner-original-operation"}`
	for _, scenario := range []struct {
		name, input, method, path, query string
		args                             []string
	}{
		{"targets", "", "GET", "/api/v1/task-owner-targets", "cursor=next%2B%2F%3D&limit=5&project_id=project-one&q=%E4%BA%BA%E5%91%98+%25", []string{"task-owners", "targets", "--query", "人员 %", "--limit", "5", "--cursor", "next+/="}},
		{"preview", preview, "POST", "/api/v1/task-owner-changes/preview", "", []string{"task-owners", "preview", "--file", "-"}},
		{"apply", apply, "POST", "/api/v1/task-owner-changes", "", []string{"task-owners", "apply", "--file", "-"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != scenario.method || r.URL.Path != scenario.path || r.URL.RawQuery != scenario.query {
					t.Errorf("unexpected route: %s %s", r.Method, r.URL)
				}
				if scenario.input != "" {
					var body, expected map[string]any
					decode := json.NewDecoder(r.Body)
					decode.UseNumber()
					if err := decode.Decode(&body); err != nil {
						t.Fatal(err)
					}
					decode = json.NewDecoder(strings.NewReader(scenario.input))
					decode.UseNumber()
					_ = decode.Decode(&expected)
					expected["project_id"] = "project-one"
					if !reflect.DeepEqual(body, expected) || r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) {
						t.Error("original selection, numbers, operation or identity changed")
					}
				}
				_, _ = w.Write([]byte(`{"project_id":"project-one","operation_id":"owner-original-operation"}`))
			}))
			defer server.Close()
			code, _, stderr := contextInvoke(t, server.URL, "en-US", scenario.input, scenario.args...)
			if code != 0 || calls != 1 {
				t.Fatalf("exit=%d calls=%d error=%s", code, calls, stderr)
			}
		})
	}
}

func TestTaskOwnersCLIRejectsAmbiguousMutationBeforeNetwork(t *testing.T) {
	valid := `{"owner_id":"human-1","expected_owner_version":3,"tasks":[{"task_id":"task-a","expected_version":1}]}`
	for _, input := range []string{
		valid,
		strings.ReplaceAll(valid, `:1}`, `:1.0}`),
		strings.ReplaceAll(valid, `:1}`, `:1e0}`),
		strings.ReplaceAll(valid, `"tasks":[{"task_id":"task-a","expected_version":1}]`, `"tasks":null`),
		strings.TrimSuffix(valid, "}") + `,"preview_sha256":"` + strings.Repeat("a", 64) + `","acknowledge_invalidation":false,"idempotency_key":"original-operation"}`,
		strings.TrimSuffix(valid, "}") + `,"project_id":"another-project"}`,
	} {
		t.Run(input, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
			defer server.Close()
			code, _, stderr := contextInvoke(t, server.URL, "zh-CN", input, "task-owners", "apply", "--file", "-")
			if code == 0 || calls != 0 || !strings.Contains(stderr, "负责人") {
				t.Fatalf("exit=%d calls=%d error=%s", code, calls, stderr)
			}
		})
	}
}
