package platform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func workspaceLocalizedCall(f *fixture, method, path, locale string, body any) (int, map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	r := httptest.NewRequest(method, "http://suma.test/api/v1"+path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept-Language", locale)
	if BrowserCommandRequiresView(method, strings.TrimPrefix(path, "/")) {
		r.Header.Set("X-Suma-Expected-User", f.view.ID)
		r.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
	}
	if f.cookie != nil {
		r.AddCookie(f.cookie)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	var out map[string]any
	if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		return w.Code, nil, err
	}
	if w.Header().Get("Content-Language") != locale {
		return w.Code, out, fmt.Errorf("response locale %q, want %q", w.Header().Get("Content-Language"), locale)
	}
	return w.Code, out, nil
}

func assertWorkspaceLocalizedError(t *testing.T, f *fixture, method, path string, body any, status int, code, key, chinese, english string) {
	t.Helper()
	for locale, want := range map[string]string{"zh-CN": chinese, "en-US": english} {
		gotStatus, out, err := workspaceLocalizedCall(f, method, path, locale, body)
		if err != nil {
			t.Fatal(err)
		}
		detail, ok := out["error"].(map[string]any)
		if gotStatus != status || !ok || detail["code"] != code || detail["message_key"] != key || detail["message"] != want || detail["locale"] != locale || detail["localized"] != true || detail["fallback"] != nil {
			t.Fatalf("%s diagnostic changed meaning/status/key: status=%d body=%v", locale, gotStatus, out)
		}
	}
}

func TestWorkspaceLocalizationUnconfiguredKeepsOriginalReason(t *testing.T) {
	f := newFixture(t)
	t.Setenv("SUMA_WORKSPACE_ROOT", "")
	for _, configured := range []bool{false, true} {
		wantReason, wantAvailable := "当前身份没有此操作权限", false
		if configured {
			t.Setenv("SUMA_WORKSPACE_ROOT", t.TempDir())
			wantReason, wantAvailable = "请先配置当前项目的工作区", true
		}
		for _, locale := range []string{"en-US", "zh-CN"} {
			status, out, err := workspaceLocalizedCall(f, "GET", "/workspace?project_id="+f.project, locale, nil)
			if err != nil || status != 200 || out["configured"] != false || out["binding_available"] != wantAvailable || out["reason"] != wantReason || out["files"] != nil || out["root"] != nil {
				t.Fatalf("unconfigured state became healthy or lost its original reason: %d %v %v", status, out, err)
			}
		}
	}
	assertWorkspaceLocalizedError(t, f, "GET", "/workspace/file?project_id="+f.project+"&path=main.go", nil, 422, "workspace_not_configured", "workspace.project_not_configured", "请先配置当前项目的工作区", "Configure a workspace for the current project first.")
	var bindings, operations int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_workspace_bindings`).Scan(&bindings); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_workspace_file_operations`).Scan(&operations); err != nil || bindings != 0 || operations != 0 {
		t.Fatalf("read created workspace state: bindings=%d operations=%d err=%v", bindings, operations, err)
	}
}

