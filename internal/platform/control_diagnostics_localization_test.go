package platform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
)

func TestControlDiagnosticsConcurrentHTTPRejectionsPreserveBusinessState(t *testing.T) {
	f := newFixture(t)
	const userText = "标题长度应为 1–500 个字节"
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": userText, "description": "任务完成须经过测试证据与人工验收"}), 201)
	snapshot := func() string {
		t.Helper()
		var counts [4]int
		for i, table := range []string{"suma_tasks", "suma_operations", "suma_activity", "suma_task_plans"} {
			if err := f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&counts[i]); err != nil {
				t.Fatal(err)
			}
		}
		return stringify(map[string]any{"counts": counts, "task": f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)})
	}
	before := snapshot()
	server := httptest.NewServer(f.handler)
	defer server.Close()

	tests := []struct {
		method, path, code, key, zh, en string
		body                            map[string]any
		status                          int
	}{
		{"POST", "/tasks", "invalid_input", "control.records.task_completion_evidence_required", "任务完成须经过测试证据与人工验收", "Completing a task requires test evidence and human acceptance.", map[string]any{"project_id": f.project, "idempotency_key": "control-denied-create", "title": "拒写草稿", "status": "accepted"}, 400},
		{"PATCH", "/tasks/" + task["id"].(string), "version_conflict", "control.records.version_changed", "记录已变化，请保留草稿并重新加载", "The record has changed. Keep your draft and reload.", map[string]any{"project_id": f.project, "idempotency_key": "control-denied-update", "expected_version": 99, "title": "不应保存"}, 409},
	}
	var wg sync.WaitGroup
	for _, tc := range tests {
		for i := 0; i < 12; i++ {
			locale, want := "zh-CN", tc.zh
			if i%2 == 1 {
				locale, want = "en-US", tc.en
			}
			wg.Add(1)
			go func(locale, want string) {
				defer wg.Done()
				raw, err := json.Marshal(tc.body)
				if err != nil {
					t.Error(err)
					return
				}
				req, err := http.NewRequest(tc.method, server.URL+"/api/v1"+tc.path, bytes.NewReader(raw))
				if err != nil {
					t.Error(err)
					return
				}
				req.AddCookie(f.cookie)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept-Language", locale)
				resp, err := server.Client().Do(req)
				if err != nil {
					t.Error(err)
					return
				}
				defer resp.Body.Close()
				var body struct {
					Error map[string]any `json:"error"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				d := body.Error
				if resp.StatusCode != tc.status || resp.Header.Get("Content-Language") != locale || d["code"] != tc.code || d["message_key"] != tc.key || d["locale"] != locale || d["localized"] != true || d["message"] != want || d["details"] != nil || d["fallback"] != nil {
					t.Errorf("%s %s language=%s status=%d diagnostic=%v", tc.method, tc.path, locale, resp.StatusCode, d)
				}
			}(locale, want)
		}
	}
	wg.Wait()
	if after := snapshot(); after != before {
		t.Fatalf("localized rejection changed task, plan, receipt or activity: before=%s after=%s", before, after)
	}
	for _, locale := range []string{"zh-CN", "en-US"} {
		req, _ := http.NewRequest("GET", server.URL+"/api/v1/tasks/"+task["id"].(string)+"?project_id="+f.project, nil)
		req.AddCookie(f.cookie)
		req.Header.Set("Accept-Language", locale)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var current map[string]any
		err = json.NewDecoder(resp.Body).Decode(&current)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 || current["title"] != userText || current["description"] != task["description"] || current["version"] != task["version"] {
			t.Fatalf("user-authored diagnostic text or version changed for %s: %v %v", locale, current, err)
		}
	}
}

func TestControlDiagnosticHTTPProjectionPreservesDetailsAndUnknownSource(t *testing.T) {
	details := map[string]any{"project_id": "project-original", "expected_version": 0, "accepted": false, "reason": "任务完成须经过测试证据与人工验收"}
	known := &APIError{409, "version_conflict", "记录已变化，请保留草稿并重新加载", details}
	unknown := &APIError{400, "invalid_input", "标题长度应为 1–500 个字节；外部校验器原始补充", details}
	before := stringify([]any{known, unknown})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setResponseLanguage(w, r)
		if r.URL.Path == "/known" {
			writeError(w, fmt.Errorf("wrapped: %w", known))
			return
		}
		writeError(w, unknown)
	}))
	defer server.Close()
	for _, tc := range []struct {
		path, locale, text, key, fallback, code string
		status                                  int
		localized                               bool
	}{
		{"/known", "en-US", "The record has changed. Keep your draft and reload.", "control.records.version_changed", "", "version_conflict", 409, true},
		{"/known", "zh-CN", "记录已变化，请保留草稿并重新加载", "control.records.version_changed", "", "version_conflict", 409, true},
		{"/unknown", "en-US", unknown.Message, "legacy.invalid_input", "source_untranslated", "invalid_input", 400, false},
		{"/unknown", "zh-CN", unknown.Message, "legacy.invalid_input", "source_untranslated", "invalid_input", 400, false},
	} {
		t.Run(tc.path+"/"+tc.locale, func(t *testing.T) {
			req, _ := http.NewRequest("GET", server.URL+tc.path, nil)
			req.Header.Set("Accept-Language", tc.locale)
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var body struct {
				Error map[string]any `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			d := body.Error
			if resp.StatusCode != tc.status || d["code"] != tc.code || d["message_key"] != tc.key || d["message"] != tc.text || d["localized"] != tc.localized || d["locale"] != tc.locale || resp.Header.Get("Content-Language") != tc.locale {
				t.Fatalf("diagnostic projection changed contract: status=%d body=%v", resp.StatusCode, d)
			}
			if tc.fallback == "" && d["fallback"] != nil || tc.fallback != "" && d["fallback"] != tc.fallback {
				t.Fatalf("wrong fallback: %v", d)
			}
			wantDetails := map[string]any{"project_id": "project-original", "expected_version": float64(0), "accepted": false, "reason": "任务完成须经过测试证据与人工验收"}
			if !reflect.DeepEqual(d["details"], wantDetails) || d["message_params"] != nil {
				t.Fatalf("details or undeclared parameters changed: %v", d)
			}
		})
	}
	if after := stringify([]any{known, unknown}); after != before {
		t.Fatal("HTTP projection mutated original diagnostic or details")
	}
}
