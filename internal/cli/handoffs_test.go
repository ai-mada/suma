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

func TestHandoffsRequestContract(t *testing.T) {
	for _, scenario := range []struct {
		name, method, path, query, locale string
		args                              []string
		body                              map[string]any
	}{
		{"list", "GET", "/api/v1/task-handoffs", "before=previous-id&limit=5&project_id=project-one&task_id=task-1", "en-US", []string{"handoffs", "list", "--task", "task-1", "--limit", "5", "--before", "previous-id"}, nil},
		{"create", "POST", "/api/v1/task-handoffs", "project_id=project-one", "zh-CN", []string{"handoffs", "create", "--task", "task-1", "--version", "12", "--to-resource", "human-1", "--to-team", "team-1", "--note", "中文 unchanged", "--operation", "original-create-key"}, map[string]any{"project_id": "project-one", "task_id": "task-1", "expected_task_version": json.Number("12"), "to_resource_id": "human-1", "to_team_id": "team-1", "note": "中文 unchanged", "idempotency_key": "original-create-key"}},
		{"accept", "POST", "/api/v1/task-handoffs/handoff-1/commands", "project_id=project-one", "en-US", []string{"handoffs", "accept", "handoff-1", "--version", "2", "--reason", "明确接收 raw", "--operation", "original-accept-key", "--acknowledge-invalidation"}, map[string]any{"project_id": "project-one", "expected_version": json.Number("2"), "action": "accept", "reason": "明确接收 raw", "idempotency_key": "original-accept-key", "acknowledge_invalidation": true}},
		{"return", "POST", "/api/v1/task-handoffs/handoff-1/commands", "project_id=project-one", "zh-CN", []string{"handoffs", "return", "handoff-1", "--version", "2", "--reason", "退回补充 raw", "--operation", "original-return-key"}, map[string]any{"project_id": "project-one", "expected_version": json.Number("2"), "action": "return", "reason": "退回补充 raw", "idempotency_key": "original-return-key", "acknowledge_invalidation": false}},
		{"withdraw", "POST", "/api/v1/task-handoffs/handoff-1/commands", "project_id=project-one", "en-US", []string{"handoffs", "withdraw", "handoff-1", "--version", "2", "--reason", "撤回 raw", "--operation", "original-withdraw-key"}, map[string]any{"project_id": "project-one", "expected_version": json.Number("2"), "action": "withdraw", "reason": "撤回 raw", "idempotency_key": "original-withdraw-key", "acknowledge_invalidation": false}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != scenario.method || r.URL.Path != scenario.path || r.URL.RawQuery != scenario.query || r.Header.Get("Accept-Language") != scenario.locale || r.Header.Get("Authorization") != "Bearer fixture-token" {
					t.Errorf("wrong handoff route: %s %s", r.Method, r.URL)
				}
				if scenario.body == nil {
					if r.ContentLength > 0 || r.Header.Get("X-Suma-Expected-User") != "" {
						t.Error("read acquired a write body or identity header")
					}
				} else {
					var body map[string]any
					decoder := json.NewDecoder(r.Body)
					decoder.UseNumber()
					if err := decoder.Decode(&body); err != nil || !reflect.DeepEqual(body, scenario.body) {
						t.Errorf("handoff body changed: %+v %v", body, err)
					}
					if r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
						t.Error("original account view was not retained")
					}
				}
				_, _ = w.Write([]byte(`{"id":"handoff-1","version":9007199254740993,"note":"用户原文 unchanged","allowed_actions":[],"basis_current":false}`))
			}))
			defer server.Close()
			code, out, stderr := contextInvoke(t, server.URL, scenario.locale, "", scenario.args...)
			if code != 0 || calls != 1 || stderr != "" || !strings.Contains(out, "9007199254740993") || !strings.Contains(out, "用户原文 unchanged") || !strings.Contains(out, `"basis_current": false`) {
				t.Fatalf("code=%d calls=%d output=%s error=%s", code, calls, out, stderr)
			}
		})
	}
	t.Run("acknowledgement-and-required-write-options", func(t *testing.T) {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
		defer server.Close()
		base := []string{"handoffs", "accept", "handoff-1", "--version", "1", "--reason", "核对原依据", "--operation", "original-accept-key"}
		for _, args := range [][]string{base, append(append([]string{}, base...), "--acknowledge-invalidation=false"), {"handoffs", "create", "--task", "task-1", "--version", "1", "--to-resource", "human-1", "--note", "说明"}, {"handoffs", "withdraw", "handoff-1", "--version", "0", "--reason", "说明", "--operation", "original-key"}} {
			code, _, stderr := contextInvoke(t, server.URL, "zh-CN", "", args...)
			if code != 2 || stderr == "" {
				t.Fatalf("missing confirmation or version/key accepted: %d %s", code, stderr)
			}
		}
		if calls != 0 {
			t.Fatal("invalid write reached the control plane")
		}
		if !requestIsReadOnly("GET", "/api/v1/task-handoffs?task_id=t") || requestIsReadOnly("POST", "/api/v1/task-handoffs") || requestIsReadOnly("POST", "/api/v1/task-handoffs/h/commands") {
			t.Fatal("handoff mutations were classified as read-only")
		}
	})
	t.Run("unknown-result-keeps-one-explicit-operation", func(t *testing.T) {
		var bodies []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			bodies = append(bodies, string(raw))
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			connection.Close()
		}))
		defer server.Close()
		args := []string{"handoffs", "accept", "handoff-1", "--version", "1", "--reason", "same原文", "--operation", "one-original-key", "--acknowledge-invalidation"}
		for _, locale := range []string{"en-US", "zh-CN"} {
			code, _, stderr := contextInvoke(t, server.URL, locale, "", args...)
			if code != 1 || !strings.Contains(stderr, "one-original-key") && !strings.Contains(stderr, "outcome may be unknown") && !strings.Contains(stderr, "结果可能未知") {
				t.Fatalf("unknown write not reported: %d %s", code, stderr)
			}
		}
		if len(bodies) != 2 || bodies[0] != bodies[1] || !strings.Contains(bodies[0], `"idempotency_key":"one-original-key"`) {
			t.Fatalf("implicit retry or changed manual retry payload: %v", bodies)
		}
	})
	t.Run("bilingual-help-and-authoritative-error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"服务端原文 denied","details":{"version":9007199254740993}}}`))
		}))
		defer server.Close()
		for _, locale := range []string{"zh-CN", "en-US"} {
			code, out, stderr := contextInvoke(t, server.URL, locale, "", "handoffs", "--help")
			if code != 0 || stderr != "" || !strings.Contains(out, "--acknowledge-invalidation") || locale == "zh-CN" && !strings.Contains(out, "任务交接") || locale == "en-US" && !strings.Contains(out, "Task handoffs") {
				t.Fatalf("missing bilingual help: %d %s %s", code, out, stderr)
			}
			code, _, stderr = contextInvoke(t, server.URL, locale, "", "handoffs", "return", "handoff-1", "--version", "1", "--reason", "原意见", "--operation", "original-return-key")
			if code != 1 || !strings.Contains(stderr, "服务端原文 denied") || !strings.Contains(stderr, "9007199254740993") {
				t.Fatalf("server error changed: %d %s", code, stderr)
			}
		}
	})
}
