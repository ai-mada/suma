# 问题修复、回归关单与重开验证

状态：suma **1.0.0／T-QUALITY-119，本机限定闭环已验证**。更新：2026-10-29。责任：主集成、问题领域、命令执行和工作台。完整P0–P7、需求1–26以及58项限定实现／34项待办／92项总计保持不变。

[领域合同](../../contracts/issue-repair.md)、[架构](../architecture/issue-repair.md)、[中文操作与AI SOP](../development/issue-repair.md)、[English](../en/issue-repair.md)与[精简证据](issue-repair-summary.json)共同说明本轮范围。源码变更摘要：`7cc06dcd62afd29b9c3f8731a9a47563f9abc673f7cfd989b242abe71a3f7ef5`，共 42 个源码／合同文件，不含文档正文与memory；清单保留在本轮忽略目录，当前提交中的源码字节另行核对。

## 实现与正式边界

- 问题保持一个正式Record；专用事务保存问题、周期、新修复任务、原唯一需求回流关系与固定回归绑定。已有回流需求复用，不存在时创建草稿，不自动派发、发布需求或批准。
- 通用创建／更新不能制造关闭、修改生命周期或伪造修复关系。每次写入核对当前身份、项目、原操作和预期问题／周期版本；旧回执只恢复原结果，不创建第二任务。
- 回归绑定该修复任务的当前已发布计划、修订、SHA与非空用例集合。测试来源的问题必须包含原失败case及相同精确JUnit选择器，不接受无关绿色测试替代。
- 关单重新核验当前正式任务验收、需求／设计／候选依据、固定计划与逐例报告。每条选定证据还须关联同运行的调度器候选pin、成功产码节点的原attempt／fence／Worker／证据SHA／提交；测试Worker必须不同。仅有测试节点自报head、旧运行、旧计划、未知或缺失证据不能关单。
- 明确`command`流程可使用完整固定`command/text-v1`档案；私有与Pool领取、进度及结果重核当前授权与指纹，拒绝结构化Agent冒充命令。智能体开发继续走原任务预览；没有第二派发器。
- 关单与重开属于当前浏览器审批权限，机器和普通API写权限不能替代。重开保留关闭快照并进入新周期，新周期创建新任务，旧关闭运行不得再用。

## 必要检查

| 检查 | 结果与证据范围 |
| --- | --- |
| Go定向检查 | 21项集成／回归检查通过，涵盖问题命令、严格HTTP字段／别名、任务范围计划筛选、旁路、权限、原操作、过期依据、候选来源、固定命令与旧指纹、原测试计划及当前初始化；实际OS纵向另1项通过，合计22个不同顶层用例，未跑完整套件 |
| Web行为 | 5项不同场景通过；增加历史证据href后仅复验其中1项，不重复计数。涵盖显式准备、发布范围绑定、冲突重核、关单确认／重开、未知回执恢复、身份切换与403清理 |
| 类型／生成／语言 | 最终TypeScript通过；生成器检查541个模型Schema、274条API及共享TS类型；5348项中英文目录检查通过，自有生产源码硬编码0；产品版本仍1.0.0 |
| Web构建 | 2次成功，分别是集成构建及实际界面发现后补齐历史证据链接。保留既有大chunk提示，不将构建成功写成性能验收；未单独重建控制面或Runner二进制 |
| 当前数据库 | 真实SQLite新库及当前合同重启／旧合同写前拒绝包含在定向检查；OceanBase／PostgreSQL未由本轮验证 |

原始记录在`.cache/mcp019-deps/`及`.cache/e2e/issue-repair-119/`，Web聚焦记录在`.cache/e2e/issue-119/`。所有缓存、数据库、截图、日志和制品不进入Git。

## 真实OS／HTTP旅程

最终通过记录为`20261028T174300510Z`，5.53秒完成测试函数。使用Windows、Go1.26.10、Node22.16.0、Git2.56.0.windows.3与真实SQLite：

