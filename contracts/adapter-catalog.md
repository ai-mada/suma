# 工具接入目录合同

阶段：T-ADAPTERS-139；速码 suma 1.0.0；能力连接领域。目标是在同一入口解释工具的接入方向、源码能力和配置路径，沿用[现有工具矩阵](../docs/integrations/development-tool-matrix.md)及各工具正式合同。交互见[接入设计](../docs/design/adapter-connections.md)。实现与真实验证由阶段报告确认，本合同不代表新增厂商执行适配。

## 唯一只读接口

`GET /api/v1/adapter-catalog?project_id=...` 返回 `AdapterCatalogPage {project_id, as_of, items}`。当前人员项目读取权在服务端重新验证；允许有当前资格的浏览器与用户API身份，Runner／Agent拒绝。仅接受唯一非空project_id，未知或重复参数拒绝；响应为 `Cache-Control: no-store`。

items是随代码维护的目录，规范上限64项；不是用户注册表，没有本接口的POST／PATCH、安装、探测或派发动作。as_of表示当前目录读取时间，不能充当设备最后在线时间或提供方探测时间。

`toolcontract.AdapterDescriptor`：

| 字段 | 含义与限制 |
| --- | --- |
| id、product | 当前目录项的稳定身份与产品名；同品牌不同入口有独立身份 |
| entrypoint_kind | cli、sdk、app_server、mcp_client、cloud_api |
| direction | suma_to_tool：平台通过指定适配执行；tool_to_suma：外部客户端调用平台；二者不能互相推断 |
| adapter、protocol | 可选的现有固定适配器与协议；不能凭产品名猜测配对 |
| implementation | implemented：当前入口源码已实现；guide_only：只有接入指南；planned：待实现。都不代表实际健康或厂商验收 |
| setup_kind | runner_profile、client_export、manual_guide；决定前端前往哪个现有配置流程，不包含可执行命令 |
| capabilities | 明确的AdapterCapability列表 |
| source_refs | 可回查的仓库源码／正式合同引用，不是网络下载或任意执行目标 |
| limitations | 用于本地化解释的限制机器码，不是待执行脚本或指令 |

`toolcontract.AdapterCapability {key, support, source_refs, limitations}` 的support只允许supported、unsupported、unverified。缺少证据不得按supported显示。

key枚举为task_start、history_read、session_load、new_turn、interrupted_resume、cancel_request、terminal_observation、host_stop_proof、usage_observation、provider_billing、candidate_capture、independent_test、model_selection、gateway_budget、workspace_read、workspace_write、human_input、configuration_export、project_access。

这些字段说明源码入口的能力，不代表每个安装实例都启用了它。配置实例的能力投影仍验证原Profile及其明确能力字段；静态目录不放宽ValidateProfile、当前项目授权、模型准入、工具配置或原调度门禁。

## 必须保持的事实边界

- 源码implemented、设备已安装、已报告探测、配置已启用、当前可派发、执行成功和业务验收是不同事实。
- history_read、session_load、new_turn和interrupted_resume分开；支持读取会话不授予安全重跑资格。
- cancel_request、terminal_observation、host_stop_proof分开；取消请求成功不证明进程停止。
- host_stop_proof在静态目录和Profile投影均为unverified；没有具体OS／attempt停止证据，不能从进程封装能力推断分离子进程已停止。
- candidate_capture目录所示源码路径需要工作树、制品配置、成功执行及干净的新提交等条件；Profile可写时仍只投影unverified，只读时为unsupported。它不是本次运行已有候选的事实。
- usage_observation、provider_billing与gateway_budget分开；观察到Token不证明供应商账单或硬费用上限。
- model_selection只表示对应入口支持指定模型，不证明模型可用或供应方模型身份。
- workspace_read／write、candidate_capture和independent_test分开；工作区可写不代表候选已封存或独立测试通过。
- WorkBuddy云任务与CodeBuddy CLI各自登记；CodeBuddy的实现、探测或任务结果不能证明WorkBuddy云服务接入。
- Zed等编辑器调用速码的方向不能变成速码可反向启动其无头执行器的声明；配置导出也不代表客户端已连接。

本接口不返回凭证、命令正文、机器目录、实时健康、费用或执行授权。当前安装观测继续使用tool-discovery及其新鲜度、设备／项目与凭证资格；后续配置和派发沿原强类型领域命令。

unsupported只说明速码当前这个入口未提供对应能力，不断言厂商产品整体不具备该能力。

## 演进与验证

新增入口须同时给出独立身份、方向、协议、当前实现状态、来源与限制；能力改变先更新原适配实现与回归，再更新目录，禁止仅改supported字符串宣称集成完成。

本阶段验收覆盖当前权限、闭合查询、方向与能力枚举、限制显示、设备发现和原配置流程的正确接续。厂商真实账户、所有平台安装、云任务核对、完整会话恢复、物理多机与生产验收继续按[唯一实施计划](../docs/delivery/implementation-plan.md)推进。
