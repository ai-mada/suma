# 统一模型执行与绑定设计

状态：suma 1.0.0／T-MODEL-118 当前领域设计。责任：模型合同、控制面、DAG、Runner、账务与工作台。完整 HTTP、SSE、请求日志及重启设计见[统一模型网关](model-gateway.md)，字段合同见[model-execution](../../contracts/model-execution.md)。实际检查、协议运行和未验范围由[本轮报告](../quality/model-gateway-verification.md)维护。

## 当前数据合同与初始化

产品仍为 **1.0.0**，唯一持久数据合同为 **`suma-initial-gateway-v1`**，权威声明在 `internal/store/baseline.go`。platform／dag 的完整声明须一致；初始化只读检查后拒绝旧、混合或未登记数据，不迁移、不改写旧标记、不清空用户目录。部署使用单独的新空数据目录／空数据库，保留原实例的业务原件、凭据和历史证据；已声明当前合同的库仍可幂等重启。详见[当前持久数据合同](model-gateway.md#当前持久数据合同)。

## 模型领域与模块责任

| 概念 | 当前职责 |
| --- | --- |
| `ModelDeployment` | Runner 允许列表中的不可变公开模型修订；完整供应商路径与真实密钥留在 Go |
| `ModelSelection` | 当前工具范围内用户明确选择的部署版本、参数和可选 attempt 预算 |
| `ModelBinding` | 计划和唯一生产节点共享的完整快照，含部署／参数摘要、预算、准确 Rate 和预留 |
| `ModelEvidence` | Go 模型网关产生的真实请求身份、状态、计数与关闭证据 |
| 账务 attempt | 组织／项目额度的原子预留、已知用量、整数费用估算、结算与未解 hold |

原生 Suma 编码与 Octop Harness／ACP 共用上述领域。控制面 `model_execution.go` 读取当前授权目录与匹配费率，任务派发冻结 Binding，DAG 和 Runner 核对真实工具／assignment，`internal/modelgateway/` 独占供应商 HTTP 及持久请求准入。TypeScript／Python runtime 只通过 attempt 回环连接使用模型，不拥有供应商密钥或独立账务权威。

## 参数、预留与费用

支持 `max_steps` 和 **attempt 累计** `max_output_tokens`，具体可选上限来自当前 `parameter_limits`，合同最高为 128 次／131072 输出 Token。一次请求不获得完整总额度的新副本，Go 按剩余许可准入；完整用量才能归还未用许可，缺失／partial／unknown 不记零。

输入预留为 `context_window × max_steps`，输出预留为总输出额度，Token 预算覆盖两者。冻结费率按输入／输出两档、整数微货币计算；没有匹配报价时为未定价，不是免费。声明上下文窗口是保守依据，不是精确 tokenizer 或供应商账单硬保证。

普通任务、专家逐项计划、原生受管修复与修订沿现有专用权限使用同一 Binding；计划、费率、工具或需求依据变化须明确重新预演。已开始节点保留其原定义，不自动采用当前模型或当前报价。Octop 不因共享模型链路而获得原生冲突修复用途。

## 原操作、事实与恢复

模型请求先持久准入再上游发送，保留真实 SSE token 流，终态／usage 后缀在权威记录持久后才释放。同 step 同正文仅返可用原 completed 缓存，不二次请求；未知结果和跨重启缺缓存不授权重发。Runner journal 保留原 Binding、attempt／fence、已知计数与未完成事实。

模型响应完整、用量完整、账本结算、代码检查、候选封存、同 SHA 独立测试和人工验收分别判断。取消、代码失败或进程退出不清零消耗；未知保留 hold 并核对原操作。UI 共用模型与网关事实卡，未读取账本时不推断 settled，链接既有用量页即可。

## 范围与进一步设计

只读规划／提炼／分诊／运维 purpose、OpenCode 固定模型及 Codex app-server 继续遵守各自合同，不据此宣称全部工具都共享模型网关。独立模型全局注册、多模型同时报价、自动最低价路由、精确 tokenizer、供应商账单对账与真实大规模分布式验收须另行设计和取证。

## English summary

The product remains 1.0.0. The sole current data contract is `suma-initial-gateway-v1`; incompatible existing directories are rejected without migration or deletion, so a new deployment uses a separate empty directory/database.

Native coding and Octop ACP share immutable deployment selection, model binding, frozen rate and attempt budget. Go alone owns provider credentials, HTTP and the durable request ledger. Parameter limits are server-provided; output is a total attempt allowance and input reservation is a conservative declared context bound times maximum calls. SSE, original-operation recovery and journal restart follow the linked gateway design. Request coverage, usage coverage, ledger settlement and source acceptance remain separate, with actual evidence in the current report.
