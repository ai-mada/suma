package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func reviewRequestsCLIInvoke(t *testing.T, origin, locale, input string, command ...string) (int, string, string) {
	t.Helper()
	args := append([]string{"--control", origin, "--language", locale, "--project", "project-one", "--expected-user", strings.Repeat("a", 32), "--expected-organization", strings.Repeat("b", 32), "review-requests"}, command...)
	var out, stderr strings.Builder
	code := Execute(context.Background(), args, strings.NewReader(input), &out, &stderr)
	return code, out.String(), stderr.String()
}

func TestTaskReviewRequestsCLIExactScopedReadsAndSinglePage(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		args       []string
		query      map[string]string
	}{
		{"list", "/api/v1/task-review-requests", []string{"list", "--task", "task-one", "--limit", "50", "--cursor", "original_request_cursor"}, map[string]string{"task_id": "task-one", "limit": "50", "cursor": "original_request_cursor"}},
		{"targets", "/api/v1/task-review-targets", []string{"targets", "--task", "task-one", "--query", " 原名 %_! ", "--limit", "50", "--cursor", "original_target_cursor"}, map[string]string{"task_id": "task-one", "q": " 原名 %_! ", "limit": "50", "cursor": "original_target_cursor"}},
		{"get", "/api/v1/task-review-requests/request-one", []string{"get", "request-one"}, map[string]string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				query := r.URL.Query()
				if r.Method != http.MethodGet || r.URL.Path != tc.path || query.Get("project_id") != "project-one" || len(query) != len(tc.query)+1 || r.Header.Get("Accept-Language") != "en-US" {
					t.Error("review request read lost its exact target or original scope", r.Method, r.URL)
				}
				for key, value := range tc.query {
					if query.Get(key) != value || len(query[key]) != 1 {
						t.Error("original read parameter changed", key, query.Get(key), value)
					}
				}
				_, _ = w.Write([]byte(`{"next_cursor":"explicit-only","can_request":false,"note":"准确交付 unchanged"}`))
			}))
			defer server.Close()
			code, out, stderr := reviewRequestsCLIInvoke(t, server.URL, "en-US", "", tc.args...)
			if code != 0 || stderr != "" || calls != 1 || !strings.Contains(out, `"can_request": false`) || !strings.Contains(out, "准确交付 unchanged") || !strings.Contains(out, "explicit-only") {
				t.Fatal("read transformed the response or followed its cursor", code, calls, out, stderr)
			}
		})
	}
}

func TestTaskReviewRequestsCLIOriginalWritesHaveNoQueryOrNewOperation(t *testing.T) {
	for _, tc := range []struct{ action, path, body string }{
		{"create", "/api/v1/task-review-requests", `{"project_id":"project-one","task_id":"task-one","expected_task_version":17,"run_id":"run-one","expected_run_version":29,"reviewer_resource_id":"human-one","note":" 请审查原交付\n保留换行 ","idempotency_key":"original-create-operation"}`},
		{"cancel", "/api/v1/task-review-requests/request-one/commands", `{"project_id":"project-one","expected_version":3,"action":"cancel","reason":" 取消这个原请求 ","idempotency_key":"original-cancel-operation"}`},
	} {
		t.Run(tc.action, func(t *testing.T) {
			var bodies []map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != tc.path || r.URL.RawQuery != "" || r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
					t.Error("closed POST or captured identity changed", r.Method, r.URL)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				bodies = append(bodies, body)
				_, _ = w.Write([]byte(`{"id":"request-one","version":3,"binding_current":false,"state":"pending"}`))
			}))
			defer server.Close()
			args := []string{tc.action}
			if tc.action == "cancel" {
				args = append(args, "request-one")
			}
			args = append(args, "--file", "-")
			for range 2 {
				code, out, stderr := reviewRequestsCLIInvoke(t, server.URL, "en-US", tc.body, args...)
				if code != 0 || stderr != "" || !strings.Contains(out, `"binding_current": false`) {
					t.Fatal("original command failed or rewrote its receipt", code, out, stderr)
				}
			}
			var original map[string]any
			if err := json.Unmarshal([]byte(tc.body), &original); err != nil || len(bodies) != 2 || !reflect.DeepEqual(bodies[0], original) || !reflect.DeepEqual(bodies[1], original) {
				t.Fatal("explicit recovery changed the original JSON", bodies, err)
			}
		})
	}
}

