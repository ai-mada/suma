package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// The client uses a real loopback HTTP server, including the production language
// middleware, authorization and error projection. No process-wide locale exists.
func releaseDiagnosticRequest(client *http.Client, base string, cookie *http.Cookie, view Principal, method, path, locale string, body any) (int, map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequest(method, base+"/api/v1"+path, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", locale)
	if cookie != nil && BrowserCommandRequiresView(method, strings.TrimPrefix(path, "/")) {
		req.Header.Set("X-Suma-Expected-User", view.ID)
		req.Header.Set("X-Suma-Expected-Organization", view.TenantID)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	response, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(response.Body).Decode(&out); err != nil {
		return response.StatusCode, nil, err
	}
	if response.Header.Get("Content-Language") != locale {
		return response.StatusCode, out, fmt.Errorf("response locale %q, want %q", response.Header.Get("Content-Language"), locale)
	}
	return response.StatusCode, out, nil
}

func releaseDiagnosticCounts(t *testing.T, f *fixture) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, table := range []string{"suma_sessions", "suma_operations", "suma_release_workers", "suma_artifact_uploads", "suma_artifact_proofs", "suma_release_candidates", "suma_release_plans", "suma_release_deployments", "suma_release_authorizations", "dag_runs"} {
		var count int
		if err := f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		out[table] = count
	}
	return out
}

func TestReleaseDiagnosticsHTTPRejectionsAreLocalizedAndSideEffectFree(t *testing.T) {
	f := newFixture(t)
	server := httptest.NewServer(f.handler)
	defer server.Close()
	before := releaseDiagnosticCounts(t, f)
	for _, example := range []struct {
		path, key, chinese, english string
		body                        map[string]any
	}{
		{"/release-candidates", "release.candidate.name_required", "请填写 1–200 字节的候选名称", "Enter a candidate name of 1 to 200 bytes.", f.body(map[string]any{"name": " "})},
		{"/release-workers", "release.worker.id_invalid", "执行节点标识应为 1–100 位字母、数字、点、短横线或下划线", "The worker ID must contain 1 to 100 letters, digits, dots, hyphens, or underscores.", f.body(map[string]any{"worker_id": "invalid worker", "profiles": []any{}})},
		{"/release-plans", "release.plan.preview_digest_required", "必须确认最新计划预览摘要", "Confirm the digest of the latest plan preview.", f.body(map[string]any{"expected_environment_generation": 0})},
	} {
		for locale, want := range map[string]string{"zh-CN": example.chinese, "en-US": example.english} {
			status, out, err := releaseDiagnosticRequest(server.Client(), server.URL, f.cookie, f.view, "POST", example.path, locale, example.body)
			if err != nil {
				t.Fatal(err)
			}
			detail, ok := out["error"].(map[string]any)
			if status != 400 || !ok || detail["code"] != "invalid_input" || detail["message_key"] != example.key || detail["message"] != want || detail["locale"] != locale || detail["localized"] != true || detail["fallback"] != nil {
				t.Errorf("release rejection lost its precise reason: status=%d locale=%s body=%v", status, locale, out)
			}
		}
	}
	if after := releaseDiagnosticCounts(t, f); !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected translated requests created credentials, execution or release evidence: before=%v after=%v", before, after)
	}
}

