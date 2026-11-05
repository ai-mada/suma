# 跨项目排期重叠验证

T-PORTFOLIO-SCHEDULE-OVERLAP。Windows、Go 1.26.10、Vite 6.1.0、Chrome 156.0.8078.80 与隔离 SQLite。

- `go test -overlay <阶段隔离配置> ./internal/platform -run '^TestProjectScheduleOverlap' -count=1 -v` 通过 4 项。覆盖真实账号跨项目人力绑定、同日及跨周、半区间和已验收任务排除、隐藏项目、观察者及负责人撤权、2000／2001 条上限和封闭 HTTP 合同。
- `go test -overlay <阶段隔离配置> ./cmd/suma-contracts -run '^Test(ProjectPortfolio|ProjectScheduleOverlap)Contract$' -count=1` 通过 2 项。`suma-contracts -check` 在并行仓库工作台提交合入后重新核对 1064 个模型、479 条路径。相关 Go 和合同共 6 项复验通过。生成结构只新增排期重叠查询和 3 个类型。
- `node node_modules/vitest/vitest.mjs run src/ProjectScheduleOverlap.test.tsx` 通过 5 项。组合与管理导航 10 项回归通过。10449 对双语词条及生产硬编码 0。直接运行 `tsc --noEmit` 仅报告并行仓库工作台在途文件一处 TS2532；TypeScript Compiler API 将该并行范围的 3 个修改文件恢复为 HEAD 内容、排除 2 个新增文件后，按原工作区配置检查通过，零诊断。并行仓库工作台提交后，整库 `tsc --noEmit` 通过，最终双语目录 10495 对，生产硬编码 0。
- 正式 HTTP 在隔离库建立 3 个项目、2 个账号、5 个项目授权、3 条人员绑定、4 项任务及 4 个计划。Chrome 最终 11 项通过，零运行错误。所有者可见周一和周三；权限受限的成员只见周三。真实撤权后刷新移除原重叠和任务标题。验证键盘入口、下一周、进入准确跨项目任务和浏览器返回原周。目视复核中文明亮 1366px 与英文深色 390px。
- 浏览器报告 `.cache/project-schedule-overlap/browser-evidence-x2Lnyl/result.json`，SHA256 `3f1227d1810ac9303bf7af4ef358c9e856dca71397b1e4f7d6ae92cda51e6340`。控制面按 `71d0197` 加本阶段源构建，隔离并行仓库工作台 Go 改动。Chrome 已关闭，控制进程结束且端口释放。盘点后清理 278 个浏览器临时文件，保留两份报告、截图、验收脚本及 `.cache/project-schedule-overlap/browser-cleanup.json`。生成验证清理 14 个临时文件，收据保存在 `.cache/project-schedule-overlap/contract/scratch-cleanup.json`。

本阶段验证正式计划日期的交集。组织总容量、实际超配、跨数据库与组合规模继续按实施计划验收。
