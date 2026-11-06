# 原生模型执行 AI SOP

状态：T-P4-036／1.0.0，限定本地实现及验证通过。操作对象为当前已批准的原生 Suma 可写工作区工具；[产品范围](../product/model-execution.md)、[实现边界](../architecture/model-execution.md)和[统一验证证据](../quality/model-execution-verification.md)分别维护。

## 管理员准备

1. 在 Runner 本地模型部署 JSON 允许清单登记准确部署 ID／修订／摘要、模型 ID、上下文窗口、公开源地址及完整 base URL 摘要。通过 `--model-deployments` 显式加载；私有 base URL 和 `api_key_env_ref` 不发布到控制面。凭据使用独立进程环境引用，不复用 Runner／Agent 认证令牌。
2. 本地命令显式启用 `model_execution_protocol=suma-model-execution-v1`，并满足原生 Suma JSONL、agent-jsonl 输入、可写工作区能力。Runner 使用原生 CLI 的 `--model-execution-probe` 核实实际参数能力，再按既有工具库存协议上报公开部署。能力探测不调用模型。
3. 按原工具审批与路由流程启用准确档案，确认当前 worker 库存有效。多个部署只是允许清单；工具目录“可领取”不能替代模型认证、模型调用或业务验收。
4. 需要计价时，沿原 `usage/rates` 合同，以当前权限、工具版本、费率版本和幂等键登记运营方输入／输出费率及准确部署三元组。当前每工具仅一份最新匹配费率供新选择使用；其余部署可保持未知价格。组织／项目 enforce 政策启用前确认币种与报价可用。

## 明确选择、审阅与执行

当前身份及原项目保持不变，全局 CLI 参数放在命令前；使用已经签发的凭据，保存在本次进程的 `SUMA_TOKEN`：

```powershell
suma --control https://suma.example --project PROJECT_ID --language zh-CN models options --tool TOOL_ID --expected-tool-version TOOL_VERSION
```

同合同 API 为 `GET /api/v1/model-execution/options?project_id=...&tool_profile_id=...&expected_tool_version=...`。返回的 scope 必须与当前项目及工具 ID／版本／摘要相同；返回旧响应、切换身份或工具后不复用原选择。该 GET 不调用厂商。

在原任务派发入口明确选中部署，填写 `max_steps`、`max_output_tokens` 和可选 attempt 预算，再创建原 `task-dispatch-plans` 固定预览。普通入口默认 8／4096，上限 16／32768；不得追加未支持的提供方参数、URL 或环境覆盖。检查部署修订／摘要、绑定摘要、输入／输出预留、预算币种和准确运营方费率；未知价格不能当作零费用。

Start 使用原计划 ID／版本与原操作身份，重新核验当前项目、工具、路由和模型资格。领取时组织与项目配额和 attempt 预留在同一事务中处理。Runner 在创建工作区／模型主进程前匹配本地完整部署及凭据引用；任一不符停止，不能自动切换到环境中的 `SUMA_MODEL` 或另一个模型。

## 异常处理

| 观察 | 处置 |
| --- | --- |
| 非原生工具或未声明模型能力 | 保留原工具派发配置；不向外部 CLI 注入独立模型绑定 |
| 库存过期、工具暂停／撤销、部署不再匹配 | 等管理员修正当前资格后重新读取选项及准备计划；不编辑历史计划伪装成原选择 |
| 预算不足或 enforce 缺少匹配报价 | 调整准确预算／运营方费率或策略后重新预览；不自动换价、换币种或换模型 |
| 提供方返回另一模型、别名、缺失身份或越界用量 | 保留原 request／response 摘要和已报告计数，读取原 attempt 核对状态；不把它记为可信完成 |
| 请求失联、停止、部分／未报告用量 | 保留原调用者、计划、run／node／attempt／fence 与幂等键；先读取状态，不重启模型追求一个“成功”回执 |
| 有独立可信用量来源 | 仅在当前允许操作下使用既有 `usage/attempts/{id}/confirm`，提交 expected_version、来源、原因、明确确认及稳定幂等键；计数不能低于已报告事实 |

费用是按原冻结运营方费率计算的估算值；后来更新当前报价不改旧 attempt。模型身份或用量核对不能代替独立代码测试、Review、合并或生产批准。未经可信执行前证明，不能将失败推断为零用量。

## 在终端核对用量证据

将已有的工作台登录会话令牌保存在当前进程的 `SUMA_TOKEN` 中。使用相同项目、操作者及组织，先筛选尝试，再用返回的尝试 `id` 查看证据。

```powershell
suma --control https://suma.example --project PROJECT_ID --expected-user USER_ID --expected-organization ORG_ID --language zh-CN usage attempts --month 2026-11 --state confirmed --limit 20
suma --control https://suma.example --project PROJECT_ID --expected-user USER_ID --expected-organization ORG_ID --language zh-CN usage evidence ATTEMPT_ID --limit 20
```

两条命令各返回一份 JSON。若 `next_cursor` 非空，在原命令后加 `--before NEXT_CURSOR`，保留原项目、尝试及筛选。证据列表保留服务端顺序，可以区分原始结果、后续核对记录和人工确认，缺失的计数保持缺失。命令只读取速码已存档的记录，无需调用厂商。

## 核对厂商用量与账单

1. 在速码打开原运行与计量计费，记录 UTC 调用窗口、模型版本、尝试／fence、厂商返回的完成 ID、输入／输出／缓存计数、币种及冻结费率。先确认原尝试用量是否完整。
2. 在厂商用量页选中同一组织、项目和 UTC 窗口。用量导出按模型及 API 密钥分组时，核对调用次数和计数。费用导出按账期与费用项核对，结合缓存价、服务档位、计费精度和项目内其他流量判断差额。 [OpenAI 用量与费用导出](https://help.openai.com/en/articles/20001072)
3. 有授权且厂商费用已出账时，保留准确的费用依据。项目调用密钥只提供模型访问，OpenAI 组织费用 API 使用单独的管理凭据。月度发票、按项目的厂商费用报表和速码的逐尝试估算分别记录。 [OpenAI 组织费用接口](https://developers.openai.com/api/reference/resources/admin/subresources/organization/subresources/usage/methods/costs)

## 集成证据及外部连接

阶段集成按[架构检查点](../architecture/model-execution.md#实现与集成计划)记录当前合同、真实原生 CLI 请求、用量及异常回执，再在[统一验证文档](../quality/model-execution-verification.md)披露通过项和未验项；本 SOP 不把设计、配置或 HTTP 200 当作实际验收。

模型适配复用现有受限 fetch，工作区传输复用官方 MCP SDK。[OpenAI 实际联调记录](../quality/openai-model-live-verification.md)分别记录项目认证、当前 Runner、厂商调用与待核对预留。厂商账单和物理多机需要独立证据。开发工具可运行 `suma ... developer check` 进行[真实只读 MCP 检查](../integrations/developer-connection-check.md)。远程文件访问参见[出站 broker](../architecture/remote-workspace-broker.md)。
