# 模型辅助知识／规范／技能提炼合同

阶段：T-P4-026；版本：1.0.0；状态：实施中；负责人：平台集成。完整 P0–P7 及移动端等目标保持进行。

## 主路径与门禁

当前浏览器项目写入者选择 1–8 个真实资料／任务／需求／问题来源，固定 kind/id/version/quote，选择 memory/spec/skill、目标、实际可用专用模型工具及输出预算，明确同意把选中内容交给该固定模型。服务端解析当前来源原文、版本及 SHA，完整 prompt 不超过 32,000 UTF-8 字节，不静默截断。复用现有 Go 托管 DAG，单个 agent 节点、最多一次模型调用、MaxRetries=0；不另建调度器。

输出是提案：可编辑、可核对原件、明确采用为 `model_assisted` 项目草稿。草稿写入与人工明确选择需要当前浏览器 ProjectWrite；正式发布仍按既有 BrowserProjectApprove。不得自动发布、组织共享、安装技能、编辑代码或授予工具权限。后续组织目录共享复用 T-P4-025。

## 执行合同

`Profile.KnowledgeExtraction` 为显式 capability：protocol=`suma-knowledge-extraction-v1`、model、endpoint_origin。仅 Suma、read_only、无 workspace；与 Planning 专用 profile 互斥。固定本地配置通过实际 `--knowledge-extraction-probe` 离线能力发现后上报，不能把配置成功当作在线或可领取。

原生 request `purpose=knowledge_extraction`、max_steps=1、max_tokens 1–8192（默认4096）。没有控制面工具或工作区工具；不读取其他资料，也不允许模型请求工具。输出完整 JSON 单独封装为 final evidence，用量沿用真实 Runner final evidence 与原额度事务；同一 profile 的固定模型／origin 与费率版本进入账本。规划与提炼可复用计价机制，不能以新 purpose 绕开预算。

领取前重核创建凭据、当前项目写权、来源、固定 profile／版本／路由。已开始执行保持封存来源；心跳继续重核实际创建者与执行权限；结果核验当前 Runner／Pool 身份、固定 attempt／fence 和工具证据，保存已经发生的输出与真实用量，不因发起者退出或来源变化抹掉原件。完成回执不能授权新的执行或发布。采用前再次核验固定来源；已变化时 `basis_current=false`，禁止采用。撤权或失联仍按既有停止、租约及核对语义处理，未知用量不能当零。

## 结构化提案

`KnowledgeIntelligenceOutput={schema_version:1,title,summary,content,source_indices,rationale}`。title 1–200 字节，summary ≤1000，content 1–16000，rationale 1–8000；source_indices 为不重复、0-based、指向封存来源的索引子集，1–8 项。服务端严格解析，不接受模型自造来源、版本、SHA、审核者、权限或命令。原件单独保存并计算摘要，结构错误显示提炼失败，不能伪造 ready 或自动重跑模型。

未知观察先独立保存，不能把空／部分观察当作模型原件。原件在首个已知结果固定。同一个未停止、租约仍有效的只读提炼尝试，可由当前执行身份、固定工具与创建者权限共同核验的正式完成回执自动核对；不重新调用模型。原 unknown 与 final 同事务保留，业务格式错误仍记录真实用量并使提案失败。普通 CLI／规划、过期租约、已停止、取消、旧 worker／fence 不走此路径，继续原核对协议。

模型原提案与人工修订分别保存。明确采用时，现有知识草稿、固定来源、不可变 model_adoption 关系、analysis CAS、活动与回执同事务提交；回滚不能漏出半个草稿。后续草稿编辑和发布不改写最初模型原件及采用关系。

## 公共接口

- GET/POST `/knowledge-intelligence/analyses`：分页读取／创建一次提炼执行。
- GET `/knowledge-intelligence/analyses/{id}`：当前权限下的运行、依据、原件、提案及可用操作。
- POST `/knowledge-intelligence/analyses/{id}/commands`：revise、adopt、dismiss；采用返回同一 Analysis，带 adopted_knowledge_id。

Analysis 状态为 running/ready/failed/reconciling/adopted/dismissed。Create 固定 project_id、goal、locale、kind、sources、tool_profile_id、expected_tool_version、max_steps=1、max_output_tokens、acknowledge_model_use、idempotency_key。命令携带 expected_version、稳定 key、reason，adopt 须 acknowledge_draft。权限先于回执核验，响应丢失恢复原操作，CAS 冲突保留编辑内容并重新核对。

## 交互与并行边界

在项目知识页增加默认折叠“智能提炼”，保留原“原文提取”语义。来源、模型、输出预算与确认在一个简洁表单；运行展示真实链接和事件，原件／来源按需展开，采用后提供草稿入口。无专用工具时明确指向工具设置；全部产品文字提供中文和英文，原始资料与模型内容不随语言切换重写。

后端代理拥有 knowledge_intelligence*.go；Runtime 代理拥有专用 capability、Runner／探测和原生 TS purpose；UI 代理拥有独立 KnowledgeIntelligence 面板／样式／领域词条。Root 管 HTTP／数据库／回调与 admission、计价、新旧知识溯源、生成合同、工具设置入口、文档及实际联调。仅运行必要权限／事务／purpose／来源／费用切面与一次本地模型协议真实流程，不把夹具当作厂商或生产验收。
