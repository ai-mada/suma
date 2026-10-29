# 执行工具工作区验证

2026-10-29 · T-TOOLUX-129 · Windows x64。实现与使用说明见[设计](../design/execution-tools-workspace.md)。本轮限定组织与项目设置的执行工具，不代表全平台生产验收完成。

## 结果

- 工具相关5个测试文件，58项通过。其中新增10项实际组件交互测试，覆盖空状态、筛选与搜索、断线／悬挂重试、统一刷新、暂停恢复、清单自动识别、多个候选、手工名称、无效输入、中英文草稿保持；既有身份、工具事实、模板和严格库存解析48项通过。
- `pnpm --filter @suma/web typecheck` 通过；`pnpm --filter @suma/web check:i18n` 通过，成对目录6348键，生产硬编码0。
- `vite build --outDir ../../.cache/tool-ux-129/web-dist --emptyOutDir` 通过，构建有既有大分块提示。构建输出隔离，未覆盖活动预览。Go1.26.10直接使用本地已安装工具链，将当前工作区控制面与Runner构建到本轮缓存，未修改Go源码。
- 真实Chrome／Playwright：1440×1000桌面、390×844窄屏、中文／英文、深色，检查空列表完整宽度、状态筛选、登记弹窗、自动清单识别、暂停保存、刷新保留选中项和窄屏详情刷新控制；无页面异常与横向溢出。界面多状态使用合成HTTP，真实服务旅程另列。
- 独立代码复核发现并修复：读取失败后的悬挂重试不能恢复历史可用状态；刷新栏移至列表／详情之外，使窄屏详情仍可暂停、恢复和刷新。新增悬挂重试测试先失败后通过，复核确认两处已覆盖。

## 真实服务旅程

使用独立空SQLite、回环控制服务和本轮隔离Web构建。浏览器实际登记项目范围执行身份；Runner真实探测本机Node.js，页面粘贴实际非秘密清单并创建暂停档案；用户核对后启用，真实Runner通过自身凭据上报，平台读取确认可派发。继续从页面暂停，确认不可派发；刷新页面仍为暂停；重新启用恢复资格；撤销节点凭据立即阻止派发。原CAS和服务端权限路径保持使用。

这是实际HTTP、数据库、机器进程与页面组合验证；商业模型请求0，任务执行0，没有把本机探测等同于模型健康、任务成功或正式验收。测试凭据只存在本轮内存／隔离数据库，不进入Git。自有服务与Runner由测试脚本关闭。

## 全套检查中的既有问题

最终Web全套：1195项，1192通过、3失败，无工作进程异常。以下三项在本轮产品源码修改前的基线运行中已经失败，未修改其归属模块：

| 文件 | 用例／现象 |
| --- | --- |
| `requirement-design-consumers.test.tsx` | `treats version zero as an editable empty design and saves an incomplete draft without creating tasks`：布尔断言失败 |
| `write-action-recovery.test.tsx` | `keeps the original response validator with an unknown operation across provider and validator changes`：预期2次请求，实际3次 |
| `i18n/development-consumers.test.tsx` | `preserves dispatch selections, request versions and raw tool evidence across locale changes`：原推荐模块读取缺失项目列表，`.find`失败 |

因此本轮工具范围通过，不声明整个Web测试套件通过。共享工作区存在其他正在进行的终端、桌面与开发讨论改动，保留原样，不纳入本轮提交。

文档检查同时报告其他在途终端文档的13处待创建验证报告／摘要链接；本轮新增设计、报告及摘要的相互链接均存在。API结构检查完成315条路径、2993处模式引用。不声明全仓库文档检查通过。

## 证据与复现

原始检查保存在忽略目录`.cache/tool-ux-129/`；最终界面截图、测试摘要、浏览器及真实服务回执与SHA256清单位于`dist/quality/execution-tools-129/`。机器可读精简结果见[摘要](execution-tools-workspace-summary.json)。截图只证明相应数据状态；合成与真实证据分开记录。

主要命令：`pnpm --filter @suma/web exec vitest run src/ToolWorkspace.test.tsx src/ToolSetupGuide.test.tsx src/ToolFacts-consumers.test.tsx src/tool-model.test.ts src/execution-pools/tool-identity.test.tsx`；全套使用`vitest run --maxWorkers=5`。浏览器插件未提供，使用已安装Playwright及本机Chrome。外部设计服务调用因可能上传截图／项目线索被自动审批拒绝，本轮全部设计、实现与验证在本地完成。
