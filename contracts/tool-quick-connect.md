# 工具快速接入合同

阶段：T-TOOLS-151；速码 suma 1.0.0。目标是减少工具配置的复制粘贴，复用[工具执行合同](tool-execution.md)与[接入目录](adapter-catalog.md)。本文定义当前行为，不替代实际验收证据。

## 已登记节点上的自动配置

“添加工具”仅展示目录中 `suma_to_tool`、`implemented`、`runner_profile` 的执行入口。读取当前项目的 `/api/v1/release-workers` 与 `/api/v1/project-execution/runners`；两个来源均完成当前读取后，唯一活动节点自动选中。多个节点可手动选择，或在未指定目标时明确发起查找。登记状态不能推断在线状态。

所选节点沿 `GET /api/v1/tool-discovery?project_id=P&worker_id=W` 读取原上报；仅在原60秒期限内采用匹配 adapter／protocol 的可用观察。唯一匹配配置自动选择并建议名称，多个配置明确选择。重新读取不触发远程探测或延长原报告期限。

T-TOOL-NODE-MATCH-273 的明确查找最多读取 8 个已授权候选节点的原上报。仅当全部读取成功且唯一节点仍有匹配的可用配置时自动选中。多个匹配或部分读取失败时只列出已确认节点，过期结果失效。选中后照常重新读取该节点并查重。固定目标、替换配置或已选目标继续使用原路径。

登记沿原 `POST /api/v1/tool-profiles`，保留原稳定操作标识及准确项目、节点、名称、固定Profile；响应必须匹配原请求且为暂停配置。启用另走原 `/tool-profiles/{id}/commands`，使用当前版本及独立确认。注册、启用、当前可派发、模型认证与任务成功分别成立。

T-TOOL-QUICK-DESKTOP-251 为当前可用配置提供「保存并核对启用」。仅在本次保存直接返回同作用域、同配置的暂停回执后，进入原启用确认。只保存和名称框回车仍停在暂停结果。未知写入沿原操作恢复，核对动作独立确认后才发起启用请求。

T-TOOL-QUICK-RECOVERY-266 在已选入口旁提示本机未检测到、启动方式不受支持、仅检测到 Cursor 编辑器或检测请求失败。过期信息提示重新检测。重新检测只读取本机安装状态，保留已选工具、节点及名称。节点未上报、无匹配和过期独立提示。确认拒绝后可以明确重试；冲突需核对已有配置；结果未知时以原请求和原操作标识核对，禁止发起新保存。

## 桌面安装观察

preload只开放 `discoverInstalledTools({project_id,user_id,tenant_id,adapter_id,locale})`。项目、人员与组织标识均为32位小写hex；adapter_id只允许 `codex-jsonl`、`codebuddy-cli`、`dsh-multica`，locale为zh-CN／en-US。主进程核验准确父窗口、主frame和本机服务origin；探测前后分别读取真实会话、准确项目及真实会话。普通GET的Expected头不充当凭据或身份选择权。

固定已打包control程序以 `--discover-installed-tools --local-tool=ID` 运行；同名Runner CLI模式复用相同Go实现。独立分支先于业务凭据、数据库、配置和Runner启动；Runner拒绝混用执行与工作区配置，control拒绝混用维护模式。只查有界绝对PATH与固定已知npm包元数据，使用已有固定版本／帮助／协议探测；不执行shell shim、下载、登录、模型请求或任务。

本机报告为 `InstalledToolDiscovery{protocol:"suma-installed-tools-v1",items}`，最多三项；桌面一次只接收所选一项。item包含 `adapter_id,product,installed,launch_supported,reason_code`，可选 `executable,version,command`。Go的command仅在固定探测成功后给出本机预设，主进程在返回页面前删除；不会生成Profile、worker、available或执行许可。version最多256个UTF-8字节且无控制字符／行分隔符。reason_code闭合为 `not_installed`、`unsupported_launcher`、`invalid_package`、`node_unavailable`、`probe_failed`、`workspace_required`。

主进程剥除业务凭据和可执行环境覆盖，仅传普通OS环境及进程局部临时目录；子输出上限256KiB，授权JSON上限64KiB。只允许一个探测。超时／关闭取消Go探测；先软停、再有界硬停，收到真实close才解除占用。未确认关闭保持busy且不返回安装事实。原项目、账号或界面代次变化后不采用迟到结果。

## AI SOP与恢复边界

T-TOOL-QUICK-START-239 在添加弹窗中使用现有的九入口聚合扫描。当前作用域内只有一个已确认可启动、无探测诊断且在当前可执行目录中有唯一描述符的入口时，空白弹窗可自动填入该入口。自动填入受原观察到期时间、未确认停止状态及操作锁约束。预选入口、已恢复的有效草稿、固定目标和本次弹窗内的手动选择均优先。自动填入只启动原配置读取，登记仍须明确操作。

1. 优先读取当前目录、节点和新鲜上报，采用已存在的固定Profile；不根据产品名拼接执行参数。
2. 核对后登记暂停配置，再独立确认启用；过期、失权和不匹配均停止后续写入。
3. 登记或启用结果未知时保留原作用域、请求、版本及操作标识，沿原回执核对／恢复；不修改原请求或自动换新操作标识。高级粘贴／导入仍走原严格解析和离开保护。
4. 本机安装观察只用于解释安装与启动方式，不能替代Runner上报、供应方授权或派发依据。WorkBuddy云授权与CodeBuddy CLI分别处理。

当前未实现托管Runner凭据交付、加密本机journal或自动启动。后续应由受控本机组件持有私有加密journal，先记录原发行操作，再接收／保存一次性凭据并启动准确节点，浏览器不复制secret。未知发行先核对原回执；已有节点回执不含token，不能据此恢复丢失secret或静默重复发行。凭据无法恢复时须明确处置原节点后重新授权。
