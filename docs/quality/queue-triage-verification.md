# 待派发队列分诊验证

T-QUEUE-TRIAGE 将共享池当前诊断接入原就绪队列，并提供只针对已加载记录的分诊筛选。 [交互设计](../design/scheduling-workspace.md#队列分诊) · [调度合同](../../contracts/scheduling-kernel.md#项目准入与公开合同)

## 定向检查

- Windows amd64、Go 1.26.10。`go test ./internal/platform -run '^(TestPoolHTTPCrossOrganizationPrivacyAndProjectRunnerChoices|TestPoolHTTPBilateralGrantAndExplicitCurrentScope|TestPoolHTTPRoutePreviewFixedGrantAndOldRun|TestSchedulingReadScopeCannotLeakOtherProject|TestSchedulingZeroPolicyBlocksActualAttemptAndBudgetThenAllows)$' -count=1 -v` 通过 5 项。先复现队列缺少池暂停原因，再核验跨组织真实 HTTP、授权撤回、队列续页与准确节点一致。读取期间无新增尝试或操作回执。原件保存在 `.cache/scheduling-queue-admission/backend/`。
- Node 22.16.0。在 `apps/web` 下运行 `node ./node_modules/vitest/vitest.mjs run src/scheduling-dom.test.tsx src/scheduling-scope.test.tsx src/i18n/workspace-consumers.test.tsx --maxWorkers 2 --minWorkers 2`，通过 35 项。覆盖筛选、诊断范围校验、重试、撤权、浏览器历史和双语。`node ./node_modules/typescript/bin/tsc --noEmit --incremental false` 和 `node scripts/check-i18n.mjs` 通过，核对 10,728 个双语词条。原件保存在 `.cache/scheduling-queue-web/`。

## 实际浏览器

在 `b69e071` 上叠加本阶段源码，用 `go test -overlay … ./internal/platform -run '^TestQueueTriageActual$' -count=1 -timeout=120s -v` 启动隔离 SQLite 控制面。通过原业务接口创建私有运行和共享池运行，随后暂停资源池并撤回共享授权。

- 英文浅色 1366px。真实队列同时显示阻塞和待核对节点。键盘可筛选、展开诊断并打开准确运行与节点。筛选与展开不增加网络读取，浏览器返回恢复所选筛选并重新读取队列。
- 中文深色 390px。验证筛选无结果及切换到待核对状态，筛选控件至少 44px，页面无横向溢出。

两条旅程共 20 次本地 HTTP 读取。操作回执、尝试和运行事件数量增量均为零。浏览器控制台无错误，外部访问为零。请求、截图及清理回执位于 `.cache/queue-triage/`。实测使用 SQLite，生产资源池与物理 Runner 分别验收。12ui 包仓库访问受限，界面沿用速码现有组件与令牌。
