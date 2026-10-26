# 模型辅助知识提炼：限定本地验证

阶段：T-P4-026；产品版本：1.0.0；日期：2026-10-26。
状态：核心、协议、一次本地端到端旅程、代表性中英 GUI 及主预览前向升级已限定本地验证。完整 P0–P7 继续实施。

## 验证范围

实现固定 1–8 项当前来源、专用只读提炼 profile、单次无工具模型执行、严格提案、原件与修订分离、明确采用为 `model_assisted` 草稿及不可变溯源。正式发布继续单独批准。
复用真实权限、Go DAG、Runner／Pool 回写、用量账本、额度与原知识修订链。未把配置、模型文字、夹具结果或提案 ready 称为厂商／生产验收。
设计见 [PRD](../product/knowledge-intelligence.md)、[架构与 SOP](../architecture/knowledge-intelligence.md)、[合同](../../contracts/knowledge-intelligence.md)；机器事实见 [精简摘要](knowledge-intelligence-summary.json)。

## 必要检查与收据

以下为独立切面，父组、子例与受影响复验不累加为总用例数。Go 使用缓存 Go1.26.10 和 `run-go019.ps1 -Quiet`；无全套重跑或新依赖。

| 切面 | 已验证事实 | 忽略目录收据 |
|---|---|---|
| 后端六组 | 来源／单 run／API scope／严格 Schema；真实完成与重放；修订、开发者草稿采用与发布批准分离；漂移、未知、原件上限、撤权和回滚 | `mcp019-deps/20261026T105207072Z.*` 初轮及 `105427981Z.*` 四失败组接线后通过 |
| 跨组织 Pool | 第二组织实际浏览器凭据、独立 profile、真实领取／心跳／结果；源漂移保留结果，费用归实际项目组织，重放仅一次 | `mcp019-deps/20261026T105652702Z.*`，1 顶层通过 |
| HTTP 合同 | 严格预算、可省略默认值、固定 scope、提案到独立草稿流程 | `mcp019-deps/20261026T110508172Z.*`；默认值调整后 `112324907Z.*` 通过 |
| Runtime | 显式 capability／probe、互斥 profile、单次无工具、完整 final／usage、额外请求和工具拒绝；相关 Go 及 TS 类型检查 | `knowledge-intelligence-stage-026/runtime/verification.local.json`；最终 Node 9 项通过 |
| Core 自动核对 | 私有及 Pool 同 live attempt 的正式完成、当前权限、保留双原件、回执幂等、槽位／租约／无副作用闸门与事务回滚 | `mcp019-deps/20261026T112649244Z.*`，4 顶层／31 pass 事件；`core/verification.local.json` |
| 核对来源显示 | 正式 Runner 完成仍为 runner_result；provider observation 保持原分类；畸形记录报错 | `mcp019-deps/20261026T113135896Z.*`，2 顶层／4 pass 事件；`core/provenance.local.json` |
| HTTP 晚到结果 | empty／partial unknown → final ready；坏 JSON 的正式完成保留原件／真实 usage 且分析 failed；Stopped／过期仍 409 | `mcp019-deps/20261026T112550497Z.*`，1 顶层／5 子例通过 |
| 自动核对新权限 | unknown 后真实 logout；Runner 仍有效但 creator 已撤销，恢复 409，不新增已知事实／原件／草稿，用量仍未知 | `mcp019-deps/20261026T113344195Z.*`，仅新增 creator 子例通过 |

所有表中 `mcp019-deps` 路径均位于仓库 `.cache/`。Core 相关旧 observation 回归和 DAG vet 通过，精确命令另见其 verification 收据，不重复汇总。

## 一次真实本地协议旅程

`.cache/e2e/knowledge-intelligence-stage-026/actual/verification.local.json` 记录 29/29 检查通过。
实际链路为 Control／SQLite → Runner → 原生 bundle → 回环 HTTP 模型边界 → 正式回写及用量结算；模型是明确命名的确定性协议夹具，不是厂商模型。
请求只有一次 POST，未携带 tools，tool_choice=none；23 input + 17 output = 40 tokens。
按本地人工配置的费率估算 57 CNY 微元，组织和项目余额各新增 40 tokens／57 微元，剩余预留均为零；这不是厂商账单。

- Analysis：`11246f2e8d037e34745ede494998e05d`；Run：`knowledge-11246f2e8d037e34745ede494998e05d`。
- 固定来源 SHA：`a5b4ab5124a6c40a035d9a8bc01e39aa5aca6593e5019e9e7e58398f0f1f08de`。
- 模型原件 SHA：`ebb29eaa3d104eb1e9678e53a512ea587fa3b02f0e730dd0c3fabd52995773ab`。
- 初始结果 ready，不自动创建知识或发布；Runner 正常退出 0，模型夹具正常关闭，vendor calls=0。

