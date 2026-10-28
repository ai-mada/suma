# Octop 与统一模型执行交互规范

状态：suma 1.0.0／T-MODEL-118 当前交互设计。责任：工作台、设计系统与工具集成。关联：[架构](../architecture/octop-execution.md)、[统一网关](../architecture/model-gateway.md)、[执行合同](../../contracts/octop-harness-acp.md)、[本轮验证](../quality/model-gateway-verification.md)。复用已有 Field、details、工具页及设计令牌，不增页面或重复模型控制台；本文不预称实际浏览器通过。

## 用户路径与渐进披露

执行工具 → 接入开发工具 → **Octop Harness / ACP**。表单只显示管理员绝对 Python、`.pyz`、同目录 manifest 与固定 check IDs；必要输入完整才生成可复制本地配置。没有提供方、URL、API key、环境凭据或固定模型参数表单。

后续 probe、登记和派发说明置于折叠区。运行时文件由 Runner 管理员核实，浏览器不假称能验证远程文件。复制反馈绑定当前确切文本，失败保留草稿；语言切换保留输入，不触发安装、探测、连接或派发。

原生编码与 Octop 的模型操作共用一个面板：选择当前工具真实部署版本、最大请求次数、attempt 总输出额度，按需展开预算与依据。边界取本次选项服务的 `parameter_limits`，不以静态 UI 常量授予能力。未定价显示未定价；空目录、无权限、失效档案和读取错误分别表达，不自动选其它模型。

## 计划与运行共用事实卡

任务预演、专家逐项计划、原生修复及修订复用 `ExecutionModelFacts`；各工具保持实际支持范围。已保存计划展示原 Binding、参数、Rate 与预留，不从当前可变工具档案补历史依据。运行详情核对发布 definition、run、节点、attempt／fence 和工具摘要，再展示原模型快照。

| 常显内容 | 按需展开 |
| --- | --- |
| 固定模型、主要参数、未定价／原费率 | 完整绑定、部署／参数摘要、预算和预留 |
| 模型请求已核对／待核对／依据不一致／缺回执 | 每次请求状态、返回模型／请求 ID、摘要、许可及五类计数 |
| Go 网关是否关闭、用量覆盖、返回模型变化提示 | 模型请求计数来源与缺失计数说明 |
| Octop 协议停止、Go 进程停止、取消意图 | runtime SDK 版本、原 session、真实时间与停止边界 |

请求 wire 索引从 0 开始，UI 显示第 1 次；不改变原摘要或机器标识。未知计数显示未上报，不补零。真正零调用网关关闭可表明零模型消耗，不能呈现成已完成开发。取消仍保留已知计数。

请求核对与账本结算使用不同措辞。事实卡没有读取账本时仅显示网关状态，并提供已有用量／预算账本入口；不显示自算 settled、hold 已释放或绝对供应商费用保证，也不新增重复轮询。模型／代码／测试／验收状态不合并为一个误导性百分比。

## 原操作与异常状态

模型目录读取绑定当前项目、工具版本和摘要，切换 scope 后的迟到响应不能进入新表单。预演／Start 回执须匹配原项目／任务／工具及唯一生产节点的完整 Binding；异构或伪造回执保留原操作，不能清空后自动再派发。

未登记、不可用、缺能力、配置变化、运行中、缺回执、停止未确认、unknown 和结果失败分别显示。ACP end_turn、取消请求或退出码不等于进程树停止，网关关闭也不等于独立测试／人工验收。恢复沿现有原操作诊断和明确新计划，不增加一键绕过门禁。

## 语言、可访问性与布局

产品文案使用成对 zh-CN／en-US 域目录，可继续增加语言；日期、金额、Token 与数量使用统一 locale 格式。模型名称、端点来源、session、摘要、错误原码和用户正文不翻译。结构化诊断提供本地化解释并保留原值。

表单使用真实 label／input／select，数字字段包含动态 min／max／step；错误和状态沿原 aria 及焦点行为。details 支持键盘、默认收起，代码预览可聚焦和独立滚动，窄屏不横向溢出。继承深浅主题及减少动效，不新增强制动画。受影响交互、语言与实际布局的验证范围由本轮报告给出，组件检查不替代浏览器验收。

## English interaction summary

Octop setup contains runtime paths and check IDs only. Native coding and Octop share one model/parameter/budget panel with server-provided limits and explicit unpriced/empty/error states. The same compact facts mount is used in task, expert and supported repair plans; execution uses the frozen original binding, not mutable inventory.

Progressively disclosed request facts show actual returned model/request IDs, counters and missing usage. Gateway reconciliation, ledger settlement, ACP stop, process stop, cancellation and business acceptance remain distinct. No extra polling, automatic model fallback, implicit dispatch or inferred hold release is introduced. All controls and state copy support Chinese and English, keyboard access and responsive layouts; actual browser evidence remains in the linked report.