func TestReleaseDiagnosticsHTTPConcurrentLanguagesStayIsolated(t *testing.T) {
	f := newFixture(t)
	server := httptest.NewServer(f.handler)
	defer server.Close()
	body := f.body(map[string]any{"worker_id": "invalid worker", "profiles": []any{}})
	before := releaseDiagnosticCounts(t, f)
	var wg sync.WaitGroup
	failures := make(chan error, 24)
	for i := 0; i < 24; i++ {
		locale, want := "en-US", "The worker ID must contain 1 to 100 letters, digits, dots, hyphens, or underscores."
		if i%2 == 0 {
			locale, want = "zh-CN", "执行节点标识应为 1–100 位字母、数字、点、短横线或下划线"
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, out, err := releaseDiagnosticRequest(server.Client(), server.URL, f.cookie, f.view, "POST", "/release-workers", locale, body)
			if err != nil {
				failures <- err
				return
			}
			detail, ok := out["error"].(map[string]any)
			if status != 400 || !ok || detail["code"] != "invalid_input" || detail["message_key"] != "release.worker.id_invalid" || detail["message"] != want || detail["locale"] != locale || detail["localized"] != true {
				failures <- fmt.Errorf("concurrent release locale crossed requests: locale=%s status=%d body=%v", locale, status, out)
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if after := releaseDiagnosticCounts(t, f); !reflect.DeepEqual(before, after) {
		t.Fatalf("concurrent invalid requests changed durable state: before=%v after=%v", before, after)
	}
}

func TestReleaseDiagnosticsHTTPStalePlanPreservesHistoryAndReceipts(t *testing.T) {
	f, _, _, plan, _, _ := verifiedReleaseDeploymentPlan(t)
	ctx := context.Background()
	// A user may deliberately type words that also exist in the product catalog.
	// Persist them through the real approval API; display projection must leave
	// this historical reason and every fixed plan digest byte-for-byte intact.
	const userReason = "计划版本或内容已变化，请刷新后重新审核"
	var err error
	plan, err = f.s.ApproveReleasePlan(ctx, f.actor, f.project, plan.ID, f.body(map[string]any{"expected_version": plan.Version, "plan_revision": plan.Revision, "plan_sha256": plan.SHA256, "decision": "approved", "reason": userReason}))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() []string {
		t.Helper()
		out := []string{}
		for _, query := range []string{"SELECT data FROM suma_release_plans ORDER BY id", "SELECT data FROM suma_release_approvals ORDER BY id", "SELECT response FROM suma_operations ORDER BY operation_id"} {
			rows, err := f.s.DB.Query(query)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var raw string
				if err := rows.Scan(&raw); err != nil {
					rows.Close()
					t.Fatal(err)
				}
				out = append(out, raw)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
		}
		return out
	}
	before, counts := snapshot(), releaseDiagnosticCounts(t, f)
	server := httptest.NewServer(f.handler)
	defer server.Close()
	body := f.body(map[string]any{"expected_version": plan.Version + 1, "plan_revision": plan.Revision, "plan_sha256": plan.SHA256, "decision": "approved", "reason": "retry must not overwrite evidence"})
	for locale, want := range map[string]string{"zh-CN": userReason, "en-US": "The plan version or content has changed. Refresh and review it again."} {
		status, out, err := releaseDiagnosticRequest(server.Client(), server.URL, f.cookie, f.view, "POST", "/release-plans/"+plan.ID+"/approve", locale, body)
		if err != nil {
			t.Fatal(err)
		}
		detail := out["error"].(map[string]any)
		if status != 409 || detail["code"] != "version_conflict" || detail["message_key"] != "release.plan.version_changed" || detail["message"] != want || detail["localized"] != true || detail["locale"] != locale {
			t.Errorf("stale plan diagnostic changed its conflict contract: status=%d locale=%s body=%v", status, locale, out)
		}
		status, out, err = releaseDiagnosticRequest(server.Client(), server.URL, f.cookie, f.view, "GET", "/release-plans/"+plan.ID+"?project_id="+f.project, locale, nil)
		if err != nil || status != 200 || out["sha256"] != plan.SHA256 || out["approval"].(map[string]any)["reason"] != userReason || len(out["approval_history"].([]any)) != 2 {
			t.Fatalf("localized read changed fixed plan or human approval: status=%d body=%v err=%v", status, out, err)
		}
	}
	if after := snapshot(); !reflect.DeepEqual(before, after) {
		t.Fatal("language display changed stored plans, approval history or operation receipts")
	}
	if after := releaseDiagnosticCounts(t, f); !reflect.DeepEqual(counts, after) {
		t.Fatalf("stale plan request created execution, credentials or receipts: before=%v after=%v", counts, after)
	}
}

func TestReleaseDiagnosticProjectionPreservesMachineDetailsAndRawError(t *testing.T) {
	const source = "已发生或未知的副作用不能标为取消成功"
	details := map[string]any{"operation_id": "deploy-fixed-operation", "generation": 7, "artifact_sha256": strings.Repeat("a", 64), "original_provider_error": source}
	original := &APIError{409, "observation_cancel_unproven", source, details}
	before := stringify(original)
	w := httptest.NewRecorder()
	w.Header().Set("Content-Language", "en-US")
	writeError(w, original)
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	detail := out["error"].(map[string]any)
	wantDetails := map[string]any{"operation_id": "deploy-fixed-operation", "generation": float64(7), "artifact_sha256": strings.Repeat("a", 64), "original_provider_error": source}
	if w.Code != 409 || detail["code"] != original.Code || detail["message_key"] != "release.observation.cancel_unproven" || detail["message"] != "Side effects that have occurred or whose outcome is unknown cannot be marked as successfully cancelled." || detail["localized"] != true || !reflect.DeepEqual(detail["details"], wantDetails) {
		t.Fatalf("localization changed the unknown-effect constraint or machine details: %s", w.Body.String())
	}
	if original.Error() != source || stringify(original) != before {
		t.Fatal("projection mutated the original API error or its evidence")
	}
}

func TestReleaseDiagnosticsHTTPPublicCatalogCanRedisplayFixedReason(t *testing.T) {
	f := newFixture(t)
	server := httptest.NewServer(f.handler)
	defer server.Close()
	status, out, err := releaseDiagnosticRequest(server.Client(), server.URL, nil, Principal{}, "GET", "/localization/catalog", "en-US", nil)
	if err != nil || status != 200 {
		t.Fatalf("public catalog unavailable: status=%d err=%v", status, err)
	}
	count := 0
	var conflict map[string]any
	for _, value := range out["messages"].([]any) {
		message := value.(map[string]any)
		key := message["key"].(string)
		if strings.HasPrefix(key, "release.") {
			count++
		}
		if key == "release.plan.version_changed" {
			conflict = message
		}
	}
	if count != 146 || conflict == nil || len(conflict["parameters"].([]any)) != 0 {
		t.Fatalf("public release catalog is incomplete: count=%d conflict=%v", count, conflict)
	}
	texts := conflict["texts"].(map[string]any)
	if texts["zh-CN"] != "计划版本或内容已变化，请刷新后重新审核" || texts["en-US"] != "The plan version or content has changed. Refresh and review it again." {
		t.Fatalf("the fixed conflict reason cannot be redisplayed in both languages: %v", conflict)
	}
}
