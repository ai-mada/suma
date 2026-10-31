# Codex原生快速连接合同

T-CODEX-QUICK-CONNECT-175；suma 1.0.0；2026-10-31。状态：限定实现、组件及本机真实目录验证通过；完整桌面旅程与生产继续交付。负责人：主集成、Runner、桌面、工作台。完整31／P0–P7与58／34／92范围不变。沿已实现的[原生执行](codex-app-server.md)、[工具确认](codex-human-requests.md)和现有托管Runner补齐真实快捷入口，不新增派发器或模型服务。

## 固定用户路径

工具列表选择Codex原生执行 → 明确检测本机安装 → 填写提供方ID、Responses地址、可选凭据环境引用及Windows隔离实现 → 明确读取真实模型目录 → 连接原托管Runner → 依据新库存登记为暂停 → 明确启用。任务的模型／推理档位继续从原正式目录选择，不在连接表单复制第二个选模器。普通浏览器显示本地桌面入口与服务端Runner指引，不伪装能够检查浏览器所在机器。

## 私有执行配置

新增`DesktopManagedCodexConfiguration`：`executable,executable_sha256,reviewed_version,provider_id,base_url,configuration_sha256`必需；`api_key_env_ref,windows_sandbox`可选，但Windows必须明确sandbox。仅主进程保管，写入原加密Journal；renderer不能指定路径、任意argv／env或密钥原值。固定构造`codex app-server --listen stdio://`，复用现有配置校验、摘要版本3、隔离HOME、版本及实际目录探测。

`DesktopManagedRunnerInput.codex_configuration`只允许adapter=`codex-app-server`；与`acp_configuration`互斥。启动前复核当前原生文件、SHA、版本与配置摘要。配置保存不等于连接健康；connected必须来自本进程启动之后的真实新库存，精确匹配adapter／protocol、执行文件SHA、完整版本、配置SHA及有效原生目录。原租约、当前账号、项目、工作区、HMAC登记、暂停／独立启用不变。

新增私有CLI模式`--describe-codex-models-stdin`，与服务、maintenance、其它检测／执行模式互斥，在服务／数据库初始化之前处理。输入`CodexModelDiscoveryInput`为adapter_id、安装三项与provider_id/base_url、上述可选两项；最大16KiB、闭合字段。输入不包含goal、模型选择、仓库、worker凭据或任意命令。

Go结果`CodexModelDiscoveryResult`字段固定：`protocol="suma-codex-model-discovery-v1",state,executable_sha256,tool_version,configuration_sha256`，可选`catalogue:CodexCatalog,error_code`。state为ready／failed／unknown；ready必须有有效且同配置的原`CodexCatalog`、无error；其它状态不得有目录。错误码仅`codex_discovery_invalid,codex_discovery_unknown,codex_discovery_installation_changed,codex_discovery_credential_unavailable,codex_discovery_catalogue_unavailable,codex_discovery_not_stopped,codex_discovery_failed`。不伪造Prompt／费用／OS隔离测量；探测只发送initialize及model/list，不发送thread／turn。

## 复用现有桌面管道

原`manageLocalTool` IPC继续拥有连接／状态／停止；无需扩展原主进程窗口或新renderer权限。

- Codex探测请求与原scope（project_id/user_id/tenant_id/adapter_id/locale）同源，adapter固定`codex-app-server`。action=`detect`；或`models`并携`installation_id,operation_id,configuration`；或`model-status`及可选operation_id。configuration为`provider_id,base_url,api_key_env_ref?,windows_sandbox?`，没有安装路径或密钥原值。
- 探测响应沿既有`{status:'ok',observation}`形状，observation为原四项scope、phase、installations，以及可选probe。phase使用unconfigured/detected/not_installed/wrapper_only/unavailable/needs_workspace/authorization_required/binding_changed/reading/ready/failed/unknown/expired/stop_unconfirmed。安装行仅installation_id/label/kind及可选tool_version/executable_sha256；kind为native/wrapper_only/unverified。
- probe为`id,operation_id,installation_id,source_sha256,created_at,expires_at,discovery?`，discovery为上述Go结果。原操作持久保存，60秒依据有效期；status只读取，未知时只核对原body/key，不重新运行探测。source绑定当前项目工作区、安装、配置和主进程凭据HMAC，不公开原秘密。
- 连接携`codex_selection:{installation_id,probe_id}`和`expected_configuration_sha256`，与acp_selection互斥。私有配置由主进程从准确原探测取出；重连原操作复用原配置，不允许刷新或新草稿替换未结操作。
- 明确停止且原子进程已关闭后，用户可提交新selection并重新核对当前安装、凭据来源、工作区及有效目录，以采用新配置；保留原机器登记body/key与身份。普通重连和所有未结状态继续原配置，不静默更换。
- `DesktopToolConnection`对该adapter增加可选tool_version/executable_sha256/configuration_sha256/catalog_sha256，connected时必须有完整有效事实。这里没有固定model：任务选择仍由正式目录／Binding决定。

## 简洁交互与效率

同一连接卡按“检测 → 必要配置／读取目录 → 连接”渐进展示，技术摘要折叠，未知操作优先核对，配置草稿按当前人／组织／项目／adapter隔离。参数只接受真实能力；安装、配置有效、模型目录、执行节点新库存、登记与启用分别表达。原因码给明确下一步，不自动安装、借用用户登录、降级隔离或悄悄启用。

复用成熟现有原生协议及组件，中文／英文完整。按层并行实现；仅新增当前来源、私有边界和主流程所需定向检查，复用未改动174执行／技能／质量证据。不重复全量测试或跨平台矩阵。必要时进行一次代表交互，禁止让非功能环境审计阻塞功能交付。真实厂商账号、模型、费用、跨平台及生产另记边界。
