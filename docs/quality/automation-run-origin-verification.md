# 自动化来源追溯验收

状态：T-AUTOMATION-RUN-ORIGIN-276 已通过本机定向验证。版本：1.0.0。范围：运行详情、自动化历史、项目读取权限。

在运行详情展开「自动化来源」，核对触发方式、UTC 时间、原规则名称与修订，按需展开触发 ID。链接打开当前项目的原自动化规则。原运行使用触发时的不可变修订，规则后来改名不改变历史。界面区分确认无来源、无权读取和临时读取失败，切换账号、项目或运行会停止旧读取。

控制面按项目及运行精确读取持久触发绑定，同时核对历史规则修订与该运行固定的流程。只有唯一、完整的已开始触发记录可归因。普通运行返回明确空值，其他项目的运行不能通过当前项目读取。增加反查索引，无新业务表或写接口。 [合同](../../contracts/automation-events.md)

参考 Multica 由原自动化运行 ID 定位其规则的做法。速码沿现有 DAG 和触发事实实现。 [Multica 固定参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/service/task.go#L1077-L1084)

## 定向证据

- 真实 HTTP 与 SQLite 走通人工触发、调度绑定、规则改名后的原修订回读、普通运行空值、越权与撤权。自动化的创建、原操作修订、签名事件和合同定向用例同轮通过。
- 4 项 Web 组件检查通过，覆盖按需读取、错误重试、上下文切换、拒绝旧数据及团队来源样式复用。受影响的 4 个 TypeScript 文件无诊断。
- 实际 Chrome 使用隔离组件与合成响应，验证中文 390px、英文 1280px、键盘展开、触发 ID 与原规则链接。未写入业务数据。联机浏览器全流程、OceanBase、PostgreSQL、物理多节点和生产规模仍待验收。

```powershell
go test -p 2 ./internal/platform -run '^(TestAutomationRunOrigin|TestAutomationHTTPCreateAndEnable|TestAutomationReceiptImmutableRevisionAndFixedWorkflow|TestAutomationIngressQueuesFixedWorkflowAndStoresOnlyRestrictedFacts)' -count=1
go test -p 2 ./cmd/suma-contracts -run '^TestAutomationAndStreamContracts' -count=1
# 在 apps/web 下运行
pnpm exec vitest run src/RunAutomationOrigin.test.tsx src/RunTeamOrigin.test.tsx
```
