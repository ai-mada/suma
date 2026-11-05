# Codex app-server 执行合同

状态：T-P4-041／1.0.0，限定单任务本地实际执行已验证。Owner：控制面、执行器、工作台。关联需求9、12、14、18、21、22；完整P0–P7与需求1–22保持实施。权威类型为[Codex合同](../internal/codexcontract/codex.go)，不扩展或冒用[原生模型合同](model-execution.md)。

## 配置、发现与选择

T-CODEX-QUICK-CONNECT-175已提供[桌面原生快速连接](codex-quick-connect.md)，通过当前用户明确配置和实际目录生成下述固定配置，免手写命令文件；独立Runner仍可由管理员采用相同typed配置。

新增精确工具协议 `adapter=codex / protocol=codex-app-server-v1`，JSONL适配器保持另一当前执行协议的独立含义。Runner本地管理员声明 `codex_app_server` 的 `provider_id`、完整 `base_url`、可选 `api_key_env_ref`，以及Windows上必须明确选择的 `windows_sandbox`（unelevated／elevated／mxc，无静默降级）；只允许固定 `app-server --listen stdio://` 及 `input=codex-app-server`。工作流不能设置本地provider、凭证、环境、进程参数或工作目录。

每个探测／attempt新建独立CODEX_HOME，生成固定Responses协议配置；不复制用户登录文件。完整本地配置及生成策略版本形成摘要，密钥值不进入摘要、库存、计划或日志。项目配置不获得修改执行权限的来源地位。真实初始化和model/list通过后，当前Runner才报告公开 `codex_catalog`；版本／帮助检查不传凭证，不创建thread或turn。

`GET /model-execution/options` 复用当前项目、工具ID、工具版本、摘要和库存授权；`supported` 仍仅表示原生合同，`codex_supported`、`codex_catalog`、`codex_reason` 表达独立能力。model/list目录不证明厂商账号授权、端点健康、实际提供方模型或价格。

单任务 `POST /task-dispatch-plans` 使用 `codex_execution_selection`：目录摘要、实际支持的 `model`、`reasoning_effort`，以及可选正整数 `token_notice_limit`。Token阈值仅提醒；不接受原生Token／费用上限或其他私有字段。公开 `plan.codex_execution` 和选定agent节点保存相同固定Binding，自动重试次数为0。原生绑定、旧模板的provider／环境／权限参数、硬限额字段与Codex绑定互斥；共用合同门禁，不能到启动后才默默忽略。

## 当前权限与预算

Preview、Start、Claim分别核对当前工具、目录／本地配置摘要和原计划依据。organization／project的 `observe` 限额仍是观察阈值；任何 `enforce` 策略均明确拒绝此能力不足的工具。不能把预算策略暗改为软阈值，也不靠缺少费用数据声称免费。

`BudgetMode=observe_only`、`IdentityEvidence=configuration_ack_only` 固定在Binding。模型、推理强度和目录变化需重新预览。专业模型授权／多供应商费率、自动最优模型选择与其他CLI的能力不能由本协议推定。

## 执行、回执与恢复

Runner沿现有独立Git工作树、进程树控制、候选采集、持久输出和结果回写执行。initialize核对新home；重新model/list核对原目录；thread/start核对确切cwd、模型、推理强度、provider配置标签、固定`on-request`审批和限于本工作区且无网络的sandbox确认，之后才发送单次turn/start。当前审批策略明确进入配置摘要版本3及真实ACK；命令／文件的一次人工请求、异步写出和持久观察以[统一工具确认合同](codex-human-requests.md)为准，不提供会话或永久授权。

请求ID仅关联RPC响应，不作为持久幂等证明。丢失thread／turn开始回执、未支持的权限／认证申请、未知进程效果或错误身份均不自动重发或resume。取消发送原thread／turn的interrupt，收到ACK只证明请求被接收；等待原终态并核验进程树停止，平台状态仍由原DAG／租约决定。

用量只读取原thread／turn最新total快照，不把重复通知累加，不把last误作整次总量。计数必须为安全整数并单调，已知小计保留，覆盖标记partial／unreported；原attempt账本保留Codex配置意图，通用厂商Model、EndpointOrigin和Rate不被填成配置标签，费用保持未定价。越过本次Token提醒阈值形成一次事实，不把通知当作强制停止保证。

浏览器仍用单任务预览／确认。模型、推理强度和提醒三个输入复用现有设计系统，中英切换保留草稿。坏2xx保留原key；Start清除原操作前只读核对原计划预留run及派生workflow，读取失败保持待核对。团队／冲突修复入口独立选择、自动resume和原生远程文件broker不在此协议的已实现断言内，仍按完整计划继续推进。

[设计与AI SOP](../docs/integrations/codex-app-server.md)。实际CLI、源码检查、协议夹具与生产验收必须分别记录。

Windows当前CLI会将cwd隐含写根与additional roots分别投影；存在runtimeWorkspaceRoots时须恰好等于本attempt工作区，附加roots只能为空或重复该根。原始只读ACK不能被误当作可写；额外／错误根、网络开启和未明确的Windows实现均拒绝。此处确认配置，不能替代OS隔离／网络阻断的独立验收。

[PRD](../docs/product/codex-model-execution.md)、[交互](../docs/design/codex-model-execution.md)、[限定验证](../docs/quality/codex-app-server-verification.md)。

取消在等待initialize／目录／thread ACK时先记录停止意图；迟到ACK和持久化进度失败不允许提交后续首次RPC。已提交RPC仍需按原回执及进程停止核对，不能把发出取消等价为未执行。

## 开发候选

拥有原候选传输能力的 `agent` 节点，在当前 Codex 绑定、完成回执和权威进程树停止均确认后，可由 Runner 封存实际工作区改动。该路径要求原基线的独立工作树和受保护的 Git 读取，沿现有父封存检查点保存候选。如果工作区已经有提交，继续沿原 clean HEAD 传输规则核验。候选绑定原执行身份，后续独立测试导入该提交。任务验收依据当前测试及复核权限。

缓存写入计数是可选事实。缺失时保留此前的有效小计，明确的无效值或下降仍进入核对。运行详情从原工作流版本读取固定配置，CLI 回执与当前尝试及绑定一致后才展示关联确认。 [候选交付与节点设置](../docs/integrations/codex-candidate-delivery.md)