func TestTaskReviewRequestsCLIUnconfirmedAndForbiddenDoNotRetry(t *testing.T) {
	for _, status := range []int{0, 403} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if status == 0 {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = conn.Close()
				return
			}
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"code":"forbidden","message":"当前审核权限已撤回"}}`)
		}))
		body := `{"project_id":"project-one","expected_version":3,"action":"cancel","reason":"Original cancellation","idempotency_key":"original-cancel-operation"}`
		code, out, stderr := reviewRequestsCLIInvoke(t, server.URL, "zh-CN", body, "cancel", "request-one", "--file", "-")
		server.Close()
		if code != 1 || out != "" || stderr == "" || calls != 1 || status == 403 && !strings.Contains(stderr, "当前审核权限已撤回") {
			t.Fatal("unknown/denied write retried or lost the server error", status, code, calls, out, stderr)
		}
	}
}

func TestTaskReviewRequestsCLIInvalidInputNeverUsesTransport(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	for _, args := range [][]string{{"list"}, {"list", "--task", "task-one", "--limit", "51"}, {"list", "--task", "task-one", "--limit", "01"}, {"targets", "--task", "task-one", "--query", "bad\u0085name"}, {"targets", "--task", "task-one", "--cursor", strings.Repeat("x", 4097)}, {"get", "../request"}, {"get", "request-one", "--task", "replacement"}, {"create"}, {"cancel", "request-one", "--file", "-", "--operation", "replacement"}} {
		code, out, stderr := reviewRequestsCLIInvoke(t, server.URL, "en-US", "{}", args...)
		if code != 2 || out != "" || stderr == "" {
			t.Error("invalid flags accepted", args, code, out, stderr)
		}
	}
	valid := `{"project_id":"project-one","task_id":"task-one","expected_task_version":17,"run_id":"run-one","expected_run_version":29,"reviewer_resource_id":"human-one","note":"Explicit review request","idempotency_key":"original-create-operation"}`
	for _, input := range []string{strings.Replace(valid, "project-one", "foreign-project", 1), strings.Replace(valid, `:17`, `:1e1`, 1), strings.Replace(valid, `"note":"Explicit review request"`, `"note":"   "`, 1), strings.Replace(valid, `"idempotency_key":"original-create-operation"`, `"idempotency_key":""`, 1), strings.TrimSuffix(valid, "}") + `,"reviewer_user_id":"forged"}`} {
		code, out, stderr := reviewRequestsCLIInvoke(t, server.URL, "zh-CN", input, "create", "--file", "-")
		if code != 2 || out != "" || !strings.Contains(stderr, "原JSON") {
			t.Error("invalid original create accepted or untranslated", code, out, stderr)
		}
	}
	if calls != 0 {
		t.Fatal("invalid local input reached the network", calls)
	}
}

func TestTaskReviewRequestsCLIHelpLanguages(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en-US"} {
		code, out, stderr := reviewRequestsCLIInvoke(t, "http://127.0.0.1:1", locale, "", "--help")
		if code != 0 || stderr != "" || !strings.Contains(out, "review-requests cancel") || !strings.Contains(out, "review-requests targets") || locale == "zh-CN" && !strings.Contains(out, "审核责任请求") || locale == "en-US" && !strings.Contains(out, "Review responsibility requests") {
			t.Fatal(locale, code, out, stderr)
		}
	}
}
