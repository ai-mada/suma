package platform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func executionDiagnosticHTTP(server *httptest.Server, f *fixture, method, path, locale string, body any) (int, http.Header, map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, nil, nil, err
	}
	req, err := http.NewRequest(method, server.URL+path, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", locale)
	if f != nil && f.cookie != nil {
		req.AddCookie(f.cookie)
	}
	res, err := server.Client().Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer res.Body.Close()
	var out map[string]any
	err = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, res.Header, out, err
}

func executionDiagnosticMatches(status int, header http.Header, out map[string]any, locale string, wantStatus int, code, key, text string) error {
	detail, ok := out["error"].(map[string]any)
	if status != wantStatus || header.Get("Content-Language") != locale || !ok || detail["code"] != code || detail["message_key"] != key || detail["message"] != text || detail["locale"] != locale || detail["localized"] != true || detail["fallback"] != nil || detail["details"] != nil {
		return fmt.Errorf("execution diagnostic changed meaning or machine fields: locale=%s status=%d body=%v", locale, status, out)
	}
	return nil
}

func TestExecutionLocalizationConcurrentHTTPDenialsCannotTriggerRuns(t *testing.T) {
	f, rule := automationFixture(t, "forbid", "manual", "skip")
	server := httptest.NewServer(f.handler)
	defer server.Close()
	// A missing catalog entry must not turn a draft rule into an enabled rule,
	// create an occurrence, or leak the concurrent request's language.
	body := f.body(map[string]any{"action": "trigger", "expected_version": rule.Version})
	var group sync.WaitGroup
	failures := make(chan error, 24)
	for i := 0; i < 24; i++ {
		locale, want := "en-US", "Only enabled rules can be triggered manually."
		if i%2 == 0 {
			locale, want = "zh-CN", "仅已启用规则可人工触发"
		}
		group.Add(1)
		go func() {
			defer group.Done()
			status, header, out, err := executionDiagnosticHTTP(server, f, "POST", "/api/v1/automations/"+rule.ID+"/commands", locale, body)
			if err == nil {
				err = executionDiagnosticMatches(status, header, out, locale, 409, "version_conflict", "execution.automation.trigger_requires_enabled", want)
			}
			if err != nil {
				failures <- err
			}
		}()
	}
	group.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	for _, table := range []string{"dag_runs", "suma_automation_occurrences"} {
		if count := autoCount(t, f, table); count != 0 {
			t.Errorf("rejected language requests created %s: %d", table, count)
		}
	}
	current := f.call("GET", "/automations/"+rule.ID+"?project_id="+f.project, nil, 200)
	if current["version"] != float64(rule.Version) || current["state"] != "draft" || current["revision"] != float64(rule.Revision) {
		t.Fatal("rejected triggers changed pinned automation state", current)
	}
}

func TestExecutionLocalizationHTTPValidationRetainsExactStatusAndCode(t *testing.T) {
	f := newFixture(t)
	server := httptest.NewServer(f.handler)
	defer server.Close()
	for _, example := range []struct {
		method, path string
		body         any
		status       int
		code, key    string
		zh, en       string
	}{
		{"GET", "/api/v1/automations?project_id=" + f.project + "&limit=201", nil, 400, "invalid_input", "execution.pagination.limit_200", "limit 必须为 1..200", "limit must be between 1 and 200."},
		{"POST", "/api/v1/task-dispatch-plans", f.body(map[string]any{}), 400, "invalid_input", "execution.dispatch.preview_fields_required", "派发预览需要项目、任务版本、固定流程、开发节点及工具版本", "A dispatch preview requires the project, task version, pinned workflow, development node, and tool version."},
		{"POST", "/api/v1/scheduling/policy-previews", f.body(map[string]any{}), 400, "invalid_input", "execution.scheduling.policy_fields_required", "请显式提供 expected_version、concurrency_limit、resource_limits 及 budget_limit（null 表示不限制）", "Explicitly provide expected_version, concurrency_limit, resource_limits, and budget_limit (null means unlimited)."},
		{"GET", "/api/v1/runs/unused/events/stream?project_id=" + f.project + "&after=-1", nil, 400, "invalid_input", "execution.stream.cursor_invalid", "事件游标必须为非负安全整数", "The event cursor must be a non-negative safe integer."},
		{"GET", "/api/v1/runs/unused/events/stream?project_id=" + f.project + "&after=0&after=0", nil, 400, "invalid_input", "execution.stream.cursor_duplicate", "事件游标不可重复", "The event cursor must not be repeated."},
	} {
		t.Run(example.key, func(t *testing.T) {
			for locale, text := range map[string]string{"zh-CN": example.zh, "en-US": example.en} {
				status, header, out, err := executionDiagnosticHTTP(server, f, example.method, example.path, locale, example.body)
				if err == nil {
					err = executionDiagnosticMatches(status, header, out, locale, example.status, example.code, example.key, text)
				}
				if err != nil {
					t.Error(err)
				}
			}
		})
	}
	for _, table := range []string{"dag_runs", "suma_task_dispatch_plans", "suma_scheduling_previews"} {
		if count := autoCount(t, f, table); count != 0 {
			t.Errorf("invalid requests created %s: %d", table, count)
		}
	}
}

