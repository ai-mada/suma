# 失败分诊与修复草稿：限定验证报告

阶段：T-P4-030；产品版本：1.0.0；整理日期：2026-10-27。
状态：本阶段所列功能已完成定向检查、同机实际链路、中英界面、正常闭合及主预览更新。完整 P0–P7 继续实施。
依据：[领域合同](../../contracts/failure-diagnosis.md)、[CLI 合同](../../contracts/failure-diagnosis-cli.md)、[机器摘要](failure-diagnosis-summary.json)。

## 范围与边界

从真实持久失败节点固定 run、node、attempt、issue、版本及证据摘要，经专属只读模型能力产生诊断。
观察、假设和模型原件保持原文；人工只编辑修复建议，明确采用后复用原 issue-intake 创建或打开当前项目需求草稿。
采用不代表修复成功、正式验收、自动重试或 Git 集成。未知／reconciling 仍使用原运行核对，不能据此调用诊断模型。

定向检查使用本地协议／HTTP／模型响应夹具。当前实际流程是同一 Windows 主机的真实 Control、Runner、DAG 与账本，
加一次自有 loopback 固定模型响应；没有据此宣称真实厂商模型、跨物理机器或生产验收。
记录源码检查、本机协议流程与真实厂商／跨物理机器／生产验收的独立边界。

## 已通过的必要检查

顶层组、子例、先行单项和修复复验分别记录，不相加为阶段总数。以下收据路径均相对仓库；完整 SHA256 见机器摘要。

| 切面 | 实际结果与范围 | 收据／定位 |
|---|---|---|
| 平台领域 | 7 个不同顶层组：真实失败与采用回链、并发单一草稿／回滚、当前权限、已有需求、领取前后来源漂移、严格输出与尾部证据、未知／异常原件 | `.cache/e2e/failure-diagnosis-stage-030/domain/verification.local.json` |
| Go Runtime／受影响能力 | 15 顶层、21 pass 事件，exit 0；包含 FailureDiagnosis 及原 Planning／KnowledgeExtraction 受影响组；两包 vet exit 0 | `runtime-checks/verification.local.json`；`.cache/mcp019-deps/20261026T163948171Z.*`、`164202306Z.*` |
| 禁止工具请求 | 修复后仅受影响 stream 1 顶层／1 pass，exit 0；只读诊断不能接纳工具活动 | `.cache/mcp019-deps/20261026T164133159Z.*` |
| 原生 TypeScript | 4 项通过，Node tests 与类型检查 exit 0；单次请求、专属 purpose、无工具、完整结果和 usage | `runtime-checks/ts-command.local.json` |
| CLI | 1 顶层／6 pass，exit 0；精确 JSON／作用域／确认／原 key，读取不自动写入，不伪造浏览器权限 | `.cache/e2e/failure-diagnosis-stage-030/cli/verification.local.json` |
| Shared Pool | 1 顶层／1 pass，exit 0；租户隔离、来源漂移、原回执重放、账本只结算一次及失效采用拒绝 | `.cache/mcp019-deps/20261026T170957717Z.*` |
| Root HTTP／定价／本地化 | 初次联查中定价、原文保护与诊断覆盖 3 顶层已通过；HTTP 修复后仅重跑 1 顶层，exit 0 | `.cache/mcp019-deps/20261026T165252209Z.*`、`165447799Z.*` |
| Root 元数据本地化 | 仅复验原文／人工原因保护的 1 顶层，exit 0；canonical 内容不替换，另附显示元数据 | `.cache/mcp019-deps/20261026T170314695Z.*` |

表内 `runtime-checks/` 是 `.cache/e2e/failure-diagnosis-stage-030/` 的子目录。
Pool 测试中的 40 tokens／57 micros 是夹具账本断言，不能替代下述实际流程的最终账本核对。

前端结果来自代理工具输出，**没有独立 raw 日志文件，不补造日志路径**：

- `FailureDiagnosisPanel.test.tsx`：00:45:11（+08:00）原 5 项通过。覆盖默认折叠零读取、身份／项目／来源隔离、full／tail 告知、专属模型确认、原操作未知恢复、修复字段 CAS、已有需求与来源漂移。
- `FailureDiagnosisLauncher.test.tsx`：00:55:53 原 2 项通过。只在明确展开后读取真实当前 Run.nodes，准确选择失败节点；未知只核对原运行，切换范围中止旧请求。
- Panel 本地化新增单项：01:01:20 仅运行 1 项通过，其余 5 项跳过。CN→EN 不重新 GET，缓存机器诊断按元数据重显；恰好等于模板的人工原因保持原文。
- Root 工具设置受影响 2 项通过；最终 `tsc --noEmit` session `24416` exit 0；i18n 4090 对检查通过。这里依据主集成工具结果，未虚构独立收据。

## 保留的中间失败

- 领域初次组合命令 `164254227Z` exit 1：严格输出组已过，实际采用暴露旧 `stringsAt` 不接收 `[]string`。
  显式转换为 `[]any` 后，只复验失败的实际 DAG／采用组 `164331322Z` exit 0；其后五个边界组 `164451241Z` exit 0。
