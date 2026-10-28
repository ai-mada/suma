# 统一智能输入与草稿计划验证

状态：T-INTENT-114限定范围通过，完整平台继续实施。日期：2026-10-28；产品：1.0.0。

[精简摘要](intelligent-operations-summary.json) · [领域合同](../../contracts/intelligent-operations.md) · [架构与验收SOP](../architecture/intelligent-operations.md) · [中文指南](../development/intelligent-operations.md)／[English](../en/intelligent-operations.md) · [完整实施计划](../delivery/implementation-plan.md)

## 已交付行为

当前项目统一入口将自然语言目标及0–8个明确来源交给固定模型，形成最多8步可编辑计划。当前动作限定需求草稿、需求设计草稿和项目知识草稿；服务端严格校验类型、依赖、目标及来源，返回只读预览与默认值出处。本次明确确认同一预览后，复用既有领域事务helper，将三类草稿、修订、活动、真实结果、分析CAS和原操作回执在同一数据库事务内提交，任一步失败整体回滚。

模型运行沿唯一Go DAG、现有原生运行时及提供方端口，专用能力为`suma-intelligent-operations-v1`：一次调用、无工具／工作区、固定模型与地址、最多8192输出Token，prompt与提案各最多32000 UTF-8字节。当前浏览器或限定项目API的真实权限先于读取、写入和成功回执；来源／目标变化阻止采用。未知执行、已知无效输出和可审阅提案分别记录，原模型内容、人工修订及采用结果保持独立。

安全上下文仅投影允许的description与需求／任务验收条目；正式来源使用既有canonical SHA，不能把Record.Data、配置或环境明文带入模型。明显私钥、常见Token及URL凭证有有限预检查，该检查不代表完整秘密分类。确认仅覆盖本批草稿，不建立长期自动批准策略，也不发布设计／知识、采用正式任务或启动开发。

## 分组定向检查

共39个不同受影响用例分组通过，重跑和子场景不重复累计；这不是一次完整套件的39项运行。

| 检查组 | 不同用例数 | 覆盖范围 |
| --- | ---: | --- |
| 新Go领域与合同 | 16 | 平台9、Runner3、工具合同1、生成合同2、本地化1；严格输入、来源／目标／当前权限、原子回滚、原回执、未知后已知结果、私有／Pool领取及能力边界 |
| 既有Go回归 | 2 | 知识提炼late-known原件恢复、需求设计不完整草稿诊断与结构 |
| Web模型与组件 | 15 | 模型6、UI9；提案／拓扑、原操作恢复、修订／预览确认、双语及失败／结果导航 |
| 原生运行时 | 4 | 实际CLI模型协议、一次无工具调用、预算／工作区拒绝、专用离线能力探测 |
| 工具导入与设置 | 2 | 既有工具导入用例1、新智能操作设置预设用例1 |

Go使用Windows本机缓存Go1.26.10及离线依赖，TEMP／TMP／GOTMPDIR均限定仓库`.cache/`。主要命令为：

```text
go test ./internal/platform ./internal/runner ./internal/toolcontract -run TestIntelligentOperations -count=1
go test ./internal/platform ./cmd/suma-contracts ./internal/localization -run ^TestIntelligentOperations -count=1
go test ./internal/platform -run "^(TestIntelligentOperationsPoolManagedClaimCurrentProfileAndResult|TestKnowledgeIntelligenceLateKnownOriginalHTTP|TestRequirementDesignDraftDiagnosticsAndStructure)$" -count=1
npm exec -- vitest run src/intent-operations-model.test.ts src/IntentOperationsPanel.test.tsx
```

初次Go组合中Runner与工具合同已通过，平台因managed运行未加入既有领取白名单失败；已补齐私有与Pool原门禁，随后平台／合同／本地化及Pool专项通过，没有另建派发器。精简原始入口为`.cache/mcp019-deps/20261028T092412356Z.*`、`20261028T092922032Z.*`与`20261028T093249048Z.*`；最后一组包含Pool新项和上述两个旧回归。

