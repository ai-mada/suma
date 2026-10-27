# 独立模型选择与计价：原生执行最小纵向设计

状态：T-P4-035设计输入，2026-10-27，负责人：控制面／Runner／原生运行时。完整需求1–22及P0–P7范围继续推进，本文只安排下一条实现路径，不缩减完整平台目标。本文基于本地源码与已安装依赖的只读检查；独立模型绑定、动态选择和新增计价合同尚未实现。035另行修复原生工作区执行限额丢失，不能据此声明模型已可选择或可计价。
关联：[034选择依据](intelligent-capability-selection-notes.md)、[当前只读选项合同](../../contracts/task-dispatch-options.md)、[用量类型](../../internal/platform/usage_accounting_types.go)、[原生运行时](../../packages/agent-runtime/src/index.ts)。本次不下载参考仓库、不安装依赖、不调用模型。

## 决定与最小交付

本设计建议的下一条完整纵向实现为：**用户为原生Suma开发节点选择一个已授权、不可变版本的模型部署与执行参数；明确预览冻结模型及费率；Runner实际使用该配置；逐次用量按冻结费率核算并进入既有配额、结算与未知结果恢复。** 首轮不向无法证明实际模型的外部CLI开放此选择，不声称跨模型质量／速度最优。

工具定义运行时、工作区能力与受限检查命令；模型部署定义提供方连接、模型身份与支持参数；费率定义独立的有效报价。三者分别版本化，通过正式执行绑定关联。模型与费率修改不得悄悄改变已开始attempt，也不能复用旧工具摘要伪装成同一执行定义。

## 已核对的真实代码边界

| 代码证据 | 当前行为及直接影响 |
| --- | --- |
| [toolcontract/types.go](../../internal/toolcontract/types.go)、[validation.go](../../internal/toolcontract/validation.go) | 只有Planning／KnowledgeExtraction／FailureDiagnosis三类只读能力携带固定model和endpoint_origin；可写工具没有独立模型身份、参数或费率绑定 |
| [Runner tools.go](../../internal/runner/tools.go)、[environment.go](../../internal/runner/environment.go) | 工具摘要固定CommandSpec、可执行与运行时入口；EnvFrom只记录环境变量名称，不固定其运行时值。可写路径若从环境转发SUMA_MODEL／BASE_URL，不能据此证明模型身份不变 |
| [cli.ts](../../packages/agent-runtime/src/cli.ts) | 实际RuntimeConfig来自SUMA_MODEL、SUMA_MODEL_BASE_URL、SUMA_MODEL_API_KEY；普通AgentRequest不能独立选择模型 |
| [index.ts](../../packages/agent-runtime/src/index.ts) | 使用受限fetch调用baseURL/chat/completions；请求model取RuntimeConfig，max_completion_tokens取本轮剩余输出额度；当前没有通用temperature／top_p／reasoning参数合同 |
| [usage_accounting_model.go](../../internal/platform/usage_accounting_model.go) | usagePricedModel明确要求启用、Suma、只读、无workspace且恰有一类专用模型能力；不能只放松这一判断便让可写开发工具进入计价 |
| [usage_accounting_hooks.go](../../internal/platform/usage_accounting_hooks.go) | Claim同事务检查／预留组织与项目额度，attempt固定rate快照；当前费率在领取时按工具读取最新版本，不等于用户预览时接受的准确报价 |
| [usage_accounting_store.go](../../internal/platform/usage_accounting_store.go) | 通过big.Int计算输入／输出两档费率，并统一向上取整到微货币；支持USD／CNY，未实现缓存读写分档、按请求固定费或账单对账 |
| [tool_stream.go](../../internal/runner/tool_stream.go) | 已按request_id、step、终态与用量覆盖校验；固定model校验主要用于专用单次只读用途，可写配置尚没有模型绑定回执核对 |

执行限额是当前实际缺陷：原生工作区在[execute.go](../../internal/runner/execute.go)重建stdin时原先只有goal、request_id、workspace_bridge，丢失node的max_steps／max_output_tokens。账务缺省4096，SDK普通／修复缺省分别8192／65536；修复SDK支持128步，但原Go流校验通用上限16。035修复这条准确链路，模型独立选择仍按下文后续实施。

## 可复用的成熟SDK与不能假定的能力

