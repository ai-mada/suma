# 项目知识发现：下一交付切片

版本：1.0.0；状态：待实现；负责人：平台集成；依据：T195 当前源码只读审计。当前阶段优先完成工具维护与未知执行核对，本页保留下一知识切片，不代表该能力已交付。

## 已有能力与准确断点

已验收交付可以形成固定来源，经过模型提案、人工采用草稿、独立发布，再按准确修订进入后续任务。复用[交付知识合同](../../contracts/delivery-knowledge.md)、[知识提炼合同](../../contracts/knowledge-intelligence.md)、[项目知识合同](../../contracts/project-knowledge.md)和[Codex 原生技能合同](../../contracts/codex-native-skills.md)，不重建 T172／T173 的链路。

当前断点是：**已发布的记忆、SPEC 和技能尚不能通过统一检索被发现并便捷选入任务上下文**。

| 当前实现 | 具体边界 |
| --- | --- |
| `searchKeywordsTx`，`internal/platform/search.go` | 只查询 requirements、tasks、issues、documents 四类正式记录，不查询项目知识发布正文。 |
| `SearchIndex`，`internal/platform/projector.go` | 外部索引命中统一按 documents 读取原件并重新授权，不能据此接受项目知识命中。 |
| `ProjectIndexes`，同上 | outbox 载荷按 Record 解析；项目知识发布／退役尚未接入带领域来源的投影。 |
| `CommandProjectKnowledge` | 已有不可变发布／退役、版本、当前来源及操作回执；当前不产生项目知识索引事件。 |
| `project.search`／`project.knowledge` | 前者复用四类记录搜索，后者要求调用者已知准确 id／revision／sha256；不能替代可发现目录。 |
| `TaskDispatch` | 通过已发布列表手动选择知识，固定引用进入原预览；当前没有基于任务的知识检索推荐。 |

因此，发布成功不能显示为“记忆已自动同步到检索”，索引命中也不能直接显示为“已用于开发”。

## 建议完整交付链路

```mermaid
flowchart LR
    A[准确发布修订] --> B[检索发现]
    B --> C[原文与来源预览]
    C --> D[明确选择准确引用]
    D --> E[原任务预览与启动复核]
    E --> F[固定执行上下文与结果证据]
```

第一步可完成当前 SQL 的已发布正文检索，复用当前权限和来源资格核验，返回准确修订引用与紧凑片段。结果提供类型、来源和发布版本，能够预览并明确选入原任务表单；不自动勾选、调用模型或启动任务。此范围应标为关键词检索，不宣称语义索引已完成。

后续语义投影复用原事务 outbox、`KnowledgeIndex`、LanceDB／OceanBase 后端和既有分代／墓碑规则。发布与索引事件同事务保存，来源增加明确领域身份；退役生成墓碑，重复或乱序事件不得复活旧发布。不能把知识伪装成 documents Record，也不能为索引另建业务派发器。

检索结果进入界面或模型前，必须按当前项目权限、发布修订、摘要、状态和原来源资格重新解析。草稿正文不能覆盖当前发布正文；新发布、退役、来源变化或撤权必须阻止新采用。索引尚未追平时显示实际覆盖与滞后，不以空结果代表全项目没有知识。

最终选择沿用 `ProjectKnowledgeReference{id,revision,sha256}`、`resolvePublishedKnowledgeTx` 和原任务预览／启动事务。保留完整修订证据，模型侧继续采用紧凑来源投影与原输入字节上限，不静默删减验收条件。

## 实施与验收重点

1. 控制面负责准确已发布检索、来源资格、引用解析与原任务接口复用；语义扩展时负责事务事件和投影命名空间。
2. 界面复用原检索结果、知识详情和任务选择区，保持中文／英文、键盘操作、加载／失败／空态、项目切换及旧响应隔离。
3. 仅检查新增断点：发布正文与草稿隔离、准确引用、当前权限、来源漂移、退役、重复事件及明确选入；原已通过的提炼和技能物化不重复整套验证。
4. 真实模型消费与随后代码结果应有独立证据；目录、物化、调用请求和回合 ACK 各自记录，不能推导模型已阅读或质量提升。

外部专业记忆系统、双向／离线同步、其他工具原生技能格式、可执行插件、评测优化及物理多机仍属于独立后续交付范围。

## 证据与范围

本页依据当前控制面、Worker、知识服务和前端消费者的只读审计。T172 已有来源、采用、发布及固定上下文的定向检查；其实际界面证据是受控正式评审来源和只读 HTTP／组件流程，不能代替真实 Git／测试进程与后续厂商执行。T173 已有已安装 CLI 技能目录观察，尚不能证明真实模型应用技能。

知识相关完整待办见[实施计划](implementation-plan.md)的“知识资产语义索引投影、外部记忆同步、可执行插件技能和完整专家策略”。保留的 58／34／92 范围基线与当前 62／36／98 checkbox 数量不等，待建立稳定 ID、需求与证据映射；不据此删减原 31 项需求或宣称整项完成。

*English: This planned slice makes published project memory, SPEC and skills discoverable, previewable and explicitly selectable through the existing task context contract. Current extraction, approval, publication and Codex skill loading are reused. Keyword discovery, semantic projection, external synchronization and verified model use remain distinct deliverables.*
