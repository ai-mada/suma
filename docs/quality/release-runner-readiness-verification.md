# 专用发布 Runner 能力观测验收

速码 1.0.0，`T-RELEASE-RUNNER-READINESS-258`。关联需求为分布式节点管理、发布前核对、可解释状态和中英文工作台。 [发布合同](../../contracts/release-control.md#发布节点能力观测) · [工作台设计](../design/release-workspace.md)

## 设计与操作

发布预览和计划详情根据当前项目、发布凭据和准确提供方标识展示能力观测。近期没有匹配报告时，页面分别显示未登记、尚未观测、过期或上报缺少能力，并注明本次核对时间。已认证的专用 Runner 可在普通领取时上报与本机发布协议相符的提供方标识。原领取回执重放不会重新计时。

预览、审批和排队仍沿原授权链执行。能力上报与发布环境连通、执行槽位和部署健康分别判断。原共享池只承接其已授权普通运行。

交互参考 Multica 对运行时可达性与任务负载分别建模的方式。 [Multica 固定源码](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/packages/core/agents/derive-presence.ts#L1-L10)

## 阶段验证

| 范围 | 验证结果 |
| --- | --- |
| Windows、SQLite 和认证 HTTP | `TestReleaseRunnerReadinessUsesCurrentDedicatedClaimAndDoesNotGateDeployment` 验证尚未观测、协议和提供方不匹配、已匹配、过期、重复回执、轮换以及其他身份和项目的隔离。缺少上报时发布可以排队，随后由符合条件的专用 Runner 领取。 |
| 中英文工作台 | 21 项定向前端测试通过。Chromium 使用 API 测试数据在 390 像素中文和 1280 像素英文下检查回滚预览、计划详情、执行确认、键盘展开和横向溢出。验证期间未发送部署写入。 |
| 接口 | Go、OpenAPI 与 TypeScript 合同一致。应用发布组的只读预览按提供方汇总能力信息，独立发布和回滚复用同一字段。 |

浏览器结果和截图保存在本地忽略目录 `.cache/q258/release-readiness-browser/`。此阶段验证截至认证领取和本地队列，生产提供方连通、跨机部署和 OceanBase 未做实际验证。
