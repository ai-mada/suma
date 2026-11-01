# Cursor ACP 开发执行

T-CURSOR-ACP-189；suma1.0.0；原31项／P0–P7继续。本合同把原Cursor被动发现推进到受审执行，复用现ACP SDK与当前Go派发、模型、权限、工作树、候选和质量链，不新增派发器或业务表。编辑器、被动候选、已配置工具与真实运行分别表达。

当前独立节点配置与服务停止语义见[T191](cursor-setup.md)：标准cancel只有通知写出，无session-close ACK；完整RPC后已证实的受控停止保留真实OS退出值，不等于自然退出0。原189实际未认证探测的SIGTERM仍是当时unknown证据，不回写成成功。

## 公共执行合同

目录ID为cursor-cli，adapter=cursor，protocol=acp-stdio-v1。复用ACPExecutionCapability／Evidence与原节点acp_execution；Runtime必须准确等于Profile.Adapter，不把Cursor当Qoder别名。固定字段：protocol_version=1、sdk_version=0.13.5、model_source=administrator_session_config、budget_mode=observe_only、identity_evidence=configuration_ack_only、auth_strategy=explicit_api_key_env、permission_mode=human_once。公开合同不含路径、argv、密钥值或用户登录文件。

管理员明确模型与本地密钥环境变量引用；只将原值映射给Cursor所需环境，不从原全局登录或其他工具凭据回退。Profile摘要绑定完整本地配置与实际执行宿主；当前库存、预览、Start和Claim复核相同能力。任务／讨论开发／专家团队复用当前固定工具模型，不制造平台ModelExecution／Codex选择。组织enforce预算沿原ACP门拒绝，不把observe-only伪装成硬上限。

## 原生协议与权限

采用官方agent acp，复用acp-go0.13.5原SDK会话、当前Client、Journal与普通／Pool权限API。准确模型配置ACK之后才能Prompt；会话及模型依据保持原值，不把配置选择称底层提供方身份或账单证明。正常会话、消息、工具活动、用量、权限等待与终态通过原进度上报。

只允许原session/request_permission中的明确allow_once／reject_once。不能默认批准allow_always、MCP或计划。[T190](cursor-inputs.md)通过原工具输入领域支持ask_question／create_plan，采用准确IDs和正常typed决定；计划拒绝不发会触发native fallback的RPC error。其他未支持方法明确失败，不无限等待或伪造同意。未知native副作用不重发Prompt、不自动load旧session。取消、进程Wait／树停止与供应方ACK各自留证。

## 本地宿主边界

Windows官方包采用打包Node与index.js，不能把cursor.cmd编辑器或任意shell launcher当执行入口。原厂缓存先核完整ZIP、路径及依赖闭包；实际包宿主、版本、完整性和固定参数由独立本机核验确定后进入配置。私有配置保存包根目录和完整闭包摘要，在配置计算与最终启动门复核；Node 本体摘要不能替代依赖完整性。固定参数为 `index.js --disable-auto-update --sandbox enabled acp`，不允许 renderer 改写。探测使用独立目录及明确范围，安装包不进入Git，不改变机器PATH或用户全局环境。

正式执行使用独立配置与当前工作树，只将明确环境引用的原值映射为 `CURSOR_API_KEY`。不自动导入原home的MCP／hooks／其他工具设置；项目祖先的执行配置及固定企业 hooks 存在时拒绝当前启动，不读取或删除其正文。不传force／yolo／approve-mcps／任意配置覆盖。实际私有参数与启动来源由Go深模块管理，renderer不能提交任意argv。SDK权限本身不是整个CLI的OS隔离证明；版本探测即使指定endpoint仍可能发起后台网络，禁自动更新也不等于禁网，实际支持范围和未覆盖机制必须在验证报告表达。

## 交付与验收

原工作树修改只有经准确候选封存才进入交付；同提交独立测试和人工验收沿原门，工具不自认证质量。桌面／浏览器保持一个简洁接入面板：准确安装来源、模型、密钥引用和状态；恢复保留原adapter／项目／身份／body／key，旧被动观察不授予执行资格。中英、窄屏、加载、失联和冲突沿既有组件。

Root拥有acp／toolcontract共享、目录、生成合同、文档及Git；平台复用现ACP闭合并做必要验证；Runtime拥有本机包核验、固定配置、SDK执行与本机Go桥；Web拥有原ACP接入消费者及薄桌面JS。先锁实际版本／协议再实施宿主，不凭文档编造ready。只新增／直接受影响检查，不重复188或全套；真实CLI握手、模型账户、受控协议、HTTP／SQLite和GUI分别留证。

来源：[官方ACP](https://cursor.com/docs/cli/acp)、[参数](https://cursor.com/docs/cli/reference/parameters)、[配置](https://cursor.com/docs/cli/reference/configuration)、[权限](https://cursor.com/docs/cli/reference/permissions)、[第三方hooks](https://cursor.com/docs/reference/third-party-hooks)。Multica以本地已锁定源码作行为参考，不复用其yolo参数或不严格的帧／模型回退。
