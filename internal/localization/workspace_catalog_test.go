package localization

import "testing"

func TestWorkspaceAndSearchCatalogExactDiagnostics(t *testing.T) {
	for _, example := range []struct{ key, source, english string }{
		{"workspace.project_not_configured", "请先配置当前项目的工作区", "Configure a workspace for the current project first."},
		{"workspace.directory_unavailable", "工作区目录不可用", "The workspace directory is unavailable."},
		{"workspace.read.text_limit", "编辑器仅打开 1 MiB 以内的文本文件", "The editor can only open text files of at most 1 MiB."},
		{"workspace.save.hash_conflict", "文件保存冲突，请重新加载", "The file could not be saved because it changed. Reload it."},
		{"workspace.save.reconciliation_required", "先前保存结果不确定；请将文件恢复为原内容或已提交内容后重试同一请求", "The previous save result is uncertain. Restore the file to its original or submitted content, then retry the same request."},
		{"workspace.save.operation_invalid", "请提供 8–128 字符的 idempotency_key", "Provide an idempotency_key of 8 to 128 characters."},
		{"workspace.save.content_limit", "仅支持 1 MiB 以内的文本内容", "Only text content of at most 1 MiB is supported."},
		{"workspace.save.idempotency_conflict", "该操作标识已用于其他内容", "This operation ID was already used for different content."},
		{"workspace.save.file_changed", "文件已被其他操作修改，请比较后保存", "Another operation has changed the file. Compare the contents before saving."},
		{"workspace.binding.server_root_missing", "服务端尚未配置 SUMA_WORKSPACE_ROOT", "SUMA_WORKSPACE_ROOT is not configured on the server."},
		{"workspace.binding.outside_allowed_root", "工作区必须位于服务端允许根目录内", "The workspace must be inside the root directory allowed by the server."},
		{"workspace.binding.directory_required", "工作区必须是目录", "The workspace must be a directory."},
		{"workspace.method_invalid", "不支持此请求方法", "This request method is not supported."},
		{"search.query_invalid", "请输入 1–500 字节的查询", "Enter a query of 1 to 500 bytes."},
		{"search.query_length", "查询长度超出限制", "The query length exceeds the limit."},
		{"search.keyword.coverage", "当前授权项目的全部需求、任务、问题和文档；返回最多 50 条", "All requirements, tasks, issues, and documents in the currently authorized project; at most 50 results are returned."},
	} {
		t.Run(example.key, func(t *testing.T) {
			key, ok := LookupSource(example.source)
			if !ok || key != example.key {
				t.Fatalf("exact source has no stable message key: got %q, want %q", key, example.key)
			}
			for locale, want := range map[Locale]string{ZhCN: example.source, EnUS: example.english} {
				rendered, err := WithSource(locale, key, nil, example.source)
				if err != nil || !rendered.Localized || rendered.Key != key || rendered.Locale != locale || rendered.Text != want || rendered.Fallback != "" {
					t.Fatalf("%s exact source rendering: %+v, %v", locale, rendered, err)
				}
			}
			for _, changed := range []string{"用户内容：" + example.source, example.source + " D:/user/project"} {
				if _, matched := LookupSource(changed); matched {
					t.Fatal("a partial or user-content diagnostic matched the static catalog")
				}
			}
		})
	}
}

func TestWorkspaceCatalogKeepsUnknownIODiagnostic(t *testing.T) {
	const source = "open D:/user/工作区/private.txt: Access is denied."
	if _, matched := LookupSource(source); matched {
		t.Fatal("unknown operating-system diagnostic matched a product message")
	}
	rendered, err := WithSource(EnUS, "legacy.internal_error", nil, source)
	if err != nil || rendered.Text != source || rendered.Localized || rendered.Fallback != "source_untranslated" {
		t.Fatalf("unknown original diagnostic was replaced: %+v, %v", rendered, err)
	}
}
