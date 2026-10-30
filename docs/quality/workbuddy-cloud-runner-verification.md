# 正式云任务Runner与恢复：限定验收记录

版本1.0.0；阶段T-WORKBUDDY-RUNNER-155；日期2026-10-30；状态：Windows／SQLite限定验收通过，完整平台范围保留。源码提交`22c3b9c93054aef8b930f2caa9a77e55e1e27a18`已推送并核对远端dev；本记录与实际发现的等待文案修正另行提交。关联[需求](../product/workbuddy-cloud-runner.md)、[架构](../architecture/workbuddy-cloud-runner.md)、[执行合同](../../contracts/workbuddy-cloud-runner.md)、[交互与AI SOP](../design/workbuddy-cloud-runner.md)。

## 已交付行为

正式私有／Pool领取在同一事务固定委派、原dispatch与完整执行身份，当前owner、机器、双边Pool授权和预算分别重验。中央单次创建、机器只读lookup、正式Worker checkpoint与outbox贯通；创建和刷新保留原正文／key，prepared／unknown不产生第二次供应方创建。

保护性等待保留原attempt/fence，释放process资源及Pool槽位，保留远端效果。供应方completed只等待交付证据；取消不伪造远端停止。只有中央在同一协调锁内核实未消费且无提交，才允许已知未创建失败及原策略内的重试。原Create receipt还核机器摘要、准确正文／key和提交ID。通用人工success／not_started不能绕过。

工具目录仅为云入口声明已实现的task_start，不虚构本机Profile、远端停止、模型选择、硬预算、候选或验收能力。连接按钮打开当前项目已有账号组件，保留草稿和未确认写入。节点面板复用父级正式事实，一主状态、下一步、折叠详情，中英文及窄屏同等可用。

移除四个无正式调用者的旧T143 Runner直连创建源码／测试文件；原字节及摘要保留在本地证据，Git历史不改写。供应方SDK与ACP底层继续复用，正式创建不保留第二路径。

## 验证范围

| 检查 | 实际结果与边界 |
| --- | --- |
| 新增不同Go顶层 | 44项：lookup／合同与生成18、领取6、原生Runner11、root合同／内核／可信结果9；分批按相关源检查，重复和已有用例不作为新增 |
| Web | 11个不同用例，其中10新增、1更新已有目录用例；覆盖固定事实、未知与未创建、草稿保护、同组件账号定位、中英文及等待不冒充审批 |
| 完整类型 | 最终隔离候选tsc退出0；保留此前真实诊断与修正，不声称首次全绿 |
| 国际化 | 7665对词条，参数／复数／标记检查通过；生产硬编码0 |
| 公共合同 | 782声明模型、783schema（含共享Error）、370路径；lookup机器接口闭合，原请求与Wait模型准确绑定 |
| 文档 | 工作树464份Markdown的本地链接、370路径与3418个schema引用通过 |
| 代码复核 | 修复Pool客户端回写丢失binding的P2；严格服务端保留，窄Create／Lookup没有扩权；其余限定核心复核无新增阻塞项 |

未运行完整历史套件或重复无关测试。Go夹具仅编译生产源与测试装配，不等于完整Go测试全绿；真实界面证据和源码检查分别记录。

## 冻结来源与构建

候选从`3f7fbbcea541785b5eeba4537d6ac92ce459ad93`的干净Git归档提取，业务旅程使用91项明确输入／移除四项旧文件的`efffc75b1f10facbb1dc563940ab4516af6b5f9ad07482fdabee09c6687430ae`。实际截图发现等待文案后，仅修正两个事实组件及一项回归；最终93项输入SHA256为`7b09f20b048dbbdae6291c71411ece4fc8ba59e1f36e41ab62c3f9389b5e1e6c`。排除桌面打包、控制面启动、讨论知识及并行T156的源码／生成WIP，见[原始输入清单](../../dist/quality/workbuddy-cloud-runner-155/product-inputs.json)和[安全检查摘要](../../dist/quality/workbuddy-cloud-runner-155/component-checks.json)。

初次隔离依赖路径缺少pnpm根junction，补齐后再执行真实检查。类型检查定位到Workflow可选reason／fence／worker字段，窄修空值归一化；未知值不能触发已执行状态。Go生产输入未变，测试夹具构造器纠正后只对测试装配必要二编；后续等待文案修正复用v2二进制，最终完整类型检查退出0。Vite共两次：首轮业务验证一次、实际发现文案问题后必要重建一次，不重复业务旅程。历史cache／dist证据不在Git归档中，归档文档检查报告缺少旧证据；没有为通过检查复制旧缓存，正式文档和合同检查另行通过。

## 实际界面验收与异常记录

第一轮隔离夹具在任何领取或供应方创建之前失败：测试程序使用了通用runner token，没有完成正式节点登记。随后正常登记已返回一次性凭据，但测试驱动的保存代码在Electron evaluate中错误调用require，凭据未持久化。原夹具14分钟期限到达后正常退出，其t.TempDir数据库被测试框架清理。请求、失败、现场截图、命令和安全终结记录均保留，不能声称原数据库已恢复；累计claim和供应方创建均为0。

新的持久owned-v2隔离夹具通过正常接口登记同一固定Worker ID，先完成真实Windows DPAPI密文写入／fsync，再进入界面。正式生产`Worker.Run`运行于独立OS子进程，全部领取、心跳、Create、Lookup和结果回写通过实际Service／Server HTTP；不使用夹具直接Create或伪造心跳。它使用新的隔离数据库，不是v1数据库的恢复。

两条业务旅程通过17项不同检查：正常与回执丢失两例各claim1、attempt1、heartbeat4、Create传输1、供应方POST1、正式result1。回执丢失发生于中央已保存准确任务后；仅终止自有Runner，在同一journal／凭据下重开，只Lookup原任务，未再次Create。两例均保持`waiting/cloud.delivery_required`、下游`pending`、用量`unreported`，不声称远端停止或人工验收。三个Worker子进程均已close：一个为预期终止，两个正常退出；商业模型调用0。

目视原截图发现“当前派发条件”把通用waiting误写为等待审批。修正只作用于节点／尝试事实，使用已有双语“节点等待中／Node waiting”；真正审批仍由`approval_required`阻塞事实明确表达，全局stateLabel未变。一项组件回归先红后绿。最终对原两任务进行三项只读视觉检查，所有请求限定GET／HEAD／OPTIONS，所有claim／Create／供应方／attempt／子进程计数保持不变，未重做17项业务检查。

最终图片：[中文1366](../../dist/quality/workbuddy-cloud-runner-155/01-cn-1366-formal-waiting.png)、[英文390深色概览](../../dist/quality/workbuddy-cloud-runner-155/02-en-390-recovered-waiting-dark.png)、[英文390执行事实](../../dist/quality/workbuddy-cloud-runner-155/03-en-390-recovered-facts-dark.png)。旧图和旧构建已哈希保全；[安全回执](../../dist/quality/workbuddy-cloud-runner-155/receipt.json)和[制品清单](../../dist/quality/workbuddy-cloud-runner-155/artifact-manifest.json)区分原业务来源与最终视觉来源。没有交付原秘密、私有请求或数据库。

## 完整平台范围

本阶段不代表真实WorkBuddy厂商、商业模型、机器ACP授权、云代码候选导入、独立测试、人工验收、生产发布、其他数据库、多物理机或跨平台安装已经验收。P0–P7及58 limited／34 pending／92总项继续保留，后续按原需求、任务、候选、测试、验收与发布合同逐项推进。