func TestExecutionLocalizationHTTPPreservesUnknownSourceAndDetails(t *testing.T) {
	const source = "仅已启用规则可人工触发: 用户原始工具诊断"
	details := map[string]any{"run_id": "run-fixed", "attempt": float64(0), "output": "仅已启用规则可人工触发", "state": "unknown"}
	api := &APIError{409, "version_conflict", source, details}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setResponseLanguage(w, r)
		writeError(w, api)
	}))
	defer server.Close()
	for _, locale := range []string{"en-US", "zh-CN"} {
		status, header, out, err := executionDiagnosticHTTP(server, nil, "GET", "/", locale, nil)
		if err != nil {
			t.Fatal(err)
		}
		detail := out["error"].(map[string]any)
		if status != 409 || header.Get("Content-Language") != locale || detail["message"] != source || detail["code"] != "version_conflict" || detail["message_key"] != "legacy.version_conflict" || detail["locale"] != locale || detail["localized"] != false || detail["fallback"] != "source_untranslated" || !reflect.DeepEqual(detail["details"], details) {
			t.Fatalf("unknown diagnostic was translated or machine details changed: %d %v", status, out)
		}
	}
	if api.Message != source || !reflect.DeepEqual(api.Details, details) {
		t.Fatal("HTTP display modified the original API error", api)
	}
}

func TestExecutionLocalizationHTTPPreservesStoredRunnerOutput(t *testing.T) {
	f := newFixture(t)
	run := streamRun(t, f)
	const raw = "仅已启用规则可人工触发"
	if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET output=?,error_text=? WHERE run_id=? AND id='one'`, raw, raw, run.ID); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(f.handler)
	defer server.Close()
	for _, locale := range []string{"en-US", "zh-CN"} {
		status, header, out, err := executionDiagnosticHTTP(server, f, "GET", "/api/v1/runs/"+run.ID+"?project_id="+f.project, locale, nil)
		if err != nil || status != 200 || header.Get("Content-Language") != locale {
			t.Fatalf("read runner output: status=%d body=%v err=%v", status, out, err)
		}
		node := out["nodes"].([]any)[0].(map[string]any)
		if node["output"] != raw || node["error"] != raw || out["id"] != run.ID || out["workflow_version"] != float64(run.WorkflowVersion) {
			t.Fatal("known catalog source inside runner output or pinned version was rewritten", out)
		}
	}
	var output, diagnostic string
	if err := f.s.DB.QueryRow(`SELECT output,error_text FROM dag_nodes WHERE run_id=? AND id='one'`, run.ID).Scan(&output, &diagnostic); err != nil || output != raw || diagnostic != raw {
		t.Fatalf("HTTP language changed durable runner evidence: %q %q %v", output, diagnostic, err)
	}
	if count := autoCount(t, f, "dag_runs"); count != 1 {
		t.Fatal("reading translated diagnostics created a run", count)
	}
}

func TestExecutionLocalizationPublicWebhookDenialDoesNotRequireSession(t *testing.T) {
	f := newFixture(t)
	server := httptest.NewServer(f.handler)
	defer server.Close()
	for locale, text := range map[string]string{"en-US": "The event signature is invalid or its credential is unavailable.", "zh-CN": "事件签名无效或凭据不可用"} {
		status, header, out, err := executionDiagnosticHTTP(server, nil, "POST", "/api/v1/hooks/"+strings.Repeat("a", 32), locale, map[string]any{})
		if err == nil {
			err = executionDiagnosticMatches(status, header, out, locale, 401, "invalid_signature", "execution.ingress.signature_invalid", text)
		}
		if err != nil {
			t.Error(err)
		}
	}
	if count := autoCount(t, f, "dag_runs"); count != 0 {
		t.Fatal("invalid public delivery created an execution", count)
	}
}
