# 修复计划修订与运行溯源：限定本地验证

T-P4-033／suma 1.0.0，2026-10-27。**限定本地功能阶段已验证，`stage_complete=true`：同机实际旅程、最终界面／冷回读及主预览前向恢复后的warm检查均通过。** 准确提交以Git历史和本地提交收据为准。完整P0–P7保持实施中，本文不代表厂商、多物理机器或生产验收。

见[领域合同](../../contracts/candidate-repair-revisions.md)、[PRD](../product/candidate-repair-revisions.md)、[架构](../architecture/candidate-repair-revisions.md)、[界面验证](candidate-repair-revisions-ui.md)及[精简机器摘要](candidate-repair-revisions-summary.json)。原始数据库、日志、截图、模型请求和临时产物保留在忽略目录，不提交Git。

## 实际闭环

复用032已验收的A／X准确来源，不重新开发或复测来源。新建原合成任务、同一修复任务和未开始的下游D，共3项业务任务；形成3项新运行。真实Git冲突保持失败，没有封存成功候选。

| 对象 | 实际结果 |
|---|---|
| 原合成计划 | `3b966d5623632bef06f7ceca8700db5b`；真实冲突、原件及来源顺序保持 |
| 初始修复P1 | `bdc9f3a7836c7e928111327c4f141d80`；ready配置B。明确修订为P2时仍无run，P1完整原行保持 |
| 修复P2 | `c61258cbc0b0c374c778f4af6f4b672a`；改选A并另行Start。A在工作区准备前因自有空目录暂时不可用失败，保存准确assignment／profile的类型化启动证明；无模型、候选、后继测试或工具进度 |
| 健康修复P3 | `f61dbb4a3aeb3c8c4f3cfb66abda6877`；恢复依据核对后明确改选B。准备新预览期间任务仍指向P2失败run，另行Start才绑定P3 |
| 同一修复任务 | `34b550eb4e8908d969ff6410c1439718`；最终accepted v4。P2与P3两条真实`bound`历史分别保存task v2／v3和准确时间，不覆盖原失败 |
| 候选与独立测试 | B产出 `cd9c611ac90ea3531247ec2340b74b083bdc1ef4`；A导入同一准确候选，只执行一次4项正式断言，再由获明确许可的本地夹具测试owner Review一次 |
| 下游D | `f89dab5b7ac8804927ee41c195a5a27a`；baseline v1绑定上述已验收候选，任务v2、todo、无run。没有派发D或冒称其后续开发完成 |

实际Control及两个Runner均正常退出0，无强停。`actual/audit-final2.local.json`核对45项事实全部通过；审计自身未调用模型、执行测试或写业务。独立最终冷回读也正常退出0，全部选定业务schema／行／账户及原非DB制品保持，无Runner、模型或业务命令重执行。

原032来源10个文件及辅助文件保持，来源DB仍为`c90ea8ab1cc105df1b5748d949be1933ef0a7936e158404ab740f2fdedd01483`。冷回读后033实际DB为`5d55064183aa76dbb4d7bfc551cc42223eddd5b5ebaaef86226565a26c3c83ff`；两者是不同用途的独立数据库，不互相恢复覆盖。

## 会计事实与资源复用

本次健康执行仅使用自有loopback模型协议夹具：**6次响应、6次真实工具调用、0厂商调用**。未报告模型Token或费用，保持`unreported`，不能推断为0。已认证P2启动失败的输入／输出Token均已知0、`usage_state=complete`，但原生修复暂无计费率，因此仍为`unpriced`，费用、币种及rate不编造。

新增两条原生attempt账本均绑定一次准确原结果；组织／项目Token聚合各为version **21→25**、unresolved **4→5**，原spent **80**、reserved **0**保持。两条CNY聚合完全不变。仅这两条原Token聚合行变化，其余原选定业务行／结构及原身份／会话数量保持，未忽略整个账本。

原生SDK bundle及manifest逐字复用032；11项源输入、161项实际闭包输入均匹配，不重复16来源Git／SDK夹具。正式测试只有上述4项一次，开发自测、组件检查及审计不是新增正式验收数量。

## 必要定向检查与最终构建