本地[package.json](../../packages/agent-runtime/package.json)与[pnpm-lock.yaml](../../pnpm-lock.yaml)固定官方TypeScript MCP客户端@modelcontextprotocol/client 2.4.0；已安装包声明来源modelcontextprotocol/typescript-sdk。[workspace-client.ts](../../packages/agent-runtime/src/workspace-client.ts)实际使用Client、StreamableHTTPClientTransport、类型化工具目录、AbortSignal和关闭连接；自动重连次数为0，并有额外端点、身份、原始参数字面量与响应大小约束。它解决工作区MCP传输，不提供模型价格、模型选择或模型用量证明。

当前原生运行时没有声明或安装openai、@openai/agents等模型SDK，模型网络层是项目自己的受限fetch。不能把OpenAI兼容JSON接口称为已经使用官方OpenAI SDK，也不能依据名称推断SDK支持独立参数或自动计费。

模型ProviderAdapter应明确抽离。后续接入官方提供方SDK时先固定版本、核对官方包及实际依赖闭包；保留取消、无重试、响应大小、重定向、保密和精确计数验证。当前运行时使用Node22 JSON源字面量识别用量整数，不能换成普通SDK解析后丢失该证据。若SDK不能提供满足要求的原始响应入口，应保留受限传输适配，而不是牺牲记账完整性。此处不声称已验证任何未安装SDK的方法。

## 合同冻结清单

| 新合同 | 不可缺少的字段与约束 |
| --- | --- |
| ModelDeploymentRevision | tenant、稳定部署ID、revision、定义SHA、provider/API协议、明确model ID、endpoint_config_sha256、公开origin、上下文上限、工具调用／完整用量能力、允许参数及范围、批准状态；不能只用origin区分同主机不同API路径 |
| ModelProjectGrant／ModelObservation | 项目授权版本、允许用途、执行节点或池身份、实际本地部署revision/SHA、观测时间和过期规则；与现有工具清单授权分开核验，不向浏览器返回密钥或任意可执行环境 |
| ModelExecutionSelection | 所选部署ID／revision／SHA、封闭参数集、参数SHA、max_steps和总输出上限；预算不是可任意拼接的provider参数 |
| ModelRateRevision | 独立报价ID／版本／SHA、准确部署revision、币种、计费语义版本、各分类单价、价格来源及证据摘要、批准人、有效期；手工配置应显示运营方合同费率，不能冒充厂商账单 |
| ModelExecutionBinding | 固定工具身份、模型选择、准确报价与政策版本、任务／流程／代码依据、输入预留依据和输出上限；进入固定计划、Node、attempt、派发摘要和结果合同 |
| ModelExecutionEvidence | 准确assignment／attempt／fence、请求绑定SHA、实际请求模型与参数SHA、观测到的provider模型ID／请求ID、逐次整数用量及覆盖、原响应证据摘要；不能按未认证的CLI结果文本更换核算主体 |

首轮可选择参数仅开放实际贯通的max_steps与max_output_tokens。后者在Runner映射为SDK的max_tokens，表示一次Agent执行的累计输出额度；每次HTTP调用再从剩余额度计算max_completion_tokens，不能每一步重置成同一个总额。其他采样或推理参数在对应ProviderAdapter完成真实支持前明确拒绝，不能静默忽略。

035原生限额合同：普通步骤默认8、范围1..16，输出1..32768；已有typed冲突修复步骤默认64、范围1..128，输出1..131072；输出缺省统一显式4096。非法类型、null、非整数、0和越界在工作区／MCP／主进程开始前拒绝，保留现有可信执行前失败证明。修复更高限额只复用准确的既有修复授权，不由字符串marker新增权限。

## 精确核算与预算不能混为一谈

已有UsageRateRevision的币种、整数微货币、输入／输出费率、预留值及attempt快照可以复用，但其按tool_profile_id组织的当前费率不能充当独立模型价目表。增加独立模型报价存储与准确报价读取；原历史attempt及原始计数保留，不追溯套用新报价。

首轮应固定一种可完整解释的token计费语义：输入总数分解为普通输入、缓存读与缓存写，必须证明分类互斥且加和不超过输入；reasoning_output是输出子集时不再重复计费。单次没有某类字段不能直接等同该类为0；只有提供方明确计费语义允许时才推导。若当前报价缺少实际发生分类的单价，则不可定价／待核对，不能把缓存默认按普通输入算成“精确成本”。

