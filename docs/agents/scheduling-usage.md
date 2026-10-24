# 原生用量、过程额度与领取恢复

状态：T-P4-005 Runtime 实现、本机夹具及域回归完成；完整仓库、最终交付包和 GUI 绑定由主集成记录。负责人：Runtime代理／主集成。合同：[调度内核](../../contracts/scheduling-kernel.md)。实现：`internal/toolcontract`、`internal/runner/tool_stream.go`、`claim_checkpoint.go`、`packages/agent-runtime/src/index.ts`。验收结果：[阶段验证摘要](../quality/scheduling-verification.md)、[机器可读摘要](../quality/scheduling-summary.json)。

## 用量事实

ToolUsage五个计数均为可选整数指针：不存在的字段保持未知，明确报告0则输出0。每个计数与累加结果限于0..9007199254740991，拒绝负数、小数、字符串、布尔、溢出、空对象、未知字段、重复字段及显式null计数。

ToolEvent.usage_state在用量事件或携带usage时必须存在，ToolEvidence.usage_state始终存在：

| 状态 | 含义 |
|---|---|
| complete | 每个模型请求的input/output均已报告，出现过的可选拆分也覆盖所有请求；Codex单turn三基础计数完整 |
| partial | 保留部分小计；有步骤缺用量、部分基础计数、可选拆分只覆盖部分步骤，或后续请求尚未结束 |
| unreported | 没有合法已报告计数，usage省略；不能解释为消耗0 |

外层usage:null解码为未知，Evidence及kind=usage事件仅与unreported一致时允许；普通无用量事件的null与省略同义，可不带usage_state。Go写出时省略nil。complete／partial不能携带null，单个计数null始终拒绝。部分小计不能用于完整结算，也不通过旧SSE补出缺失Evidence用量。

原生响应将prompt_tokens／completion_tokens映射为input／output；标准prompt_tokens_details.cached_tokens与completion_tokens_details.reasoning_tokens映射对应拆分。显式归一化拆分字段也受相同校验；同一计数两个来源矛盾时拒绝。存在的total_tokens也检查安全整数，但不重复计入小计。usage缺失、null或空对象保持未报告，不生成占位零。

模型响应在2 MiB既有限制内，利用Node22的JSON primitive source context保留已知计数字面量原文，再检查十进制整数字面量与安全范围；避免9007199254740991.1被JSON.parse先舍入成整数。其它JSON值及工具参数保持原普通类型。运行时缺原文能力时，对已报告计数明确报lossless_usage_unavailable；没有计数仍为unreported并保留allowance，不降级成零。仓库要求Node>=22，本机实际验证22.16.0。

Codex保持已核实的三基础字段必需／reasoning与cache-write可选规则。本轮没有执行真实厂商模型，不改变此前只读Codex帮助探测的验收范围。

## 请求覆盖与过程额度

每次模型HTTP前先预留输出allowance，发送不含prompt、URL或凭证的request_started(step,output_allowance)。每个请求结束恰好发送一个usage(step,usage_state,usage_reported,仅已报告计数)；HTTP异常或中止发unreported后退出。硬终止可能只有request_started，Runner因此将此前小计标partial。

Runner校验step从0递增、每次一个待结算请求及一个usage；拒绝跳号、重复、错配、未结算时的工具活动及仍挂起请求的completed。公开事实复用activity_started（status为model_request）及usage，不保存原始模型请求。明确零与部分小计经过Journal和重放保留；指针字段在Journal入口和读取时分别复制，调用方不能修改已持久事实。

默认max_steps=8、max_tokens=8192，上限分别16／32768。显式0、负数、小数、错型或越界值在任何目录／模型HTTP前拒绝，不静默替换为默认值。

每次allowance取当前剩余输出额度与4096的较小值，请求前即保留。合法实际output已知时只消耗真实输出，释放本次未用额度；output未知时保守保留全部allowance。input已知而output缺失仍保留全部输出额度。报告超过本次allowance时先保存真实用量，再拒绝本轮工具动作及后续请求。例如未知5000额度最多发4096与904两次请求，不能靠遗漏usage继续请求。

这是原生进程输出准入约束，不包含输入费用、模型定价、外部CLI私有调用、共享模型速率或厂商最终账单。完整网关、请求许可、全局／租户／项目预算和结算仍按完整平台目标继续实施。

## 明确失败分类

Runner结果携带可选failure_class；缺失默认不自动重试。仅原生模型HTTP明确返回429拒绝、对应步骤unreported且协议结果确定时归一化为transient。错误文字包含429、普通网络异常、其它控制面HTTP429、取消或未知结果均不据此分类。控制面还须检查节点幂等、当前预算、次数与退避；此分类不直接派发重试。

## 领取的落盘顺序

Worker先在私有claim.json记录稳定操作标识、控制地址摘要、项目、worker与原能力集合，再请求控制面。ACK丢失或重启使用原标识、原scope及能力指纹恢复；不得换控制服务、项目或worker重放未完成请求。

收到响应先持久记录，再按active状态写执行inbox，最后记录领取完成，之后才启动进程。closed／reconciling不创建执行inbox、不启动；空响应也先持久记录再开始下一次新领取。Assignment包含dispatch_id、dispatch_state与lease_until，状态和身份缺失／非法时拒绝。

| 崩溃位置 | 恢复行为 |
|---|---|
| 请求已保存，尚未收到响应 | 向服务重放原操作，不制造新attempt |
| 响应已保存，尚未写inbox | 仍向服务刷新原dispatch，不按缓存直接启动 |
| inbox已写，尚未启动进程 | 保守报告unknown并核对，不猜测重跑 |
| 结果已保存，ACK丢失 | 先重放原结果操作，再考虑新领取 |

独立库Client.Claim为单次调用创建操作标识，上层不能网络失败后盲调它重试；Worker使用持久化ClaimWithOperation。本轮仍是单项目、串行一个执行槽位的Runner，不因此声明跨项目授权池或分层公平完成。

## 验证与限制

域回归实际完成159个 Go 通过事件（含子测试与进程 helper）、0失败／0跳过，toolcontract／Runner两包通过，相关 vet 退出0。Native20项测试和类型检查通过。源码及过程范围、整仓库最终结果统一见[阶段验证摘要](../quality/scheduling-verification.md)，不能把域事件数视为业务旅程数。

本机实际 Node 子进程与 HTTP 夹具验证五条：两步明确零完整保留；未知5000额度耗尽；5额度报告6后不派发工具；第二次请求到达后中止进程；安全整数边界的小数原文拒绝。中止一例仅证明原生CLI停在第二个request_started；Go协议回归另证明此前小计归一化为partial，不把两者合写成生产控制面运行。

可复现入口为[用量合同](../../internal/toolcontract/usage_presence_test.go)、[Runner协议与进程测试目录](../../internal/runner)、[原生测试](../../packages/agent-runtime/tests/usage.test.ts)。真实RED、中间失败和最终原始输出留在本地 `.cache/e2e/scheduling-stage/` 与 `docs/quality/evidence/scheduling-runtime-protocol/`，不随源码提交，不覆盖旧T-P4-004记录。

平台早期全局锁迁移另见[平台门禁专项](../quality/scheduling-platform-guards.md)。上述验证没有厂商账户、真实模型或付费调用，不代表模型质量、生产权限、跨项目公平、OceanBase／PostgreSQL或真实商业账单已验收。
