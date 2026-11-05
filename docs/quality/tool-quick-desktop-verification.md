# 添加工具桌面全流程验证

T-TOOL-QUICK-DESKTOP-251 · 速码 1.0.0 · 2026-11-06。

| 检查 | 实际结果 |
| --- | --- |
| Windows、Node 22.16.0，在 Web 目录执行 `node node_modules/vitest/vitest.mjs run LocalToolsScan ToolQuickConnect QuickConnectIntegration QoderQuickConnectCoordination ManagedToolPreparationPanel ToolProfileReplacementIntegration ToolWorkspace-experience` | 13 文件、58 个用例通过。新回归覆盖准确暂停回执后仅一次进入启用审阅、错配回执不进入、原来只保存路径继续使用 |
| 窄屏启用审阅调整后，定向重跑 `node node_modules/vitest/vitest.mjs run ToolWorkspace-experience ToolProfileReplacementIntegration` | 2 文件、5 个用例通过，复用其余未改变行为的结果 |
| `node apps/web/node_modules/typescript/bin/tsc --project apps/web/tsconfig.json`、`node apps/web/scripts/check-i18n.mjs` | 全量 Web 类型检查及 10,610 对双语词条检查通过 |
| Go 1.26.10，以 `go build -o <隔离输出目录> ./cmd/suma-control ./cmd/suma-runner` 编译 `b345fcf` 的已提交 Go 源码；Vite `build --outDir <隔离输出目录> --emptyOutDir` 编译本次 Web 改动 | 控制服务、Runner 和最终 Web 构建均通过。Vite 输出原有较大分块提示；活动桌面资源目录未被用作构建输出 |

## 实际 Windows 桌面

Playwright 驱动 Electron 35.7.5 的实际速码主进程、preload 和界面。使用隔离 SQLite、测试身份与 Git 仓库，进程局部 PATH 指向已安装的 `codex-cli 0.166.0-alpha.6`。Runner 以 `--report-tools --allow-exec` 运行一次，实际探测版本与帮助并上报同一可执行文件摘要对应的只读、工作树可写配置。桌面通过真实 IPC 自动识别并选中 Codex 命令行。

在中文 1366×960 窗口选择只读配置并改名。「保存并核对启用」发出一次登记请求，数据库读回准确的暂停 v1，界面直接聚焦暂停回执并显示启用确认。英文 390×844 审阅的主要操作位于首屏，详细诊断默认折叠，无横向溢出。勾选版本确认后发出一次独立启用请求，原配置读回为已启用 v2，库存当时可用。两个请求分别持有独立操作标识，项目零任务、零运行，控制服务和窗口均正常关闭。

首次受限进程运行出现 Electron GPU 子进程退出，改用同机隔离的桌面进程后启动成功。一次测试仓库因未忽略工作树缓存被实际库存校验拒绝，补齐仓库规则后完成两种配置上报。首次跑通时发现窄屏确认按钮在首屏之外，压缩审阅内容后按同一真实链路复验通过。

截图、经筛选的实际回执和摘要存放于忽略目录 `dist/quality/tool-quick-desktop-251/`。本次覆盖 Windows 上一个 Codex CLI 接入路径，厂商登录、模型调用、任务运行和跨平台安装按各自合同验收。
