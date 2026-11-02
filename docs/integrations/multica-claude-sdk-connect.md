# Multica参考：本机SDK接入收敛

T-CLAUDE-SDK-CONNECT-201；suma 1.0.0；参考本地D:/codes/github/multica提交`5a2cc891ac338eed1919aa11a1eef5957cd82cec`。本轮参考仓库只读，工作区原本干净，没有拉取、覆盖或改写本地文件。

采用的是发现→有界版本探测→注册运行环境→持续报告可用状态的产品与架构模式。参考位置包括daemon的agents_probe.go/probeAgentCLIs、agents_refresh.go/agentDiscoveryLoop与收敛逻辑，以及daemon.go的注册入口；suma实现复用自身现有扫描、托管Runner、官方SDK宿主、加密Journal和正式配置登记。

| 模式 | suma采用方式 | 限制 |
| --- | --- | --- |
| 工具发现 | 原单项／批量扫描追加SDK用途，实际CLI版本与SHA | 不等于SDK资源、模型认证或运行成功 |
| 运行环境收敛 | 本次连接的新鲜完整库存和准确摘要匹配 | 不以旧状态或名字推导当前环境引用 |
| 本机原意图恢复 | 原HMAC身份、Journal和停止／未知处置 | 不复制自动批准或隐式权限扩大行为 |
| 统一工作台 | 原Quick和独立明确启用 | 不新建第二配置权威或平行派发器 |

基线[LICENSE](https://github.com/multica-ai/multica/blob/5a2cc891ac338eed1919aa11a1eef5957cd82cec/LICENSE)及NOTICE包含Hosted／embedded commercial附加条件，不能将整个仓库描述为无条件Apache复用。本阶段不复制受该附加条件覆盖的源码；参考架构与公开能力，使用suma既有模块独立接线。Claude SDK保留其真实依赖条款及完整资产告知，不重新标为MIT／Apache。

当前公开[Multica仓库说明](https://github.com/multica-ai/multica)强调将现有Agent CLI和自有运行环境纳入团队任务协作；suma的配置、模型、执行、独立测试和验收仍分别取证。实际实现与验收以[本功能合同](../../contracts/claude-sdk-quick-connect.md)和[限定验证报告](../quality/claude-sdk-quick-connect-verification.md)为准，不以参考项目的能力宣称本平台已全部完成。
