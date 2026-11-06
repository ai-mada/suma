# 发布协议匹配与应用批量预览

速码 1.0.0，`T-RELEASE-PROTOCOL-ADMISSION-259`。关联需求为分布式调度、发布审阅、异常恢复和中英文工作台。 [发布合同](../../contracts/release-control.md#发布领取的协议匹配) · [提供方配置](../integrations/release-provider.md) · [交互设计](../design/release-workspace.md)

## 设计决定

调度器在建立新尝试前，按租户、项目、运行和节点读取控制面保存的受管部署绑定。同一份已认证的专用 Runner 领取请求须同时上报该提供方的通用工作流能力和准确发布协议能力。只有同名通用能力或只有另一提供方的正式协议时，节点继续排队，直到可匹配的专用 Runner 领取。审批及执行请求沿原流程提交，已有领取回执按原请求与授权恢复。

应用发布集在每个应用的批量准备预览中显示对应节点观测。状态直接可见，建议和时间可展开查看。观测反映经过认证的能力报告；发布环境连通、剩余容量和实际健康由各自的证据判断。

参考 Multica 在守护进程内把已确认无法执行的运行时移出派发的做法。速码沿用自己的受管部署、租约与审计结构，并依据单次领取的正式协议能力决定资格。 [Multica 固定参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/daemon/agents_refresh.go#L460-L482)

## 阶段验证

- Windows、Go 1.26.10、隔离 SQLite 和回环 HTTP。8 项定向集成通过，包括 `TestReleaseRunnerReadinessUsesCurrentDedicatedClaimAndDoesNotGateDeployment`、`TestReleaseDeploymentActualProviderHealthAndNewGenerationRollback` 和 `TestApplicationReleaseRolloutActualOrderedPauseAndUnknownRecovery`。缺少正式协议或提供方不匹配时，受管节点没有新增尝试、fence 或租约。匹配后实际本地静态部署、健康检查、历史回滚和有序发布恢复通过。原回执重放和使用同名通用能力的普通工作流通过。
- 19 项定向前端测试和 TypeScript 检查通过。Chromium 使用 API 测试数据验证 390 像素中文和 1280 像素英文的应用发布集预览。核对成员状态独立、键盘展开、无横向溢出和确认后才可准备。浏览器只请求预览。
- 执行日志与浏览器结果分别保存在本地忽略目录 `.cache/q259/release-claim-integration.txt` 和 `.cache/q259/app-release-set-ui/`。生产提供方、跨机器网络和 OceanBase 本阶段未实测。