func TestWorkspaceLocalizationReadSaveAndBindingDiagnostics(t *testing.T) {
	f, root := workspaceFixture(t)
	original := f.call("GET", "/workspace/file?project_id="+f.project+"&path=main.go", nil, 200)
	if err := os.WriteFile(filepath.Join(root, "binary.txt"), []byte{0}, 0600); err != nil {
		t.Fatal(err)
	}
	assertWorkspaceLocalizedError(t, f, "GET", "/workspace/file?project_id="+f.project+"&path=binary.txt", nil, 400, "invalid_input", "workspace.read.text_limit", "编辑器仅打开 1 MiB 以内的文本文件", "The editor can only open text files of at most 1 MiB.")
	assertWorkspaceLocalizedError(t, f, "POST", "/workspace/file", f.body(map[string]any{"path": "main.go", "idempotency_key": "short", "expected_hash": original["hash"], "expected_context_sha256": original["context_sha256"], "content": "用户内容"}), 400, "invalid_input", "workspace.save.operation_invalid", "请提供 8–128 字符的 idempotency_key", "Provide an idempotency_key of 8 to 128 characters.")
	assertWorkspaceLocalizedError(t, f, "POST", "/workspace/file", f.body(map[string]any{"path": "main.go", "expected_hash": strings.Repeat("0", 64), "expected_context_sha256": original["context_sha256"], "content": "用户内容"}), 409, "version_conflict", "workspace.save.file_changed", "文件已被其他操作修改，请比较后保存", "Another operation has changed the file. Compare the contents before saving.")
	assertWorkspaceLocalizedError(t, f, "POST", "/workspace/binding", f.body(map[string]any{"path": filepath.Join(root, "main.go")}), 400, "invalid_input", "workspace.binding.directory_required", "工作区必须是目录", "The workspace must be a directory.")
	assertWorkspaceLocalizedError(t, f, "POST", "/workspace/binding", f.body(map[string]any{"path": t.TempDir()}), 400, "invalid_input", "workspace.binding.outside_allowed_root", "工作区必须位于服务端允许根目录内", "The workspace must be inside the root directory allowed by the server.")
	assertWorkspaceLocalizedError(t, f, "GET", "/workspace/file?project_id="+f.project+"&path=.env", nil, 403, "forbidden", "common.forbidden", "当前身份没有此操作权限", "Your current identity does not have permission for this operation.")
	assertWorkspaceLocalizedError(t, f, "GET", "/workspace/file?project_id="+f.project+"&path=../outside", nil, 404, "not_found", "common.not_found", "记录不存在或不可访问", "The record does not exist or is not accessible.")
	body := f.body(map[string]any{"path": "main.go", "expected_hash": original["hash"], "expected_context_sha256": original["context_sha256"], "content": "用户原文 original user text"})
	f.call("POST", "/workspace/file", body, 200)
	body["content"] = "changed user content"
	assertWorkspaceLocalizedError(t, f, "POST", "/workspace/file", body, 409, "idempotency_conflict", "workspace.save.idempotency_conflict", "该操作标识已用于其他内容", "This operation ID was already used for different content.")
	actual, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil || string(actual) != "用户原文 original user text" {
		t.Fatalf("locale requests changed user file content: %q, %v", actual, err)
	}
	t.Setenv("SUMA_WORKSPACE_ROOT", "")
	assertWorkspaceLocalizedError(t, f, "POST", "/workspace/binding", f.body(map[string]any{"path": root}), 400, "invalid_input", "workspace.binding.server_root_missing", "服务端尚未配置 SUMA_WORKSPACE_ROOT", "SUMA_WORKSPACE_ROOT is not configured on the server.")
}

