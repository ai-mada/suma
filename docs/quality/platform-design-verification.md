# 全平台设计与交互样稿验证

阶段：T-PLATFORM-DESIGN-134；日期：2026-10-29；源码核查基线：`3e09a51`；产品：1.0.0。

提交整合基线为 `f8efb23`，包含 `e8f441e`：保留T-EXPERIENCE-133全部代码、合同、文档与状态，仅合并文档索引冲突；保留随后新增的正式架构HTML。补充核对项目焦点及四类客户端配置导出的现状，不把本样稿验证当作T133生产界面验收。

## 交付与边界

- 扩充[统一体验设计](../design/unified-experience.md)：当前事实、6＋6入口映射、角色旅程、11项业务闭环、可视化、草稿与恢复。
- 新增[能力编排与部署架构](../architecture/unified-capability-orchestration.md)：八类状态所有权、固定任务依据、四层适配声明、配置继承、确定性选择、调度背压、用量与未知效果、四种部署形态和8项故障验收。
- 新增[工具与Harness矩阵](../integrations/development-tool-matrix.md)：11个产品／入口行，区分官方能力、当前源码与拟接入路径；逐项保留官方来源与限制。
- 实现[可点击工作台样稿](../design/prototypes/enterprise-workbench.html)：八个演示工作区，明暗主题、移动布局、详情、筛选、共享排期草稿、AI建议差异、接入向导、策略审阅与质量追踪。

样稿为独立HTML／CSS／JavaScript，数据全部合成，仅使用页面内存，无后端、外部依赖、模型调用或业务写入。演示导航不替代生产导航；刷新恢复示例。该阶段没有修改生产前后端、领域合同、依赖或数据声明，也不表示新增工具适配完成。完整范围仍在[唯一实施计划](../delivery/implementation-plan.md)，58／34／92保持不变。

## 实际检查

环境：Windows，PowerShell，Node 22.16.0，Chrome 156.0.8078.48，已安装Playwright；浏览器只访问本地 `file://` 样稿。

| 检查 | 实际结果 |
| --- | --- |
| JavaScript语法 | 样稿及浏览器脚本通过 `node --check` |
| 浏览器交互 | 44项断言通过，浏览器异常0，HTTP／HTTPS外发0 |
| 桌面 | 1440×1000，八个入口无文档横向溢出；排期深色与交付台浅色截图人工查看 |
| 移动 | 390×844，八个入口无文档横向溢出；项目默认卡片，复杂表格局部横向滚动；工具页深色与全屏详情浅色截图人工查看 |
| 键盘 | 详情焦点圈定及返回；移动导航隔离背景、Escape归还焦点；章节／视图重绘保留触发按钮焦点 |
| 草稿与范围 | 项目筛选返回、日期次序拒绝、已验收日期只读、排期三视图共享、差异与撤销、正文跨章节／路由保留 |
| 智能与接入交互 | 建议引用真实当前草稿，假设确认后采用且不重复追加；接入方向与未探测状态明确；策略先预览后采用；派发演示不声称创建运行 |
| 文档与差异 | 8份变更文档没有新增缺失链接；全仓库检查exit1，15处旧缓存引用与3e09a51逐项比对一致；323个API路径／3061个schema引用有效；`git diff --check`通过 |

可复用脚本：[verify-enterprise-prototype.cjs](../../scripts/verify-enterprise-prototype.cjs)。在仓库根目录执行，支持进程环境 `SUMA_PLAYWRIGHT_MODULE` 指定已有Playwright模块，`SUMA_CHROME_PATH` 指定浏览器；省略时使用标准模块和Playwright浏览器。不安装任何依赖。

本次命令（环境覆盖只作用于当前进程）：

```powershell
$env:TEMP = Join-Path (Get-Location) '.cache/platform-design-134/tmp'
$env:TMP = $env:TEMP
$env:SUMA_PLAYWRIGHT_MODULE = 'C:/Users/Administrator/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright'
$env:SUMA_CHROME_PATH = 'C:/Program Files/Google/Chrome/Application/chrome.exe'
node scripts/verify-enterprise-prototype.cjs
Get-Content docs/design/prototypes/enterprise-workbench.js -Raw | node --check
Get-Content scripts/verify-enterprise-prototype.cjs -Raw | node --check
Get-Content scripts/check-docs.mjs -Raw | node --input-type=module
git diff --check
```

受限进程直接以文件路径启动Node出现 `EPERM lstat C:\Users\Administrator`，语法检查改用标准输入；浏览器验证经已授权的工具审核使用本机依赖执行。初版测试定位到重复关闭按钮及被详情隔离的背景按钮，修正定位后重验；这些测试脚本失败未记为界面通过。

只读审查修复了三项交互问题：AI差异未引用当前正文、视图重绘后焦点丢失、移动导航退出后焦点未归还。另校正周容量与计划日期计算、跨月日期标签、移动卡片及局部表格阅读。最终44项结果对应摘要中的样稿和脚本SHA-256。

## 证据与仍待验证

原始结果与4张截图在项目既有忽略证据目录 `dist/quality/platform-design-134/`；源码、脚本、报告和紧凑摘要纳入提交，避免提交浏览器配置、日志或依赖缓存。临时浏览器文件仅使用 `.cache/platform-design-134/tmp/`。没有覆盖活动产品预览或修改数据库。

本阶段只验证中文样稿与代表性明暗／桌面／390px交互；不是完整WCAG符合性、英文、屏幕阅读器、触屏真机、大数据或全部异常路径验收。拖动排期、实时协作、持久草稿、稳定验收项服务端合同、真实派发／接入／发布仍待生产实现。

由于没有生产源码、公共合同或依赖变更，本阶段未重复Go、生产前端全套或构建。前序T-EXPERIENCE-131的全量Go失败仍以[原报告](unified-experience-verification.md)为准，本阶段不使其变绿。真实厂商、物理多机、跨数据库、离线执行、移动、商业计费和生产验收分别保留。

初查16处缺失引用；随后纳入f8efb23补齐的正式架构HTML，最终剩余15处历史failure-diagnosis／Octop忽略缓存证据引用。已核对基线中的同一引用集合，未复制、补造或删除原证据。