1. 在隔离Git基线保留`sum(a,b)=a-b`，真实Node执行JUnit失败，按受保护原用例创建问题。
2. 明确准备唯一问题需求及新修复任务，发布回归计划，绑定原case与精确选择器。
3. 已固定的命令配置由开发Worker实际领取；在独立工作树改为`a+b`并真实提交，原管理员工作区和HEAD不变。
4. 另一Worker通过HTTP领取同一调度器候选，实际Node运行同用例并采集原始JUnit。候选提交、报告SHA、attempt／fence与两份工作区证据一致。
5. 测试账号执行正式Review协议、关单、重开及新任务准备；原关闭快照保留，新任务无运行，原唯一回流需求复用。

这里的三次业务Node执行不包含只读版本探测和Git元数据查询；没有调用模型或商业供应商。测试账号的审批协议验证不代表真实客户人工签收，更不代表生产发布。最终独立测试提交、原始报告SHA和执行身份见精简证据。

## 代表性真实界面

采用相同源码的另一份实际OS旅程数据，确实显示第2轮未启动任务、第1轮关闭／重开历史及准确候选。验证中英文热切、折叠历史、原运行链接、原回归链接；实际点击后进入准确计划与运行，显示1用例通过、0失败／错误／跳过、原报告摘要及同一候选。390像素无横向溢出；最终截图、AX及生成制品摘要保留在忽略目录。

仅做读取、导航和语言切换，没有在浏览器发起执行或批准。数据库中的运行、attempt、Review、修复周期、任务、需求及报告数量前后一致。原始XML按钮已触发，但窗口结束前没有观察到完整XML正文，因此**不声称原始XML浏览器读取已验收**；原报告确实由真实进程采集，逐例及报告引用已在界面验证。

### 失败、修正与证据保留

- 初次编译／合同断言和夹具未提供当前需求版本的问题修正后，仅复验受影响检查；没有把失败批次算成通过。
- Windows夹具使用Node的设备路径作Git全局配置时失败。已在本轮自有目录实际复现`Invalid argument`，改用每attempt的真实空配置／hooks目录后复现成功；没有放宽正式执行或证据门禁。
- 可选界面hold原设15分钟，超过Go测试默认10分钟；功能旅程断言此前完成，但该次命令整体超时失败。已保留其数据库、对应源码归档、摘要及前后数量核对，不把它算成测试通过。hold上限改为5分钟，最终不挂hold的同一真实旅程通过；没有为旧窗口重启服务或重发原执行。

## 当前数据与剩余验收

唯一初始数据合同为`store.InitialDataContract = suma-initial-quality-v1`，独立于产品1.0.0。不将允许任意Issue状态的旧合同数据默认为当前受保护周期；旧目录保留、拒写，新实例使用独立空目录／数据库，没有迁移、回填或兼容别名。

本轮只接受可完整证明的同运行`candidate_from`回归来源；复杂map／子流程／外部来源缺等价证明时明确阻止关单。历史保存完整，当前API只显示最近10个旧周期并披露总量／是否完整。完整缺陷分类／去重、批量治理、手工／应用／发布测试、跨应用版本、物理多机、外部CI／数据库／安装和生产规模仍按完整计划推进。Octop真实操作时间线等后续项仍独立，不由本轮继承完成。

## English verification scope

This is scoped local evidence for the current1.0.0 issue-repair domain:22 distinct Go checks and5 Web behaviors passed. The actual journey reproduced a failing JUnit case, prepared a protected repair, committed code in an isolated worktree, independently retested the exact scheduler-pinned candidate under a different Worker, exercised the fixture owner's review protocol, closed, reopened and prepared a new task. No model/provider calls, customer sign-off or production deployment are claimed.

Chinese/English history and exact evidence links were inspected in the browser, including390px layout and real passing-case drilldown. The optional inspection process timed out; its SQLite and source evidence were preserved, and a necessary no-hold journey passed after fixing the harness boundary. Original XML browser-body visibility remains unverified. Current instances require a separate empty database declaring`suma-initial-quality-v1`; incompatible data is preserved rather than migrated or erased. Full platform scope remains active.