Web的模型6项与原UI8项分别有定向通过记录，原14项摘要保存在`.cache/e2e/intent-114/ui-targeted-summary.json`，没有最终一次联合14项命令。独立审阅发现failed提案缺失关闭入口、设计结果导航丢失设计panel，均已修复；Root对两项受影响场景单独复验通过（输出块`4d2db5`），当前UI去重后为9项。未完成的可选来源选择器交互测试已移除，不据此宣称该交互通过。

合同生成／检查通过，生成505个Schema、262条API路径；新请求为闭合DTO，action对应互斥payload，preview只读且不要求幂等key。5024个中英文目录键成对，自有生产硬编码检查为0。原生bundle、Control、Runner及Web各构建1次；Web约78秒，现有大分块警告保留，没有把构建当作GUI或生产验收。

## 实际本机协议闭环

独立新SQLite、测试身份、Control、Runner、当前原生bundle及自有loopback模型协议端点完成真实HTTP闭环。Node为v22.16.0，未使用商业厂商模型。公开结果入口为`.cache/e2e/intent-114/actual/public-result.json`，包含实际工具／运行／账本／资源ID、二进制与运行时摘要、断言及关闭记录。

实际执行只有1个模型请求、1个分析run和1个attempt，离线probe没有调用模型。模型返回完整结构化原件，ready及只读预览时没有业务草稿；明确采用后，真实需求版本1、该需求设计版本1／修订1、知识版本1／修订1共同保存。设计引用前序步骤实际生成的需求ID；来源、提案、原模型SHA及结果关系一致。原apply请求恢复同一批真实ID，没有新增草稿或模型调用。

真实结果中需求仍为draft，设计及知识未发布，没有正式任务采用、开发／测试派发、外部效果或人工Review。初次模型分析的执行事实与后续草稿采用分别核对，不把模型run成功当作任务交付。

单条去重用量账本记录协议端点报告的输入23、输出17，共40Token，固定本地费率计算估算57微元人民币。它们是自有协议夹具报告的计数及合成估算，不是厂商分词、模型效果、商业账单或业务验收。原额度预留／结算、组织与项目余额、正式原生工具证据及Runner原结果ACK均由本次实际协议核对，apply与回执重放未追加模型请求。

协议验证helper退出0；自有Control、Runner、probe和模型监听均已确认关闭。原件记录Control／Runner以SIGTERM关闭、exit_code为null，不宣称它们退出0、Windows平滑停止或数据库checkpoint完成。关闭后数据库与WAL／SHM仅作摘要保留；数据库完整性或优雅checkpoint不能由文件hash推断。原始日志、数据库、凭证、截图、构建及依赖继续留忽略目录，不进入Git。

## 验证与完整交付边界

本阶段没有真实浏览器／原生GUI旅程证据，jsdom组件检查、真实HTTP、Node协议及构建分别记录，不能互相替代。上一阶段的Windows浏览器URL安全策略缺口见[T-TRACE-113原记录](requirement-traceability-verification.md#浏览器缺口与交付边界)；本报告不把它改写成本阶段新发生的浏览器拒绝，也不声称已经补齐该缺口。

本阶段证明当前本机草稿计划的有限闭环及定向失败恢复；没有跨服务全局原子性、持久自动批准、通用配置模板／实例／秘密引用／连接验证、外部效果恢复、完整CLI／桌面智能旅程、商业厂商、物理多机、OceanBase／PostgreSQL实集群或生产验收证据。

完整P0–P7及需求1–26保持进行中：58项限定实现、34项待办、92项总计不变，`FND-INTENT-001`继续未勾选。当前源码、产物及实际检查的最终摘要以[本阶段JSON](intelligent-operations-summary.json)为准，不继承为全平台通过或完成。
