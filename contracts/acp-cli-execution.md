# 本地 ACP CLI 执行与一次性权限合同

T-ACP-RUNTIME-170；速码 suma 1.0.0；2026-10-31。这是已批准工具接入、分布式任务执行和人工门禁架构的当前实现细化。完整31项需求、P0–P7及58限定／34待交付／92总账继续有效。

## 目标与复用

在原工具配置、库存观察、暂停登记、明确启用、任务派发、唯一Go DAG／Worker、独立工作树、候选和同提交独立测试链内增加本地ACP。共用引擎采用已锁定的`github.com/coder/acp-go-sdk v0.13.5`，首个厂商策略为`adapter=qoder`、`protocol=acp-stdio-v1`；`qoder-client`仍是另一个方向的IDE／MCP入口。通用引擎不自动登记任意启动器。

本轮参考Multica `e8ae72c9dfc1b4474b61a9087f51b33cbee6a048`的运行时适配、任务上下文及渐进展示行为，复用速码已有代码；当前附加许可适配源码不复制。官方Qoder入口是`qoder --acp`，使用现有CLI登录或本机明确凭据引用。Zed是客户端，不把编辑器存在登记为执行器。

## 固定配置与能力

本机管理员配置`ACPExecutionSpec {runtime:"qoder",reviewed_version,model,credential_env_ref?}`；命令固定`args:["--acp"]`、`input:"acp-stdio"`、`read_only:false`。不能从任务传exe／argv／env／会话／MCP／权限模式；拒绝yolo、自动批准、resume、原生workspace／gateway／测试／专用purpose混用。精确可执行摘要、版本、固定配置及环境策略进入原Profile摘要；版本必须与管理员明确固定值一致。Windows仅支持已核对的原生入口，官方包装器解析及其他安装形态单独验收。

公共`ACPExecutionCapability`只含runtime、协议版本、SDK版本、配置摘要、固定model、model_source=`administrator_session_config`、budget_mode=`observe_only`、identity_evidence=`configuration_ack_only`、auth_strategy=`existing_cli_login`、permission_mode=`human_once`。没有可执行路径、参数、环境值或凭据。库存可用表示固定启动条件与协议满足，不是认证／模型健康、系统沙箱或开发完成。任意配置及本轮受控夹具不能证明真实Qoder就绪。

任务`acp_execution`必须准确等于当前启用Profile的公开固定能力，`MaxRetries=0`。预览、Start、private／Pool claim和执行持续保留当前成员／owner、原完整Profile、库存、attempt、dispatch及fence检查。组织或项目enforce预算在所有原准入／accounting边界拒绝；不把ACP用量观察或选项ACK称作Go网关硬预算。

## 会话、模型及协议

一次真实前台进程：Initialize → NewSession（cwd=原attempt工作树，MCP空，客户端FS／terminal能力false）→读取明确`select/category=model`配置选项→SetSessionConfigOption→核返回同选项的`CurrentValue`精确等于固定model→一次原目标Prompt。没有准确选项／ACK就禁止Prompt，不退回默认模型。选项ACK只证明配置选择，底层供应方实际模型与费用仍未核实。

SDK默认原包日志必须关闭或安全处理；严格64KiB帧、UTF-8、重复字段、JSON深度、身份、session／tool关联与总量边界先于宽松SDK解码。输出只保存安全文本、工具状态、准确停止原因和partial／unreported用量；不存thought、原packet或凭据。工具未闭合、未知停止、异常EOF、非法／迟到结果不能推成功。

客户端FS／terminal回调真实拒绝；这不限制CLI自带工具、hooks、配置、网络或插件，不是OS沙箱。注册／启用须明确这是受信本机CLI，任务只在原隔离工作树执行。代码候选仍需真实clean新HEAD和原捕获／上传渠道；不凭文字或工具自报制造候选和测试证据。

## 持久一次性权限

运行时Permission回调生成准确原请求：`idempotency_key,session_id,tool_call_id,title,kind,details,details_complete,options[]`。原选项`{id,name,kind}`完整保留；只执行准确`allow_once/reject_once`，always从不采用。安全细节不完整、超限或脱敏时`allow_once`不可用，仍可准确拒绝。所有内容在本地journal及网络前脱敏；秘密过滤不能把被删改内容伪装完整依据。

正式权限记录绑定租户／项目、原run／node／attempt／dispatch／fence、worker和Profile。Runner凭原机器身份POST `/runner/tool-permissions`保存原请求，再POST `/runner/tool-permissions/poll`读取准确权限；Pool使用对应原命名空间及当前资源池资格。请求携带原Assignment；同key不同正文拒绝，重复请求恢复原记录。最多32个请求／attempt，单请求64KiB、细节32KiB、选项16项，原进程deadline内等待。

浏览器GET `/runs/{run}/nodes/{node}/tool-permissions?project_id=...`读取当前授权列表；POST `/tool-permissions/{id}/decision`携`project_id,expected_version,idempotency_key,option_id`作明确决定。只允许当前可操作此原执行的真人，来源变化／撤权／attempt或fence变化／租约失效后不能批准。决定、版本、原操作回执同事务；当前页被准许读取不自动授予其他执行权限。

记录pending／decided／expired／superseded和准确决定；决定已保存、Runner已观察及SDK／远端已采用分开，不把poll或HTTP成功标远端ACK。异常、失联或重启后不自动重新Prompt／重放权限。打开UI和只读刷新不启动进程。交互等待保留活进程及其执行槽位，与释放槽位的DAG审批节点不同。

## 终态、取消与质量

SDK上下文取消可能主动发送Cancel通知；它没有远端停止ACK。复用原宿主进程控制、实际Wait／Finish和Windows Job证据，分别记录取消意图、原RPC结束、主进程与后代停止。不能以通知或退出0覆盖unknown。未知作用沿原journal／outbox核对，不启动第二Prompt。

`ACPExecutionEvidence`记录固定配置摘要、请求model、选择ACK／selected_model、准确session／Prompt／停止／工具完整性、权限数量与拒绝、取消及宿主停止事实。ToolEvidence、任务、代码候选、独立测试、验收、发布各自独立。正式测试使用原准确candidate_from及不同Worker，人工验收不由Agent或SDK自动产生。

## 并行实施与验收

主集成维护共享类型、toolcontract闭合校验、目录、模型预算／派发接线、生成合同、文档与Git。协议代理独占`internal/acpstdio/`；Runner代理独占明确分配的Runner新文件及必要原接缝；领域代理独占权限表／事务／HTTP和定向测试；Web代理沿真实生成类型增加简洁权限卡及固定模型事实。接口先冻结再并行，不新增派发器、第二认证体系或历史兼容分支。

必要验证覆盖严格协议／模型ACK、权限准确once／同key恢复／当前授权／旧fence、取消／EOF／工具未闭合、秘密跨片段、原库存与派发、真实工作树候选及同SHA独立测试。只运行受影响检查；未变绿结果复用。实际使用受控协议、真HTTP／SQLite／进程／SDK与代表中英文交互，真实厂商、跨系统及规模分别留证，不以模拟冒称完成。

官方依据：[Qoder ACP](https://docs.qoder.com/cli/acp)、[CLI参数](https://docs.qoder.com/cli/cli-reference)、[模型](https://docs.qoder.com/cli/model)、[成熟SDK](https://github.com/coder/acp-go-sdk/tree/v0.13.5)。
