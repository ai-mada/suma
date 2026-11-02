# 远程节点已安装工具检测合同

T-REMOTE-TOOL-SCAN-205；suma 1.0.0；负责人：主集成代理。关联[原执行合同](implementation-contract.md)、[维护与停止证明](tool-recovery.md)、[接入反馈](tool-report-feedback.md)及完整[实施计划](../docs/delivery/implementation-plan.md)。完整31项／P0–P7／58limited／34pending／92总账保持active；前一轮56c7da0为已验证并推送的实质进展。

## 目的和范围

当前库存只含已配置命令；本合同接通尚未登记工具的目标机器安装检测。用户在准确节点明确请求固定目录中的工具，原持续Runner执行被动检测并持久返回，再由原指南准备配置、原库存核对、暂停登记及独立启用。安装、启动器支持、配置、提供方登录、模型可用和任务交付是独立事实。

机器以 `--allow-installed-tool-discovery` 明确允许此目的，独立于任务 `--allow-exec`，默认关闭；commands可为空。仅持续普通Runner／Pool维护owner可声明和领取，单次report-tools不声明。原维护能力增加可选 `installed_discovery:{protocol:"suma-installed-tool-scan-v1",adapter_ids:[...]}`，绑定原逻辑instance与当前身份，不成为第二能力或调度权威。Pool还须在机器本地配置中明确该项目范围，并保留当前双边grant和路由；不能把普通项目授权当成枚举整台共享机器的许可。

只支持原目录的九个准确ID：codex-jsonl、codebuddy-cli、dsh-multica、claude-code-cli、cursor-cli、qoder-acp、codex-app-server、opencode-sdk、claude-code-sdk。请求列表1–9项且唯一，明确请求的顺序固定保存，结果逐项按该顺序对应。复用一次PATH快照、原12秒总界限及进程树收集器；不对每项另起一套12秒扫描。新扫描使用隔离HOME／APPDATA／temp及工作目录，保留固定PATH解析；不读取原登录或仓库、不安装、不调用模型、不接受任意可执行路径、参数、脚本或环境变量值。

## 唯一合同与接口

Go公共类型位于 `internal/toolinstallationcontract`；长名ToolInstallationScan系列为公开Schema，短alias仅为Go实现方便。具体闭合字段与固定码以该唯一类型及校验器为准，前端类型由原生成链生成。

- 浏览器 GET／POST `/tool-installation-scans`：当前项目和准确worker的能力／有界历史读取，或明确创建；GET `/{id}?project_id=...` 精确读取原请求。列表默认20／最多100、受限cursor。
- Page包含project_id、worker_id、as_of、原ToolMaintenanceStatus、items及可选next_cursor。未声明、过期、未知与支持分别展示。
- Create为project_id、worker_id、adapter_ids、idempotency_key。当前tool_manage、活动项目、原机器登记人及普通／Pool双边grant、路由和原维护instance必须成立；业务、事件与原操作回执同事务。
- 普通机器 `/runner/tool-installation-scans/{claim,results}`；Pool机器 `/pool-runner/tool-installation-scans/{claim,results}`。原机器身份主动出站，不接入站任意命令。
- Reference为scan_id、claim_id、fence；claim nonce、request、逻辑instance、当前身份及固定source摘要绑定。重复领取恢复同一原回执，空领取也持久；租约过期不自动重新probe。

一张 `suma_tool_installation_scans` 保存请求、状态与结果，操作回执继续使用suma_operations；按当前初始schema登记，不重置现存数据或增加历史兼容回填。与原profile inspection／未知维护占用相互排斥；同项目／节点只允许一个未终结维护操作。pending期限300秒、claim期限60秒；状态迁移采用原授权事务、DB时间、CAS及fence。项目请求上限10000。

## 结果与异常语义

状态为pending、claimed、completed、failed、unknown、closed_unknown、expired、superseded。公开观察仅包含adapter_id、准确protocol、installed、launch_supported、reason_code及可选version／executable_sha256；去除内部Command、原生路径和凭据，产品名称从正式目录读取。completed需要完整请求项和原真实停止证明。

停止证明绑定准确Reference、source_sha256及不可变receipt_sha256，记录probe_returned、all_native_trees_stopped、原生树数量和已停止数量及时间；计数必填，0树允许。不得用PID缺失或租约过期代替真实停止。

- 开始前写持久started；领取或结果ACK丢失时核对原nonce、正文、操作标识和fence，不重新检测。
- failed仅表示明确未开始，并需要原真实0树停止回执；不把超时默认归为未执行。
- started但没有完整结果时为unknown，不回收并盲重probe。原结果已保存则重传原完成正文。
- 重启后只有原停止回执、没有原完整结果时，允许同ref以独立固定操作写closed_unknown；保留结果未知事实，无观察项。没有可靠回执则继续占用，不能从浏览器强制清空或靠换ID掩盖。
- 当前人失去项目读取权限时拒绝读取并清旧呈现。机器身份撤销或路由／grant／instance变化使旧结果失去当前继续资格；仍有项目读取权限的人可审计历史，source_changed明确变化，当前维护能力不沿用旧声明，不伪造新鲜健康。

## 用户交互

一个明确节点检测入口，读取当前能力后可以检测所选工具或固定有界集合；等待、失败和未知与安装事实分别表达。页面限时轮询，停止等待不是取消机器操作。创建先持久原正文／key，未知只恢复原操作；scope／caller变化停读并清旧显示，保留原私有恢复记录。

有效结果明确选择准确工具后进入原Guide同节点，不登记或启用。原Remote指南增加明确允许项和独立“仅启动节点工具检测”命令；同时带 `--allow-installed-tool-discovery --maintenance-only`。明确维护模式不要求仓库／基线／commands，不新领取或执行任何业务任务；不能仅凭无commands宣称不领任务。原业务Journal有未确认工作时拒绝改作此模式并保留原账，不重执行或清除。原任务启动只附允许标志；一次report不附或宣称持续能力。完整中英文、44px触达、窄屏及键盘操作沿原设计系统。

## 分工与必要验证

控制代理负责唯一DTO／校验器、原维护cap透传、新表及当前授权／普通与Pool接口、互斥与原回执、定向领域检查。执行代理负责原唯一维护owner中的检测、CLI明确允许、九目录过滤、隔离进程与真实source分支、Journal／停止证明和定向恢复检查。交互代理负责远程检测组件、严格读取器、原Guide／Quick／Workspace接线、双语和定向交互。主集成负责公共合同、API生成、真实联调、设计文档、精简证据与阶段Git。

必须验证未登记工具真实被检测但任务0、丢ACK原操作不重probe、过期／旧fence／撤权、Pool满槽维护、unknown原停止回执闭合、默认无能力及明确允许、原工具／节点准确交接。只重受影响检查，不重复SDK／厂商／完整测试链。真实厂商与模型、物理多机、跨系统／数据库、规模和生产按完整矩阵分别验收。

后续已确认缺口保持：Pool机器免令牌配对与多项目私有配置接续、OpenCode SDK原生新文件准备、WorkBuddy可信代码交付与原同提交质量链，以及完整平台其他待办；本次检测不能替代它们。