- Runtime 禁工具请求先行 RED `163619915Z` exit 1，修复后上述 stream 单组 GREEN；没有把缺少完整终态当作诊断成功。
- CLI 先行帮助／确认单项 `164901083Z` exit 1，随后本域 1 顶层／6 pass 的 `165037615Z` exit 0；失败收据保留。
- Root HTTP 初次 `165252209Z` 中创建分析返回 400（期望 201）。严格数字接线补齐 `UseNumber` 后，仅 HTTP 组 `165447799Z` 复验通过，未改写初次联查为全部绿色。
- Pool 先行 `170902141Z` 编译失败，`170926182Z` 的测试查询引用不存在的 `run_id` 列；修正测试后 `170957717Z` 通过。两次失败不计通过。
- 组件缺文件阶段的预期 RED 与后续通过分开保存于工具输出；未为不存在的前端 raw 文件补造记录。
- Root 中间类型检查 `27735` 报 GET 辅助函数参数不符，修正为原作用域 transport 的正式调用方式后 `74843` 通过；
  随后为选中需求的采用来源补读取，最终类型检查 `24416` 通过。没有把中间构建充作最终网页。

## 实际流程与最终界面

原运行 `bffacdf91e492fc8886b5e3d78cf683c` 的管理员固定命令真实退出 1，形成真实持久问题。
这是一条故意失败的本地协议夹具，不将其宣称为产品缺陷。界面选择准确 `fail/attempt1`，展示五段实际来源范围，
确认专属模型和预算后，仅发起一次只读分析 `fb963b9d6b1a3528087b6ce924f49807`。
诊断运行 `diagnosis-fb963b9d6b1a3528087b6ce924f49807` 成功；模型原件、观察和假设单独保留。

本地测试 owner 在界面修订修复标题和说明，保存后再明确采用，最终分析 v4/adopted，
生成 `223291c900a31d06ef3d511b1da149ec`，需求 v1/draft，验收条目完整。
独立只读核对证明：人工稿与原模型 repair 不同；原始输出仍为
`e5dad345023168b72ec8682c21ba3190a1a9aa8f6908f778081a0a2a82f0e745`，
固定来源仍为 `b9e1c9233e16c501adcbd1fdf0122780696cf8d6f659cf01b22b9069d740cfa2`。
只有一份正式结果和一份采用关系；原任务未验收，原运行仍 failed，问题未自动关闭。

实际原生 bundle 经真实 Go Runner 请求自有回环模型一次，23 输入／17 输出，合计 40 Token，
固定费率结算估算 57 CNY 微元、余额预留释放，账本 `293e8b538c571f3d685e2431e2843d5a`。
这不是厂商推理效果或真实商业账单。模型线程、Runner 和 Control 均正常关闭，两个进程退出 0；
没有第二次模型请求、没有新增 Agent Token，源六个正式文件与源辅助文件保持，原业务行保持，数据库完整性 ok。

实际界面证明英文切换保留中文人工标题／理由、模型原件和准确运行号；问题详情从持久读取直接打开同一修复需求，
展开入口只列真实当前失败节点，不自动发起分析。首次需求详情沿列表数据渲染，漏载独立采用证明；
修正为仅对选中需求读取一次正式详情，不对列表做 N+1。最终需求能回到同一原分诊、失败节点和尝试。

宽屏下展开分诊时加宽详情区，保留流程图；窄屏继续沿原可操作列表。最终 `web-release` 的冷启动只读回读
验证了采用来源、深链、中英文和刷新恢复，没有 Runner、模型或重新登录；原分析、原运行、需求和采用关系完全保持，
回读 Control 正常退出 0。原验证网页与未采用的中间构建保存在忽略目录，没有覆盖活动预览。

| 最终制品 | 摘要 |
|---|---|
| Go 输入 | 391 项；`14b753648ad1199316bbb5cb45343b1a2684247ece30af6db65c5b0660ff4035` |
| Control | `89c7307e9a7a20edb55c60139380e37764a771b4ee483d6b8bb77ff364733e1c` |
| Runner | `a653b7e6039cf8854e92acf9879807b99ae0b150c83916a21d8e70464b364e23` |
| CLI | `4dfca049c9fa3552e27f4aab0ef2242fee7c7244e634cd715afe785baad49419` |
| 原生 bundle | `5ab81bdf05e461b73bcc50b32dec92f477b24da288cbf30d27f41f716a5c30d5`，沿用官方 MCP 客户端 2.4.0 |
| Web | 131 文件；`web-release/index.html`：`10130f26ac03a9af9c337a921665213e2de5115ba7ccc48145f18c4f9387431c` |

实际调用先用 `web-final`，最后两项前端修正经类型检查、独立构建及同一已采用数据冷启动回读，未重复模型调用。
Vite 的大 chunk 警告仍保留；本阶段不以构建成功宣称全平台性能目标通过。

## 主预览与剩余范围

主预览 `http://pool.localhost:4324/` 已前向更新到 030／1.0.0，Control `252836`、holder `137148` 正常模式。
177 张原表的完整 schema 与全部行保持一致，仅加三张空表和一个索引至 180；
18 条 authority、0 条 Grant、原登录／任务／1 failed 与2 succeeded 运行保持。
旧 029 Control 与 holder 按原停止协议正常结束，保留 WAL 感知备份、原二进制和网页；维护写入口返回 423。
没有重新签发凭据、自动授权或主预览模型调用。原始日志、截图、数据库、令牌和二进制只保留 `.cache/`，不提交 Git。

实际来源位于本地忽略目录 `.cache/e2e/failure-diagnosis-stage-030/`：`browser/verification.local.json`、
`actual/closed.local.json` 和 `primary-upgrade/final-verification.local.json`；可分发摘要见同目录 JSON，记录原始证据摘要。
真实厂商账号／模型／账单、跨物理机器部署、自动修复及独立复测、完整平台回归、生产验收继续实施。
多候选受测集成基线与后继开发源码交接保留为下一研发纵向，不能用业务依赖验收代替代码交付。
