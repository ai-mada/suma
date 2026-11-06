# 添加工具异常路径验证

T-TOOL-QUICK-RECOVERY-266 · 速码 1.0.0 · 2026-11-06。

| 检查 | 实际结果 |
| --- | --- |
| Windows、Node 22.16.0，在 Web 目录运行 `node node_modules/vitest/vitest.mjs run LocalToolsScan ToolQuickConnect QuickConnectIntegration QoderQuickConnectCoordination ManagedToolPreparationPanel ToolProfileReplacementIntegration ToolWorkspace-experience ToolWorkspaceManagement` | 14 文件、72 项不同用例最终通过。节点重读按钮移动后修正测试定位并定向复跑 Codex 的 2 项 |
| `node apps/web/node_modules/typescript/bin/tsc --project apps/web/tsconfig.json`、`node apps/web/scripts/check-i18n.mjs` | Web 全量类型检查及 10,794 对词条通过 |
| Go 1.26.10 以 `go build -o <隔离输出目录> ./cmd/suma-control ./cmd/suma-runner` 编译 `fbf8d42`，Vite 以 `build --outDir <隔离输出目录> --emptyOutDir` 编译本次 Web 改动 | 控制面、Runner、实际桌面使用的 Web 构建均通过。构建输出与活动预览隔离 |

## 真实 Windows 桌面

Playwright 驱动 Electron 35.7.5 的实际速码主进程、preload、独立 SQLite 和测试项目。9 个入口经过真实 IPC 检测。安装的 Codex CLI 启动方式已确认，Codex App Server 需要配置。实际找到 Cursor 编辑器，Cursor Agent 未就绪。Claude CLI 与 SDK、CodeBuddy、OpenCode、Qoder、dsh 在本次隔离检测中无可用安装。逐一在真实弹窗选择六个其他常用入口，验证本机提示和配置入口。

在进程专属目录加入受控的 CodeBuddy 命令启动器，通过真实 IPC 重检得到不支持的启动入口，原工具选择保留。使用真正的 Codex CLI 运行一次 Runner 库存上报。在实际界面注入明确的 HTTP 422 拒绝，确认名称和固定配置保留。重试在真实控制服务提交后刻意断开响应，界面锁住新保存。恢复时请求内容与操作标识完全相同，读回一条暂停配置。

桌面与控制服务均正常关闭，零任务、零运行和零渲染异常。截图、经过筛选的回执及摘要保存于忽略目录 `dist/quality/tool-quick-recovery-266/`。其他厂商的完整接入须具备实际支持的安装和授权后验收。
