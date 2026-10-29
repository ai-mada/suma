# 架构主规格与操作样稿验证

阶段：T-DESIGN-135；日期：2026-10-29；产品：1.0.0。起始提交 `a1c71f2`；集成保留远端 `b1439b3` 的资源日历及 `57589dd` 的人员工时实现与原证据。

## 本轮交付

- [架构主规格](../suma-architecture-design.html)：22节、80项功能、18张图；补齐场景到样稿映射，校正缺少证据的成熟度与完成性表述，明确模型四通道、Harness接续、配置收紧、修复与未知效果边界。
- [统一体验](../design/unified-experience.md)与[能力编排](../architecture/unified-capability-orchestration.md)：职责、公开资料交接、固定模型选择、配置解析与领域接入要求；继续使用原唯一实施计划。
- [可操作工作台](../design/prototypes/enterprise-workbench.html)：十二个工作区，共享导航、搜索、详情与草稿；新增模型／Harness、团队／分工、事件／自动化、架构地图，以及想法撤销、缺陷周期、发布依赖、配置冲突和离线比较。

样稿使用合成数据和本地内存，不连接业务后端、供应商、模型、设备或执行器。规则模拟、停止核对、回归与授权情景不是实际证据；没有新增生产适配器、部署或迁移。完整58项限定实现／34项待实现／92项总范围保持不变。

## 实际验证

环境：Windows、PowerShell、Node 22.16.0、Chrome 156.0.8078.48、已安装Playwright。代表性中文、1440×1000／390×844、明暗主题、减少动态效果。

| 检查 | 结果与边界 |
| --- | --- |
| 原有样稿回归 | 44项通过；复用原脚本，仅支持独立证据目录，原结果中的阶段标识保留为134；本报告记录在135最终来源上重跑 |
| 新增流程与主文档 | 80项通过；模型约束、停止／未知门禁、团队负载与自审阻止、事件去重和失败停止、缺陷回归／审阅／重开、配置CAS示意、离线撤权及解决提案 |
| 安全与草稿 | 用户标题按纯文本展示；整理记录跨页面可撤销；后续有编辑先比较；离线重开恢复原选择；不执行用户输入 |
| 键盘与布局 | 命令面板上下键、重绘保焦、被筛选移除后的回焦、移动导航隔离／返回、主文档页签及图表放大；十二工作区均无整页横向溢出，宽表局部滚动 |
| 实际视觉查看 | 模型浅色、架构深色、团队移动深色、主规格场景映射四张截图已查看；原回归另保留四张代表截图 |
| 浏览器与网络 | 两组共124项断言，异常0、对外HTTP／HTTPS请求0；新组仅访问本机43955静态服务，原组为file入口 |
| 语法与差异 | 样稿模块及两份浏览器脚本语法、主文档内联脚本、`git diff --check`检查；源码和截图摘要记录于[JSON](design-workflows-summary.json) |
| 文档链接 | 无新增缺失文件；全仓库仍有15处历史忽略缓存引用缺失，涉及文件与a1c71f2一致，检查返回1，不记为全绿；最终329个API路径与3109个schema引用有效 |

可重用脚本：[新增流程检查](../../scripts/verify-design-workflows.cjs)、[原有回归](../../scripts/verify-enterprise-prototype.cjs)。先将 `docs/design/prototypes` 与架构主文档通过同一只读本机静态入口提供；新脚本默认43955，可用 `SUMA_DESIGN_PREVIEW_URL` 指定；脚本不启动产品服务或修改数据库。

```powershell
$env:TEMP = Join-Path (Get-Location) '.cache/design-135/tmp'
$env:TMP = $env:TEMP
$env:SUMA_PLAYWRIGHT_MODULE = 'C:/Users/Administrator/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright'
$env:SUMA_CHROME_PATH = 'C:/Program Files/Google/Chrome/Application/chrome.exe'
$env:SUMA_PROTOTYPE_EVIDENCE_DIR = 'dist/quality/design-135/baseline'
node scripts/verify-design-workflows.cjs
node scripts/verify-enterprise-prototype.cjs
Get-Content scripts/check-docs.mjs -Raw | node --input-type=module
git diff --check
```

每条命令分别检查退出码。没有安装依赖或改变全局环境；本机浏览器工具经任务授权执行，临时文件只在任务忽略目录。证据保留在 `dist/quality/design-135/`，纳入提交的是代码、文档及紧凑哈希摘要，不提交浏览器缓存或日志。

## 修复及复验记录

只读审查发现并修复：问题详情标题未转义、离线比较未恢复选择、想法撤销入口无法再次打开、筛选移除对象后焦点丢失、命令面板首次向上错位。团队手工分配增加独立审阅冲突校验及字段重绘焦点恢复。浏览器首次检查发现质量页页签插入位置不匹配，修复后复验通过；哈希切换等待、团队交接详情定位两处测试脚本问题单独修正，没有把脚本失败记为通过。

原活动43954预览由提交 `a1c71f2` 的三份静态文件快照继续提供；新样稿在43955验证，未覆盖产品静态输出或重启产品服务。新旧静态预览均不读取用户数据库。没有删除原稿或历史失败证据。

集成 `b1439b3` 后逐项核对样稿、主规格及测试脚本SHA-256，与两组通过结果一致；不因无关日历提交重复这124项交互检查。同步修正设计中的日历现状，生产源码与该专项证据原样保留。

正常推送因远端新增 `57589dd` 被拒后，拉取并继续整合；交付状态与验证入口两处冲突均保留双方阶段记录，未强推。再次核对全部已测来源摘要不变，设计补充工时现状；生产源码及人员工时的原证据没有改动。

## 剩余验证

这里证明设计及合成情景交互，不是完整WCAG符合性、英文、读屏、触屏真机、大规模数据、持久协作、实际供应商／Harness恢复、真实离线执行、分布式竞争或生产验收。

未更改生产源码、依赖或公共合同，未重复无关Go／生产前端完整套件和构建。既有全套失败与远端资源日历的浏览器未验范围继续以原报告为准；本样稿检查不能使这些项目变成通过。
