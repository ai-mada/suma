package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestTaskBatchCLIRoutesAndOriginalRequest(t *testing.T) {
	const tasks = `"tasks":[{"task_id":"task-b","expected_version":2},{"task_id":"task-a","expected_version":1}]`
	const priorityPreview = `{"project_id":"project-one","items":[{"task_id":"task-b","version":2,"previous_priority":"low","priority":"urgent","changed":true},{"task_id":"task-a","version":1,"previous_priority":"urgent","priority":"urgent","changed":false}]}`
	const statusPreview = `{"project_id":"project-one","items":[{"task_id":"task-b","version":2,"status":"todo","eligibility":"editable"},{"task_id":"task-a","version":1,"status":"accepted","eligibility":"workflow_status"}]}`
	const priorityReceipt = `{"project_id":"project-one","operation_id":"original-operation","changes":[{"task_id":"task-b","previous_version":2,"version":3,"previous_priority":"low","priority":"urgent","changed":true},{"task_id":"task-a","previous_version":1,"version":1,"previous_priority":"urgent","priority":"urgent","changed":false}]}`
	const statusReceipt = `{"project_id":"project-one","operation_id":"original-operation","changes":[{"task_id":"task-b","previous_version":2,"version":3,"previous_status":"todo","status":"blocked","changed":true},{"task_id":"task-a","previous_version":1,"version":1,"previous_status":"blocked","status":"blocked","changed":false}]}`
	for _, tc := range []struct{ command, action, path, input, response string }{
		{"task-priority", "preview", "/api/v1/task-priority-changes/preview", `{"priority":"urgent",` + tasks + `}`, priorityPreview},
		{"task-status", "preview", "/api/v1/task-planning-status-changes/preview", `{"project_id":"project-one",` + tasks + `}`, statusPreview},
		{"task-priority", "apply", "/api/v1/task-priority-changes", `{"priority":"urgent","idempotency_key":"original-operation",` + tasks + `}`, priorityReceipt},
		{"task-status", "apply", "/api/v1/task-planning-status-changes", `{"status":"blocked","idempotency_key":"original-operation",` + tasks + `}`, statusReceipt},
	} {
		t.Run(tc.command+"/"+tc.action, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost || r.URL.Path != tc.path || r.URL.RawQuery != "" ||
					r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) ||
					r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) ||
					r.Header.Get("Authorization") != "Bearer fixture-token" {
					t.Errorf("wrong route, scope or original-view headers")
				}
				var got, want map[string]any
				if json.NewDecoder(r.Body).Decode(&got) != nil || json.Unmarshal([]byte(tc.input), &want) != nil {
					t.Error("unable to decode command")
					return
				}
				want["project_id"] = "project-one"
				if !reflect.DeepEqual(got, want) {
					t.Errorf("original ordered request was changed: %v", got)
				}
				io.WriteString(w, tc.response)
			}))
			defer server.Close()
			code, out, stderr := contextInvoke(t, server.URL, "en-US", tc.input, tc.command, tc.action, "--file", "-")
			var got, want map[string]any
			if code != 0 || calls != 1 || json.Unmarshal([]byte(out), &got) != nil || json.Unmarshal([]byte(tc.response), &want) != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("exit=%d calls=%d output=%s error=%s", code, calls, out, stderr)
			}
		})
	}
}

func TestTaskBatchCLIRejectsAmbiguityBeforeNetwork(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	const tasks = `"tasks":[{"task_id":"task-a","expected_version":1}]`
	for _, tc := range []struct{ name, command, action, body string }{
		{"missing-operation", "task-status", "apply", `{"status":"blocked",` + tasks + `}`},
		{"duplicate-field", "task-status", "preview", `{"project_id":"other","project_id":"project-one",` + tasks + `}`},
		{"duplicate-nested-field", "task-status", "preview", `{"tasks":[{"task_id":"task-a","task_id":"task-b","expected_version":1}]}`},
		{"duplicate-task", "task-status", "preview", `{"tasks":[{"task_id":"task-a","expected_version":1},{"task_id":"task-a","expected_version":2}]}`},
		{"fractional-version", "task-priority", "preview", `{"priority":"low","tasks":[{"task_id":"task-a","expected_version":1.0}]}`},
		{"wrong-project", "task-status", "preview", `{"project_id":"other",` + tasks + `}`},
		{"unknown-field", "task-status", "preview", `{"extra":false,` + tasks + `}`},
		{"explicit-null", "task-status", "preview", `{"project_id":null,` + tasks + `}`},
		{"preview-key", "task-priority", "preview", `{"priority":"low","idempotency_key":"original-operation",` + tasks + `}`},
		{"formal-status", "task-status", "apply", `{"status":"accepted","idempotency_key":"original-operation",` + tasks + `}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, out, stderr := contextInvoke(t, server.URL, "zh-CN", tc.body, tc.command, tc.action, "--file", "-")
			if code != 2 || calls != 0 || out != "" || !strings.Contains(stderr, "批量") {
				t.Fatalf("exit=%d calls=%d output=%s error=%s", code, calls, out, stderr)
			}
		})
	}
}

func TestTaskBatchCLIMismatchedReceiptStaysUnconfirmed(t *testing.T) {
	const request = `{"status":"blocked","idempotency_key":"original-operation","tasks":[{"task_id":"task-a","expected_version":1}]}`
	const valid = `{"project_id":"project-one","operation_id":"original-operation","changes":[{"task_id":"task-a","previous_version":1,"version":2,"previous_status":"todo","status":"blocked","changed":true}]}`
	for _, tc := range []struct{ field, response string }{
		{"project", strings.Replace(valid, "project-one", "project-other", 1)},
		{"operation", strings.Replace(valid, "original-operation", "fixture-token", 1)},
		{"task", strings.Replace(valid, "task-a", "task-b", 1)},
		{"version", strings.Replace(valid, `"version":2`, `"version":3`, 1)},
		{"target", strings.Replace(valid, `"status":"blocked"`, `"status":"todo"`, 1)},
		{"changed", strings.Replace(valid, "true", "false", 1)},
		{"malformed", "{"},
	} {
		t.Run(tc.field, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, tc.response)
			}))
			defer server.Close()
			code, out, stderr := contextInvoke(t, server.URL, "en-US", request, "task-status", "apply", "--file", "-")
			if code != 1 || out != "" || !strings.Contains(stderr, "unconfirmed") || !strings.Contains(stderr, "same operation") || strings.Contains(stderr, "fixture-token") {
				t.Fatalf("exit=%d output=%s error=%s", code, out, stderr)
			}
		})
	}
}

func TestTaskBatchCLIPreviewTransportAndDiscovery(t *testing.T) {
	for _, path := range []string{"/api/v1/task-priority-changes", "/api/v1/task-planning-status-changes"} {
		if !requestIsReadOnly(http.MethodPost, path+"/preview") || requestIsReadOnly(http.MethodPost, path) {
			t.Fatalf("wrong transport outcome policy for %s", path)
		}
	}
	for _, locale := range []string{"zh-CN", "en-US"} {
		code, out, _ := contextInvoke(t, "http://127.0.0.1:1", locale, "", "--help")
		if code != 0 || !strings.Contains(out, "task-priority") || !strings.Contains(out, "task-status") {
			t.Fatal("missing global command discovery")
		}
		code, out, _ = contextInvoke(t, "http://127.0.0.1:1", locale, "", "task-status", "--help")
		if code != 0 || !strings.Contains(out, "preview --file") || !strings.Contains(out, "apply --file") {
			t.Fatal("missing subcommand discovery")
		}
	}
}
