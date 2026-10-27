# Codex 活动工作区读取验证

状态：**1.0.0，限定本机验证通过**；2026-10-28；负责人：主集成代理与三个并行开发代理。关联：[架构与AI SOP](../architecture/codex-workspace-observation.md)、[交互](../design/codex-workspace-observation.md)、[English](../en/codex-workspace-observation.md)、[精简摘要](codex-workspace-observation-summary.json)。完整P0–P7与需求1–22继续实施。

## 交付与边界

当前 Codex app-server attempt 可以使用平台原有出站通道浏览目录和读取有界文件。共享 `liveWorkspaceRead`持有准确活动根句柄；原生工具组合该模块，保留原有MCP／CAS／检查／封存。Codex没有原生写句柄、MCP、操作账本或写领取。当前库存、原调用者、项目、双边Pool授权、lease／fence与到期正文规则保持。

外部CLI可能并发写入。读取增加文件身份、前后元数据和重复字节检查，变化返回`file_changed`；分页仍固定原SHA。它不等于整个工作树的原子快照。终端事件、上下文取消和进程收尾撤销句柄，不通过历史路径重开。

界面沿用现有工作区布局，按返回的`can_write`控制编辑入口；原生编辑期间阻止切换／刷新目标，保留草稿和原保存身份。库存导入允许准确Codex读取声明，拒绝其写入／原生能力。中英文目录4615对键、自有生产硬编码0。没有新增协议、路由、表、依赖或产品兼容层。

## 必要检查

| 检查 | 实际结果 |
| --- | --- |
| 共享工具合同 | 准确Codex目录及传输声明、原生资格与写入拒绝的定向检查通过，0.120秒 |
| Control | 3个新Codex顶层用例及权限子场景、10个受影响读取／写入回归通过，6.019秒；覆盖metadata、原ACK、过期／旧fence、撤权、库存撤回、私有／Pool准确身份和写拒绝 |
| Runner共享读取与原生回归 | 受影响集合执行87.488秒，唯一失败为新测试未关闭journal导致临时目录文件锁清理失败；补充测试关闭后只复验该项，0.206秒通过，没有重复其余检查或模型旅程 |
| Web | 一次5文件／52项受影响用例、直接TypeScript及i18n检查通过；原生草稿／待确认保存、迟到读取及只读目标分别覆盖 |
| 最终检查 | 受影响四包`go vet`、准确双语诊断用例与生成合同漂移检查通过；Web构建一次45.46秒；五个Windows amd64程序构建一次完成 |

初始RED证明Codex读声明及活句柄缺口；Control开发检查中的两个测试预期分别修正为“新fence可发现”和“撤回Pool凭据服务返回403”。生产资格没有为错误测试放宽。原始失败、最终通过与未执行范围分开保留，不累加为同轮全平台检查。

## 一次真实本机出站与浏览器旅程

`TestCodexWorkspaceActualOutboundReadAndCancellation`使用真实Control／SQLite、真实Runner、独立Git工作树及自有Go可执行协议子进程。子进程仅实现所选stdio消息，不加载厂商CLI、不调用模型、不编辑源码。先真实探测／批准档案、上报库存并派发一项观察夹具运行，再复用同一运行检查界面；114.36秒正常通过。

1. 出站HTTP领取实际目录和文件请求，返回准确`source.txt`字节与SHA；错误预期SHA返回`file_changed`，敏感`.codex/config.toml`请求拒绝。
2. 目标`can_write=false`；保存POST被拒绝。控制面未绑定宿主本地目录，响应没有根路径或凭据。
3. 界面选择实际尝试，读取目录及文本；中英文切换保持事实，刷新恢复准确目标与执行工作区，再次读取成功。没有Edit按钮；两语言提示明确Codex只读。控制台warning/error为0。
4. 同一运行取消后，原观察结果不再返回正文，新读取拒绝；管理员分支保持。保存命令、候选转交记录和评审均为0。Worker及临时服务正常退出，测试临时目录在导出摘要后清理。

没有调用实际Codex、厂商或模型；没有执行正式测试节点、人工验收或部署。原始输出中的`code_candidates`字段实际统计`suma_candidate_transfers`，当前测试已更正输出名为`candidate_transfers`；仅修正标签，没有重跑旅程。未知用量不推断为零，本报告的“模型调用0”来自自有无推理夹具实现，不是厂商usage报表。

原始命令／输出在`.cache/mcp019-deps/`。该次联合回执为`20261027T235120560Z.*`；公开关闭和浏览器证据在`.cache/e2e/codex-workspace-103/gui/`。秘密登录、截图、数据库和原始日志不提交Git。此次没有修改既有用户数据库或活动主预览。

## 当前构建与待办

运行包清单：`dist/runtime/builds/20261027T235405143Z-dd7f8bf6/manifest.json`。452个正式输入前后保持，SHA-256：`d4f02d4d90bd2f15781645f9278730b2035a20fa2c777fdb5c8eadff394cff15`。Web构建仅写本阶段忽略目录，没有覆盖现有预览资产；既有Monaco／ELK大分块提示仍保留性能待验范围。

本机协议进程和浏览器通过，不代表当前安装版Codex实测、厂商身份、物理多机／TLS、生产隔离或跨平台安装。Codex协调保存与冲突修复、远程Git、PTY／LSP／DAP、完整IDE、分布式存储、规模及其余全平台目标仍在[完整计划](../delivery/implementation-plan.md)中。