顶层组、子例、复验及重叠选择分别记录，不相加为阶段总数。

| 切面 | 范围与证据 |
|---|---|
| 修订领域 | 6个新顶层及4个受影响032组；版本／来源CAS、单后继、原回执、旧计划执行围栏和全attempt恢复依据；最终命令`20261026T225240536Z`通过 |
| 启动证明与履历 | executioncontract／Runner／platform共11个顶层；只在工作区准备前证明、完整私有／Pool身份、旧事实保留、唯一归属、nullable观察履历和已知0结算。收据`contracts-agent/verification.local.json`位于单独的`candidate-conflict-repair-stage-033`忽略目录 |
| 实际HTTP与CLI边界 | 修订HTTP／history和CLI各1组；严格整数／null／别名、受保护来历、原操作及分页。另1个HTTP顶层覆盖private／Pool两路无效证明不结算、合法结果与原回执重放只结算一次 |
| 界面 | 初始10个受影响文件37项；后续仅观察说明国际化与Review打开时读取当前run各1项定向回归；当前类型检查退出0。4,335对中英文词条按未变目录输入复用，不重跑无关组件 |
| 合同与构建 | 413个Schema、235条API路径及TS合同检查；受影响包vet及四个Go程序构建退出0。434项生产源摘要`32b90a2a564fcb0e7e40577e13abe2885a16dc0ec13dcab5feb2d77cb5c58d58` |
| 最终前端 | `web-delivery`为342项源、135个产物，入口摘要`cf059de98de9627fe5edf1ca2faa0e7bf15e375820e4ac149f2cf56d50c873d6`。只为已发现的新UI缺口重建隔离cut；原cut保留，大chunk警告保留 |

浏览器实际warm旅程14项及最终cold代表性检查通过，覆盖中英、准确历史URL／刷新、键盘历史展开、旧版禁止Start、独立测试后明确Review和D准确基线。实际发现Review弹窗可能沿用旧的“运行中”摘要后，修正为打开时重新读取当前run；最终cold仅打开弹窗验证当前成功run，没有再次提交Review。全设备／窄屏矩阵、浏览器断网及权限撤回故障没有因组件通过而宣称全部实测。

## 保留的异常与主预览恢复

首次`audit-final.local.json=false`保留：核对脚本使用了错误的制品路径。修正读取位置后，同一已完成数据库和制品通过45项审计；没有改产品、重跑模型或正式测试。首次目录故障Arm因缓存脚本将ISO时间转换成本地无时区文本而被年龄守卫拒绝，修正缓存日期读取后继续原P2；没有替换业务操作或产品构建。

主预览旧032两个进程对象经系统查询为terminal，**exit code 1、Wait返回0**，不能记录为正常退出0。确认终止后保留原DB／WAL／SHM及WAL感知备份，按当前数据库前向恢复033；未恢复旧快照或悄悄重启旧正常版本。

033主预览恢复到normal时保留184张旧表的完整schema／全部行，仅新增两张空表及1个显式索引至186。主预览warm的21项核对通过：186张表schema完全相同，182张非锁表完整行保持；4张内部授权锁表34行身份保持，仅sequence各增加24。两张新增表仍空，18 authority、0 Grant、18会话和1failed／2succeeded旧运行保持；135个实际服务产物逐一匹配。当前Control187352／holder237760使用最终匹配产物。没有恢复计数或把warm之后误述为全部表的原始行完全不变；精确证据为`primary-upgrade/warm-verification.local.json`。

## 尚未完成

真实厂商推理质量／账单、跨物理机器、分布式数据库、生产隔离与规模、完整安装／IDE／渠道／移动端及完整P0–P7继续实施。版本化修订不自动换工具、自动Review或重放未知效果；普通failed、空输出和错误文字不追认为执行前证明。

English: the same-host repair revision journey, final cold UI readback and 21 primary warm checks are verified after a WAL-aware forward recovery. Warm authorization reads change only 34 lock sequences by 24; the 182 other tables remain byte-exact at the row level. Six owned protocol responses report unknown model usage; only certified pre-workspace failure has known zero tokens, still without a price. This does not establish vendor, multi-host or production acceptance.