该一次端到端旅程与 Runtime 独立协议单元用例分别记录，不把各切面的回环调用次数相加或冒充真实厂商调用。
正常完成的证据复用；后加的未知恢复与新权限闸门另有定向验证，没有为修复再次调用该模型旅程。

## 实际 GUI 与最终配对

Root 以隔离测试账号明确修订、采用并发布；此为本地测试操作，不是业务负责人或生产验收。
只读核对 `ui/data/suma.db` 与 `ui-final/data/suma.db`，两者均保留同一 Analysis 的 adopted v4，原件与来源 SHA 不变。
生成知识 `8059e3356073730eb4b698f253ba95a8` 为 published v2／published_revision=2、method=model_assisted。
发布 SHA 为 `c34a759e7235b93219cacf23fa9e057c97fe0fe35ab655efba691a3cfd530927`，正文 SHA 为 `ce4853c274057d1db58993332deba04281f3642e351db35afd16b9abe933015f`。
两份隔离库均为 164 表；原 UI holder 收据为 graceful exit0、integrity=ok、原来源库保持。此不替代最终主预览升级的逐表／资产核对。

最终 `browser/verification.local.json` 的 SHA 为 `32d9a82baf4039e5483c22619f37de2e3c0b66404269ae0673b009a507f9eb7a`。
其记录最终中英简单标题／说明／正文／理由字段、固定来源选框、默认折叠 JSON、独立原件、采用后发布入口与溯源深链，原用户标签保持。
最终 EN／ZH 截图及其 SHA 留在该 browser 收据；最后两轮隔离 UI session 50126／9805 正常退出 0，无强制结束，没有重跑模型。

主预览 `http://pool.localhost:4324/` 当前为 normal control 199624／holder 250388，exec 2716 保持运行。
最终源输入 337 项，source SHA `b48d4561940a69e0df31cac28ddf3ad40e63455059e1a5772a6c5508e9e82d67`。
control SHA `12f28126714601fdc4bf211173035ad6b67d24d8e770cc0f2cd8dccebe61b79d`；web-release2 共 121 文件，入口 SHA `b59a3bdcccae7d67a5e38c285faf1595d528cbe2c831df70f364480d8eae9c0b`。
`primary-upgrade/final-verification.local.json` SHA 为 `f2cdf2987a8e306101fa3d67410d80075fbaa63d1d1e3278939e8a66a7619016`：原 161 表 schema／全部行完全保持，只加 3 张空表和 1 索引至 164，integrity=ok。
18 authority／0 Grant 保持，原运行 1 failed／2 succeeded 与登录未重发；loopback／浏览器别名入口字节及引用资产匹配，旧 session 43321 正常退出 0。
上述采用／发布写入仅在隔离验收库；主预览三张新表仍为空，未把测试业务迁入原库。
配对构建和过程收据留 `runtime-final`、`web-release2`、`primary-upgrade`，原始证据、数据库、截图和二进制不提交 Git。

## 失败、修复与保留事实

初轮后端真实 claim 返回空 assignment；保留 `105207072Z` 的 exit1，完成 managed 类型／admission／结果接线后只复验失败组通过。
Runtime 初始缺少 wire capability 的 RED 保留在 `104751693Z`，后续协议／探测修正与最终结果见 runtime 收据。
首次 unknown 就固定原件的语义问题已改为“未知观察独立保存，首个已知结果固定分析原件”。
新增 HTTP 恢复最初被原 Core 的 ErrStale 拒绝，`111637617Z` exit1 保留；没有 SQL 伪切回 running。
随后增加默认关闭、当前授权且同 live readonly attempt 的核对接口；Core 初始 RED `112439181Z` 和后续 GREEN 均保留。
合法完成的业务 JSON 即使无效，也保存执行／费用事实并将分析标 failed；自动核对不把业务 Schema 当作执行授权。
旧未知→已知停止场景已按收到的已知原件冻结摘要，未知原 fact 仍原样保存；相关原异常组通过记录在 `111637617Z` 内。

## 尚待完成

未做真实厂商账号／模型效果／厂商计费验收、全 CLI 命令与本地化闭环、完整设备／主题矩阵、跨数据库及分布式规模验证。
自动质量评估、长期自主提炼授权、向量检索、技能执行包、外部平台、移动端及完整 P0–P7 继续实施。