func TestWorkspaceLocalizationConcurrentRequestsDoNotShareLanguage(t *testing.T) {
	f, root := workspaceFixture(t)
	original := f.call("GET", "/workspace/file?project_id="+f.project+"&path=main.go", nil, 200)
	body := f.body(map[string]any{"path": "main.go", "expected_hash": original["hash"], "expected_context_sha256": original["context_sha256"], "content": "\x00"})
	var wg sync.WaitGroup
	errors := make(chan error, 24)
	for i := 0; i < 24; i++ {
		locale, want := "en-US", "Only text content of at most 1 MiB is supported."
		if i%2 == 0 {
			locale, want = "zh-CN", "仅支持 1 MiB 以内的文本内容"
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, out, err := workspaceLocalizedCall(f, "POST", "/workspace/file", locale, body)
			if err != nil {
				errors <- err
				return
			}
			detail, ok := out["error"].(map[string]any)
			if status != 400 || !ok || detail["code"] != "invalid_input" || detail["message_key"] != "workspace.save.content_limit" || detail["message"] != want || detail["locale"] != locale || detail["localized"] != true {
				errors <- fmt.Errorf("request language mixed: locale=%s status=%d out=%v", locale, status, out)
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	actual, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil || string(actual) != "original" {
		t.Fatalf("invalid concurrent requests changed file: %q, %v", actual, err)
	}
	var operations int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_workspace_file_operations`).Scan(&operations); err != nil || operations != 0 {
		t.Fatalf("invalid requests created save operations: %d, %v", operations, err)
	}
}

func TestWorkspaceLocalizationContextValidationAndUnacceptedProof(t *testing.T) {
	f, root := workspaceFixture(t)
	original := f.call("GET", "/workspace/file?project_id="+f.project+"&path=main.go", nil, 200)
	body := f.body(map[string]any{"path": "main.go", "expected_hash": original["hash"], "content": "用户草稿"})
	assertWorkspaceLocalizedError(t, f, "POST", "/workspace/file", body, 400, "invalid_input", "workspace.save.context_required", "请提供原文件读取结果中的 64 位小写工作区上下文摘要 expected_context_sha256", "Provide expected_context_sha256 as the original file's 64-character lowercase workspace context digest.")
	body["expected_context_sha256"] = original["context_sha256"]
	body["unexpected"] = "user content"
	assertWorkspaceLocalizedError(t, f, "POST", "/workspace/file", body, 400, "invalid_input", "workspace.save.invalid_request", "文件保存请求无效，请保留原文件读取结果并核对保存字段", "The file save request is invalid. Keep the original file snapshot and review the save fields.")
	delete(body, "unexpected")
	// A new binding to the same physical root is still a distinct draft context.
	f.call("POST", "/workspace/binding", f.body(map[string]any{"path": root}), 200)
	assertWorkspaceLocalizedError(t, f, "POST", "/workspace/file", body, 409, "workspace_context_changed", "workspace.save.context_changed", "工作区上下文已变化，请重新加载文件后保存", "The workspace context has changed. Reload the file before saving.")
	for _, locale := range []string{"zh-CN", "en-US"} {
		status, out, err := workspaceLocalizedCall(f, "POST", "/workspace/file", locale, body)
		if err != nil || status != 409 {
			t.Fatalf("context proof response: %d %v %v", status, out, err)
		}
		details := out["error"].(map[string]any)["details"].(map[string]any)
		if len(details) != 5 || details["project_id"] != f.project || details["path"] != body["path"] || details["operation_id"] != body["idempotency_key"] || details["expected_context_sha256"] != original["context_sha256"] || details["not_accepted"] != true {
			t.Fatalf("locale changed exact unaccepted operation proof: %v", details)
		}
	}
	var operations, accepted int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_workspace_file_operations`).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_workspace_accepted_operations`).Scan(&accepted); err != nil || operations != 0 || accepted != 0 {
		t.Fatalf("unaccepted localized errors created effects: operations=%d accepted=%d err=%v", operations, accepted, err)
	}
	actual, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil || string(actual) != "original" {
		t.Fatalf("unaccepted localized errors changed original file: %q %v", actual, err)
	}
}

func TestWorkspaceLocalizationRecoveryKeepsOriginalEvidence(t *testing.T) {
	f, root := workspaceFixture(t)
	original := f.call("GET", "/workspace/file?project_id="+f.project+"&path=main.go", nil, 200)
	body := f.body(map[string]any{"path": "main.go", "expected_hash": original["hash"], "expected_context_sha256": original["context_sha256"], "content": "submitted content"})
	f.call("POST", "/workspace/file", body, 200)
	if _, err := f.s.DB.Exec(`UPDATE suma_workspace_file_operations SET state='prepared' WHERE operation_id=?`, body["idempotency_key"]); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("external 用户原文"), 0600); err != nil {
		t.Fatal(err)
	}
	assertWorkspaceLocalizedError(t, f, "POST", "/workspace/file", body, 409, "workspace_reconciliation_required", "workspace.save.reconciliation_required", "先前保存结果不确定；请将文件恢复为原内容或已提交内容后重试同一请求", "The previous save result is uncertain. Restore the file to its original or submitted content, then retry the same request.")
	_, out, err := workspaceLocalizedCall(f, "POST", "/workspace/file", "en-US", body)
	if err != nil {
		t.Fatal(err)
	}
	details := out["error"].(map[string]any)["details"].(map[string]any)
	if details["operation_id"] != body["idempotency_key"] || details["path"] != "main.go" {
		t.Fatal("translated error changed recovery identity", details)
	}
	var state, response string
	if err := f.s.DB.QueryRow(`SELECT state,response FROM suma_workspace_file_operations WHERE operation_id=?`, body["idempotency_key"]).Scan(&state, &response); err != nil || state != "prepared" || !strings.Contains(response, "submitted content") {
		t.Fatalf("translated error altered durable save evidence: %s %s %v", state, response, err)
	}
	actual, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil || string(actual) != "external 用户原文" {
		t.Fatalf("recovery overwrote unknown external state: %q, %v", actual, err)
	}
}

func TestSearchLocalizationQueryValidationAndOriginalKeywordCoverage(t *testing.T) {
	f := newFixture(t)
	t.Setenv("SUMA_KNOWLEDGE_URL", "")
	for _, query := range []string{" ", strings.Repeat("a", 501)} {
		assertWorkspaceLocalizedError(t, f, "POST", "/search", f.body(map[string]any{"query": query}), 400, "invalid_input", "search.query_invalid", "请输入 1–500 字节的查询", "Enter a query of 1 to 500 bytes.")
	}
	f.call("POST", "/documents", f.body(map[string]any{"title": "original needle 用户标题", "description": "用户原始正文"}), 201)
	for _, locale := range []string{"en-US", "zh-CN"} {
		status, out, err := workspaceLocalizedCall(f, "POST", "/search", locale, f.body(map[string]any{"query": "needle"}))
		if err != nil || status != 200 || out["mode"] != "keyword" || out["coverage"] != "当前授权项目的全部需求、任务、问题和文档；返回最多 50 条" || out["total"] != float64(1) {
			t.Fatalf("search scope or original coverage changed: %d %v %v", status, out, err)
		}
		hit := out["items"].([]any)[0].(map[string]any)
		if hit["title"] != "original needle 用户标题" || hit["snippet"] != "用户原始正文" || hit["locator"].(map[string]any)["project_id"] != f.project {
			t.Fatal("search translated user content or scope", hit)
		}
	}
	t.Setenv("SUMA_KNOWLEDGE_URL", "http://unused.invalid")
	assertWorkspaceLocalizedError(t, f, "POST", "/search", f.body(map[string]any{"query": strings.Repeat("a", 501)}), 400, "invalid_input", "search.query_length", "查询长度超出限制", "The query length exceeds the limit.")
}

func TestWorkspaceLocalizationPublicCatalogAllowsDisplaySwitchWithoutNewRequest(t *testing.T) {
	f := newFixture(t)
	f.cookie = nil
	out := f.call("GET", "/localization/catalog", nil, 200)
	messages := map[string]map[string]any{}
	for _, value := range out["messages"].([]any) {
		message := value.(map[string]any)
		messages[message["key"].(string)] = message
	}
	for _, example := range []struct{ key, source, english string }{
		{"workspace.project_not_configured", "请先配置当前项目的工作区", "Configure a workspace for the current project first."},
		{"workspace.save.file_changed", "文件已被其他操作修改，请比较后保存", "Another operation has changed the file. Compare the contents before saving."},
		{"workspace.save.context_required", "请提供原文件读取结果中的 64 位小写工作区上下文摘要 expected_context_sha256", "Provide expected_context_sha256 as the original file's 64-character lowercase workspace context digest."},
		{"workspace.save.context_changed", "工作区上下文已变化，请重新加载文件后保存", "The workspace context has changed. Reload the file before saving."},
		{"search.keyword.coverage", "当前授权项目的全部需求、任务、问题和文档；返回最多 50 条", "All requirements, tasks, issues, and documents in the currently authorized project; at most 50 results are returned."},
	} {
		message, ok := messages[example.key]
		if !ok {
			t.Fatalf("public display catalog omits %s", example.key)
		}
		texts := message["texts"].(map[string]any)
		if texts["zh-CN"] != example.source || texts["en-US"] != example.english || len(message["parameters"].([]any)) != 0 {
			t.Fatalf("exact original reason cannot be redisplayed in both languages: %v", message)
		}
	}
}
