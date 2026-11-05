# 任务运行失败通知验证

T-MULTICA-RUN-FAILURE-INBOX。Windows、Go 1.26.10、Vite 6.1.0、Chrome 和隔离 SQLite。正式构建放在任务独立目录。

- `go test ./internal/platform -run '^Test(TaskRunFailureInbox|FailureProjection|NotificationInbox|NotificationOrganizationInbox|ReleaseManagedFailureProjection)' -count=1` 通过 22 项。覆盖真实 DAG 失败、问题与通知事务回滚、负责人／关注者去重、撤权、历史运行定位、损坏来源隐藏、原回执重放和发布失败回归。第一次新增夹具的预期版本类型不符合 HTTP 解码表示，修正夹具后复验。
- `go test ./cmd/suma-contracts -run '^TestNotification.*InboxContract' -count=1` 和生成校验通过。结构比对验证新增项是现有通知对象的失败类型、可选原运行坐标及对应说明。并行的 WorkBuddy 输入以已提交版本隔离生成，并在合入时基于当前 dev 再核对。
- 通知组件与模型 40 项、导航 7 项定向检查通过。隔离生成 DTO 下 Web TypeScript、10370 对双语词条及生产硬编码检查通过。Vite 独立生产构建通过，保留既有包体积警告。
- 最终 Chrome 10 项通过，零运行错误。实际浏览器账号在另一项目打开真实原运行；正式审批拒绝触发持久失败事件，问题和失败通知只生成一份，当前任务状态及个人已读分开呈现。核查 1366px 浅色和 390px 深色键盘焦点。
- 最终验证在本次隔离数据库中有意损坏该运行的任务绑定，再请求两类收件箱。服务端隐藏任务标题和运行坐标，通用来源字段保持摘要，刷新页面也隐藏原入口。初版真实验证的 8 项结果保留，权限呈现的修订以最终报告为准。
- 最终报告 `.cache/run-failure-inbox/browser-vnWEG8/result.json`，SHA256 `0252b2e1a90b4e0035d6b43f18f6c4550be1ff36c86cc622958a71ae2fa37161`。保留报告与原始截图。独立构建及浏览器临时数据经摘要盘点后清理，共 312 个文件，见 `.cache/run-failure-inbox/scratch-cleanup.json`。
- 本阶段验证平台内的原执行和问题投影。真实厂商失败、通知外送及其他数据库和多机规模仍按原计划验收。
