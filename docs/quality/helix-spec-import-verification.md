# HelixHarness 规范导入阶段验证

阶段 T-HELIX-SPEC-IMPORT，速码 1.0.0。 [产品操作](../product/project-knowledge.md#导入-helixharness-规范) · [数据合同](../../contracts/project-knowledge.md#从-helixharness-specmd-创建草稿)

- Windows 的真实 Chrome 通过 Vite 隔离页面连接实际 Go 控制面和 SQLite。英文浅色 1366px 与中文深色 390px 均验证本地文件预填、正文编辑、保存按钮可见、明确保存及刷新后读取。两个工作区各收到一次 HTTP 201，落库为人工规范草稿，来源为空，发布修订为零。选取和编辑阶段没有写请求。
- 中文窄屏额外验证了误选 `tasks.md` 时阻止保存、重新选择有效文件，以及保存后通过人工勾选复核确认才产生一次发布写入。发布修订包含原草稿审阅后的完整正文。两个布局均无横向溢出和页面脚本错误。
- CLI 定向覆盖本地离线预览、UTF-8 和字节限制、文件错误、当前凭据拒绝、项目范围校验。实际 HTTP／SQLite 验证同一 JSON 两次显式提交返回同一记录，库中仅有一个草稿和一个修订。原技能导入继续通过；Windows 的符号链接夹具因当前进程无创建权限跳过。
- Web 的 15 项定向用例通过，包含原技能解析与两类文件的草稿交互。TypeScript 和中英文词条检查通过。
- 任务知识的原派发定向回归通过，覆盖固定发布快照，以及来源变化后禁止启动及写回执。

```powershell
go test ./internal/cli -run 'Test(KnowledgeHelixSpecImport|KnowledgeImportPreviewAndExplicitDraft|ReadSkillMarkdown)' -count=1 -v
go test -p 2 ./internal/platform -run '^TestTaskKnowledgeDispatch(KeepsPublishedSnapshotWhileDraftChanges|ChangedBasisCannotStartOrWriteReceipt)$' -count=1
go build -p 2 -o .cache/helix-spec-import/control.exe ./cmd/suma-control
node .cache/helix-spec-import/browser.mjs
node scripts/check-docs.mjs
```

本阶段验证覆盖本地规范交接。宿主插件安装与执行、原仓库双向同步、外部验收报告和生产部署仍待逐项验证。

