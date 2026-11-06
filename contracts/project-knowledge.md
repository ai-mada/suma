# 项目知识、记忆、规范与技能合同

状态：T-P4-015 限定范围本地已验证，2026-10-25。负责人：平台集成。既有完整产品目标继续有效；本阶段采用现有 Go／SQL／React／Radix／i18next，不新增运行依赖。

## 目标与真实边界

资料可以沉淀为可审阅、可追溯、可复用的项目知识，再明确选择到开发任务的固定上下文。类型为 `memory`、`spec`、`skill`；skill 是已审阅的文本工作说明，不安装插件、不执行脚本、不授予工具权限。当前提取是有来源的文本提取 `extractive`，不能称为已接通模型摘要；后续模型提案必须经过同样的草稿、来源和人审门禁。

资料导航保留“文档／检索”，新增一个“项目知识”子页，以类型筛选和详情承载记忆、规范、技能，不增加三个顶层菜单。所有新产品文字提供 zh-CN／en-US，用户正文、来源原文、版本及哈希保持。

## 聚合与修订

- `suma_project_knowledge` 保存同租户、同项目的当前聚合；`suma_project_knowledge_revisions` 保存不可变修订快照。写入、审计和回执同事务。
- 每次创建、编辑、发布、退役均追加修订，聚合用 `expected_version` CAS。发布复制所审阅正文到新的 published 修订，保留来源、方法、作者与时间；历史不被覆盖。
- 编辑已发布知识生成草稿，旧 `published_revision` 在明确发布／退役前继续可用。详情同时显示 latest／published；Agent 和任务采用只读取已发布快照，不泄露新草稿。
- `published_eligible` 表示当前来源、发布状态可用于新派发；不是模型效果、执行成功或事实正确性的自动保证。资格码为 none/current/source_changed/retired/publication_changed，界面按码本地化。
- 正文最多 16,000 UTF-8 字节、标题 200、说明 1,000；最多八个来源。拒绝超限，不静默截断。列表分页最大 100，筛选、游标绑定当前项目与条件。

## 来源与提取

来源固定 `kind/id/version/quote`，kind 只允许 documents／requirements／tasks／issues。服务端在当前权限事务内核对同项目当前正式记录；archived／deleted 不可用。可见文本来自原说明、文档内容以及需求／任务验收条目，标题是独立元数据。

非空 quote 必须是可见来源正文中的原始连续片段；空 quote 取完整可见文本，超限要求选择片段。保存来源标题、版本、原文与摘要；不从调用者接收可信 snapshot、approved_by 或 source hash。manual 草稿可无外部来源，extractions 必须至少一个来源，所有草稿都需要明确发布才能采用。

来源变更或隐藏后，新发布和新派发阻止使用过时依据。旧修订、旧运行和已接受的任务上下文保持原样；退役不是对既有运行的自动取消，停止与恢复仍走原执行合同。

## 权限与接口

复用 Actor／当前事务 proof：草稿创建和修改为 ProjectWrite；发布／退役为 BrowserProjectApprove；普通读取为 ProjectRead。Agent 仅通过专用工具取得明确已发布修订，不能调用资料管理接口、编辑或批准。撤权／过期／退出后的新写与成功回执重放均拒绝。

| 接口 | 行为 |
|---|---|
| GET /project-knowledge | kind／state／q／limit／cursor 筛选分页 |
| POST /project-knowledge | 完整 manual 草稿，expected_version=0 |
| GET /project-knowledge/{id} | 当前摘要、latest 和可选 published 快照 |
| GET /project-knowledge/{id}/revisions | 当前授权的不可变历史，limit／cursor 分页 |
| PATCH /project-knowledge/{id} | 完整草稿替换，expected_version>=1 |
| POST /project-knowledge/extractions | 读取正式来源，创建可编辑 extractive 草稿 |
| POST /project-knowledge/{id}/commands | publish／retire、当前批准权限、版本与稳定 key |

写入绑定原页面账户／组织，保持原 key／body／caller 的未知结果恢复；严格 DTO 拒绝额外可信字段、分数或指数版本。与全平台一致，权限在回执之前核验。

## 从 SKILL.md 创建草稿

浏览器将用户选择的单个本地 `SKILL.md` 作为草稿输入。按 UTF-8 读取，文件上限 32 KiB。校验 YAML 前置信息中的 `name` 和 `description`，拒绝重复键、别名、无效名称和空正文。说明和 Markdown 正文分别受当前知识合同的 1,000 和 16,000 字节上限约束。名称映射为标题，说明映射为摘要，正文保留 Markdown 并将 CRLF 统一为 LF。附加元数据和文件留在原技能包。

用户在导入编辑器复核并明确保存后，沿 `POST /api/v1/project-knowledge` 创建 `kind=skill` 的人工草稿。文件路径、原文件及外部工具授权字段不进入请求。发布、修订和执行引用继续由现有知识权限及事务控制。

CLI 的 `knowledge import-skill --file PATH_TO_SKILL.md` 按同一输入范围生成可审阅的 JSON 草稿。预览在本机完成，选填 `--project` 时将项目写入结果。用户明确执行 `knowledge create --file` 后，沿原项目权限和幂等合同保存。预览不生成操作标识。相同调用者、目标和原 JSON 可用于核对结果未知的提交。

## 任务与智能体消费

`TaskDispatchPreviewRequest.knowledge` 显式选择最多十六个 `ProjectKnowledgeReference{id,revision,sha256}`，默认空。服务端从批准的不可变版本解析出 `TaskContext.knowledge`；前端或外部工具不能提供正文快照冒充已批准输入。

消费上下文仅携带一次批准正文和来源的 kind/id/version/title/sha256，不重复附带相同 quote；完整来源原文继续保存在不可变修订与历史 API，摘要仍绑定完整批准修订。

固定任务、需求、知识和流程指令的总 goal 仍最多 32,000 UTF-8 字节。超限要求减少选择或拆分，不自动删验收条件。启动在同一当前权限事务内复核已选 published revision、摘要、当前来源；草稿编辑不改变旧 published 输入，新发布、退役或来源变更使原未启动预览失效。已运行定义保持原固定内容。

Agent 控制面新增只读 `project.knowledge`，参数是明确 references；按当前 Agent scope 和原来源核对后返回发布快照。该文本是任务数据，不增加模型、插件、文件或命令权限。

## 验收与开发规约

1. 文档选择与提取→编辑→发布→任务选择→预览／启动→固定 goal 与来源可追溯；刷新及中英切换保留用户工作位置和原文。
2. 版本 CAS、同 key 重放、来源变更、旧发布与新草稿分离、退役、错误项目、API／Agent 范围、批准权限、退出后的 fresh／replay、预算上限必须有定向证据。
3. 搜索来源打开复用统一路由，问题结果进入“质量／问题”；不把 locator JSON 当成可操作来源入口。
4. A 后端只写 project_knowledge*；B 前端只写资料工作区／导航／派发展示；Root 管入口、任务上下文、Agent、生成合同和整合。只跑改动相关组，复用 014；原始运行数据和截图留忽略目录，阶段验证后只本地提交必要文件。

实际证据与待验范围见[阶段验证](../docs/quality/project-knowledge-verification.md)。