计费公式对各互斥分类使用整数计数和每百万token的微货币单价，汇总有理分子后在明确结算边界统一向上取整，复用big.Int和安全整数溢出处理。不同币种不自动折算排序；预留、按声明费率核算的金额和厂商账单分别展示。

预算预留必须有可执行依据。现有InputTokenReservation只是配置值，不是SDK已执行的输入上限；多轮消息与工具结果会增加实际输入。最小安全方案先用已验证模型上下文上限×最大模型调用次数作为保守输入预留，结合总输出上限与最高适用分类费率形成上界，并在调用前执行对应硬限制。更细的token估计可后续由成熟且与模型版本匹配的计数器优化，不能在未证实前宣称硬费用保证。

用量缺失、调用取消后结果未知、模型身份冲突或溢出沿既有reconciling及预留保留机制处理；保留原请求，不自动换模型重跑。正式账单对账需要提供方实际用量／账单来源，尚不属于本地HTTP响应计数能够独立证明的范围。

## 实际执行路径与模块改动

1. 管理员登记模型部署revision、项目grant及报价；Runner在独立的本地ModelDeployments允许清单保存端点、凭据引用及实际参数，不把可变模型目录塞进工具CommandSpec冒充同一配置摘要。
2. 复用当前任务工具选项入口，读取该工具与实际路由支持的模型部署选项、可定价状态和限制。选择是明确选择，不制造质量或费用评分；无完整报价仍显示未知。
3. 原TaskDispatch预览按当前权限冻结ModelExecutionBinding与用户接受的准确报价revision；Start再次核验grant、模型与报价有效期。不能在Claim时默默换成“最新报价”。
4. DAG将准确模型绑定交给Runner；Runner核对本地部署ID／revision／SHA及当前工具能力，解析管理员凭据引用，拒绝node任意URL、环境或模型覆盖。参数与预算进入同一准确stdin请求。
5. 原生ProviderAdapter核验绑定，在真实请求中使用所选模型、端点及允许参数；started／request_started／usage／completed保留模型绑定与提供方观测。别名或提供方返回模型身份不符时保留用量并进入核对，不自动换模型或换价。
6. CheckAttemptTx／ReserveAttemptTx按准确模型报价和同一事务锁预留；RecordResultTx将绑定一致的真实用量进入原账本，价格更新不改旧attempt。独立测试、合并、Review与生产门禁继续独立存在。

具体模块：新增internal/modelcontract及internal/platform/model_deployments*／model_rates*；扩展toolcontract的原生模型绑定能力声明、task_dispatch计划与build、DAG绑定／摘要、Runner本地部署观测／execute／tool_stream、原生index.ts／cli.ts的ProviderAdapter、usage_accounting_model/hooks/store/types及同源生成合同。界面只在当前派发弹窗增加简短模型选择和费用依据展开区，管理页复用现有权限、CAS、幂等、分页与中英文设计系统。

## 分阶段实施与必要证据

| 顺序 | 可独立交付的实际工作 | 必要验收，不扩大重复测试 |
| --- | --- | --- |
| 035执行限额修复 | 精确传递node限额，缺省4096，非法输入提前拒绝；修复typed repair的128步／每次1024回执边界 | 真实子进程stdin记录与限额边界；invalid无工作区／MCP／主进程副作用且保留可信启动失败；只跑受影响低层回归 |
| 后续模型执行绑定 | 冻结上述合同，登记两个明确版本部署，任务选择→预览→Start→Runner实际请求使用同一配置 | 记录真实出站请求中的模型、端点摘要与参数；错误版本／撤权／篡改／旧worker回执阻断；此时无完整费率仍显示未知 |
| 后续计价闭环 | 独立报价revision、准确预留、分类计数和账本结算全部接同一attempt | 一个授权本地协议提供方验证两种不同模型／报价，完整与缺失用量、重复回执及价格更新后旧attempt不重算；如需厂商准确验收另用真实账户证据 |

完成判据是选定定义、真实执行请求和核算依据逐项一致，并可解释未知和失败；配置页面、声明字段或HTTP200都不能代替这一证据。
