# Claude Agent SDK 原生协作合同

T-CLAUDE-SDK-192；速码1.0.0；原31／P0–P7／58限定／34待交付／92总账继续。本阶段使用官方TypeScript SDK0.3.314及已审CLI2.1.314，接通一次活执行的工具许可、结构化提问、候选和独立质量；原claude-jsonl-v1前台CLI与客户端MCP方向保持各自真实合同，不冒充SDK。

## 单一控制面

新目录入口claude-code-sdk、adapter=claude、protocol=claude-sdk-v1。CommandSpec.ClaudeSDK仅管理员本机固定配置，Node／完整宿主资产、原生CLI绝对路径与摘要、SDK／CLI版本、准确模型及明确API key环境引用进入原配置摘要。公开ClaudeSDKExecution不暴露路径、参数或凭据。model_source=administrator_sdk_option、budget_mode=observe_only、identity_evidence=sdk_init_model_ack_only、auth_strategy=api_key_env_ref；两级enforce策略仍拒绝，SDK成本估算不是正式账单或硬上限。

复用原Worker、DAG／Pool、当前权限、Profile、独立工作树、Journal、候选、同提交独测和人工验收，无新业务表或派发器。SDK使用default权限模式，固定Read／Edit／Write／Glob／Grep／Bash／AskUserQuestion；不传updatedPermissions、bypassPermissions、resume、动态MCP、plugin或子代理。既有CLI权限规则和SDK canUseTool回调是不同入口；SDK回调仅表示确实到达的native prompt，不虚称覆盖所有供应方内部动作或提供OS沙箱。

## 原生交互

internal/claudecontract为共享来源。Basis包含真实SDK session_id、tool_use_id、request_id、宿主生成callback_id、configuration_sha256及原input_json UTF8字节的params_sha256。request_id来自固定SDK实际类型公开的callback options，不能由用户填写或借用宿主生成ID。input_json必须闭合有界、无重复字段／非法Unicode；最多32个回调。许可只对该次callback allow_once或reject_once，原参数原样回给SDK，不改输入或长期规则；凭据／无法完整显示参数应拒绝，不用脱敏摘要批准隐藏动作。

AskUserQuestion按固定SDK输入解析1–4个问题、每题2–4个选项，保留完整原question／header／label／description和multiSelect。平台q1／o1身份仅由原输入顺序派生；服务端复核派生值与原JSON一致。用户明确选择或取消，无默认答案，不把问题答案当命令授权；原问题文字及标签重建SDK answers。暂不接自由文本、计划模式或未经审阅的新扩展。

ToolPermission及ToolInput沿原API／表增加Claude互斥来源；旧Codex观察保持其真实字段。Claude观察使用写前、写入、execution_closed事实，request_resolved必须false；写给本地SDK宿主不是供应方ACK。观察／答案摘要采用UTF8长度前缀，绑定完整来源与决定，不依赖Go／JavaScript不同JSON转义。

## 恢复与证据

原机器资格、租约／fence、当前授权、版本、回执与单writer在实际写前重验。存意图后再写；失联／部分写／未知结果保留原请求，不重开Prompt或盲重答案。准确SDK整体result、所有callback写入并关闭、工具闭合及进程树停止分别证明；取消不是停止ACK。宿主／CLI来源变化立即停止并保留未知事实。

SDK init模型确认仅配置事实；候选、同提交独立测试和验收不因SDK success跳过。真实厂商账号／模型、其他OS约束／安装、多机、规模与生产单独验收。仅新增和直接影响检查，原CLI正式旅程不重跑。

拒绝计量保留两种来源：denied_callbacks是本机真实canUseTool拒绝次数（含提问取消），native_permission_denials只记录原SDK最终result.permission_denials数组长度，未观察到该总结则省略。不能取max、假去重或由一方推造另一方；成功必须二者均已确定为0，原平台保存的reject／cancel决定也独立阻止成功。

## 并行职责

Root：本合同、claudecontract／toolcontract／Schema、设计文档、集成与Git。Runtime：packages/claude-runtime及SDK精确依赖。Runner：本机配置、原native进程桥和Journal。Control：原权限／输入联合、授权、HTTP、预算及固定计划。工作台：原卡片、配置、双语及薄桌面，复用原连接和fresh库存。所有阶段仍须可运行纵向验证，源码不等于厂商验收。
