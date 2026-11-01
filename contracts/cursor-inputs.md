# 原执行提问与计划确认

T-CURSOR-INPUTS-190；速码1.0.0；完整31／P0–P7／58限定／34待交付／92总账继续。基于189实际Cursor包与当前工具输入深模块，支持 `cursor/ask_question`、`cursor/create_plan` 在同一活ACP会话中等待明确答复。复用原表、HTTP、普通／Pool权限、CAS、回执与Journal，不新增调度器或自动审批。

## 共享合同与所有权

- Root：`internal/cursorcontract`、`internal/inputdelivery`、`internal/toolinputcontract`、toolcontract／acpcontract、生成合同、文档和Git。
- Backend：`internal/platform/tool_input*`及相关精确测试，原输入／权限共享32条计数；源摘要按runtime／方向／session与原typed请求ID隔离。
- Runtime：runner／acpstdio及直接相关测试；真实native Codex观察保留，显式映射通用送达观察。核心ACP SDK保持原依赖，只有现严格帧层接两种确切方法。
- Web：现输入模型、Card／Panel／Workflow消费者和必要双语；不新建页面或任意RPC表单。薄桌面仅必要配置识别，不新开默认批准开关。

`ToolInputRequest` 为严格独占分支：原protocol＋可选Codex指针，或acp-stdio-v1＋可选Cursor指针。答案亦为Codex原selection，或Cursor专用Decision。字段不能混用，不接受客户端native JSON／planUri。Cursor Profile 的 `user_input_protocol` 固定 `cursor-acp-input-v1`；Qoder不宣称该扩展，Codex实验性opt-in不变。

Cursor source包含准确SessionID、原Prompt typed ID、原server typed request ID、method、配置SHA及参数SHA。问题／计划payload保留完整原内容；参数SHA为严格已校验、默认值规范化后的method＋payload摘要，不伪称原始wire字节摘要。答复SHA绑定原参数摘要与重建的typed原生响应。

## 交互与语义

问题严格使用原question／option IDs。单选恰一项，多选至少一项、无重复；提交须覆盖全部原问题。支持整体跳过或取消，不捏造free text／Other／secret。上限16题、每题32项；计划正文最多128KiB、规范化完整payload最多256KiB，待办与阶段分别有界，超限明确拒绝。

计划完整只读展示原文、概述、待办及阶段；没有预选决定。明确接受、拒绝或取消由原当前项目写权限者通过原InputID、CAS和操作ID保存。typed accepted不传URI，原工具可能在本attempt私有HOME `.cursor/plans` 写入计划文件；该文件不是正式候选、需求基线或验收依据。拒绝／取消必须正常typed outcome，禁止以RPC error替代，防止厂商fallback。

保存、执行器观察、开始写、完整写、原执行关闭和进程停止分别留证。中性 `inputdelivery.Observation.ExecutionClosed` 描述执行关闭；Codex真实TurnClosed显式投影，Cursor不制造RequestResolved或原生接受ACK。源变化、暂停、撤权、预算、租约及失联沿原门禁；unknown核对原意图，不重答、不重发Prompt或恢复旧session。

同意计划不授命令／文件权限；后续操作仍走原一次工具权限。模型、预算、工作区和正式任务保持原绑定；候选封存、同提交独立测试、人工验收及问题回流继续原链。

## 证据与效率

官方依据：[Cursor ACP](https://cursor.com/docs/cli/acp)；实际包 `2026.10.22-27dcf5f` 的3635／2232模块已只读核实问答、计划回退及私有计划文件行为。真实供应方账号／Prompt未验，不能用受控协议测试代替厂商证明。只验证新增／直接受影响行为，原187／189其他绿色证据复用；界面一次隔离构建、代表中英与键盘状态，不覆盖活动预览。
