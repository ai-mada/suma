package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTaskDeliveryCLIReadsExactScopedDelivery(t *testing.T) {
	const task = "Task.v1"
	const project = "project-one"
	const response = "{\"project_id\":\"project-one\",\"task_id\":\"Task.v1\",\"task_version\":8,\"formal_acceptance\":false,\"run\":{\"run_id\":\"original-run\"},\"issues\":{\"scope\":\"current_run\",\"total\":1}}"
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/tasks/"+task+"/delivery" ||
			r.URL.RawQuery != "project_id="+project || r.Header.Get("Authorization") != "Bearer test-secret" ||
			r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) ||
			r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
			t.Error("unexpected scoped read", r.Method, r.URL)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || len(body) != 0 {
			t.Error("delivery GET carried a request body", err)
		}
		_, _ = io.WriteString(w, response)
	}))
	defer server.Close()
	code, out, stderr := invoke(t, server.URL, "--expected-user", strings.Repeat("a", 32), "--expected-organization", strings.Repeat("b", 32), "task", "delivery", task)
	if code != 0 || calls.Load() != 1 || stderr != "" {
		t.Fatal("exact scoped delivery read failed", code, calls.Load(), stderr)
	}
	var got, want map[string]any
	if json.Unmarshal([]byte(out), &got) != nil || json.Unmarshal([]byte(response), &want) != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("delivery projection changed", out)
	}
}

func TestTaskDeliveryCLIRejectsMalformedArgumentsAndPreservesServerRefusal(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "{\"error\":{\"code\":\"forbidden\",\"message\":\"access denied\"}}")
	}))
	defer server.Close()
	for _, invalid := range [][]string{
		{}, {""}, {"."}, {".."}, {"a..b"}, {"a/../b"}, {"a%2fb"}, {"a?project_id=other"}, {"-task"},
		{" task"}, {"a\nb"}, {"任务"}, {strings.Repeat("a", 65)}, {"task-a", "task-b"}, {"task-a", "--limit", "1"},
	} {
		if code, out, stderr := invoke(t, server.URL, append([]string{"task", "delivery"}, invalid...)...); code != 2 || out != "" || !strings.Contains(stderr, "cli.task_delivery.usage") {
			t.Errorf("invalid arguments %q were not refused locally: code=%d", invalid, code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid inputs reached the server", calls.Load())
	}
	code, out, stderr := invoke(t, server.URL, "task", "delivery", "task-a")
	if code != 1 || out != "" || !strings.Contains(stderr, "\"forbidden\"") || calls.Load() != 1 {
		t.Fatal("server refusal did not remain a read error", code, calls.Load(), stderr)
	}
}

func TestTaskDeliveryCLIHelpLanguages(t *testing.T) {
	for _, test := range []struct{ locale, label string }{{"en-US", "delivery evidence"}, {"zh-CN", "任务交付证据"}} {
		for _, args := range [][]string{{"--help"}, {"task", "delivery", "--help"}} {
			var out, stderr strings.Builder
			code := Execute(context.Background(), append([]string{"--language", test.locale}, args...), strings.NewReader(""), &out, &stderr)
			if code != 0 || !strings.Contains(out.String(), "task delivery TASK") || !strings.Contains(out.String(), test.label) || stderr.Len() != 0 {
				t.Errorf("%s %v help missing task delivery: code=%d", test.locale, args, code)
			}
		}
	}
}
