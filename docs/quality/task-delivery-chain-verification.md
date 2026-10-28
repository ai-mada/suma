# 任务交付闭环与本地执行诊断验证

状态：限定范围完成；全平台继续实施。负责人：主集成代理；日期：2026-10-28；任务：T-DELIVERY-112；产品：1.0.0。

[精简证据摘要](task-delivery-chain-summary.json) · [任务交付合同](../../contracts/task-delivery-chain.md) · [本地诊断合同](../../contracts/runner-journal-inspection.md) · [中文操作](../development/task-delivery-chain.md)／[English](../en/task-delivery-chain.md) · [参考来源映射](../integrations/referenced-conversation-integration.md)

## 已交付

依赖图采用明确的任务勾选，焦点与派发清单独立，最多选择16个未验收任务。存在未保存依赖、未知结果、冲突或过期图时阻止派发；不隐式加入前置任务。沿用既有专家团队预览、固定配置、逐任务模型与预算、启动和原操作恢复，没有第二个派发器。

任务详情侧栏的“交付与质量”默认折叠，展开后按当前身份／项目／任务读取绑定运行；刷新和打开不启动执行、创建问题或提交验收。接口用同一当前授权及项目协调事务读取正式事实，协调锁本身涉及数据库写入，不宣称纯SELECT。摘要分开表达业务状态、运行依据、节点、测试节点、正式验收、当前运行的团队引用和问题来源；每类引用最多20条并显示完整性。测试节点计数不是用例覆盖率，运行成功不等于正式验收。未开始节点不会生成 attempt=0 链接。

明确退回验收会在同一事务保存原Review、阻塞任务、清除旧验收资格、撤销已验收依赖下游的资格并创建质量问题；同一原操作回放不重复创建。正式问题来源绑定原始运行失败事件及回执，或实际退回Review及回执；可编辑的Data不能证明来源。普通问题回流和人工采用分诊共用一个专用领域回执，绑定租户、项目、问题和需求；通用记录回执或伪造元数据不能建立关系。公共记录Create／Update拒绝内部保留前缀，此结论不扩展成所有命令的全局前缀保护。

完成旧代码后再改变任务依据，团队投影显示阻塞，不能继续显示等待旧依据验收。读取验收证据时，仅确实无记录按业务失效处理；SQL或上下文基础设施错误原样传播，不伪装成无效验收。

Runner增加 `--inspect-state --state <明确目录> --language zh-CN|en-US`，在读取凭据及执行配置前进入独立只读分支。限制单文件16MiB、最多100条白名单引用，拒绝链接／非普通文件及不稳定文件身份，不输出原命令、配置、日志、凭据或错误正文，不初始化目录、不改ACK、不发请求、不重跑。它只覆盖一个journal，不代表完整设备健康、服务器确认、持久隔离原因或恢复界面。

## 定向检查

共43个不同的受影响顶层用例通过：平台20、合同1、Runner／CLI6、后端词条1、Web组件13及Web模型2。子测试、重复执行和跳过项不重复计数；1个Windows链接用例因创建符号链接权限不可用而跳过，未据此声明链接攻击测试通过。新增行为均保留失败后修复的定向观察，包括缺失交付接口、旧依据错误投影、退回未建问题、基础设施错误误分类、伪造回流来源以及未开始测试节点的attempt零值链接。

主要Go命令及实际退出码、日志摘要在精简摘要中逐项记录。受影响核心命令为：

```text
go test ./internal/platform ./cmd/suma-contracts -run 'TestTaskDelivery|TestExpertTeamCompletedSourceChange|TestEvidenceReadInfrastructureErrorsAreNotInvalidAcceptance|TestExpertTeamOrchestration|TestWorkspaceMetadataCannotReplaceExplicitTestNode|TestReviewRejectsWorkspaceCandidateNotCoveredByTests|TestReviewRechecksAuthority|TestReviewSuccessReceipt|TestCompensationInvalidatesTransitive' -count=1 -json
go test ./internal/platform -run '^TestIssueIntake|^TestTaskDelivery|^TestFailureDiagnosis(AdoptionRollbackAndConcurrentSingleIssueDraft|ExistingIssueRequirementOnlyOpensCurrentDraft|ActualDAGRepairAdoptionAndOriginalIntakeReplay)$' -count=1
go test ./internal/runner ./cmd/suma-runner ./internal/localization -run 'TestInspectJournalState|TestJournalInspectionCLI|TestTaskDependenciesCatalogExactPairedSources' -count=1 -json
```

使用已缓存Go1.26.10及离线依赖，Go操作串行。源文件最后仅整理换行；新增内部保留前缀的词条随后单独复验一次。Web三份受影响文件原14项通过后，增加未开始节点attempt零值用例并定向通过，不重复整套用例。类型检查通过，当前两种语言4825个键成对、项目自有生产硬编码0。合同生成及检查通过，当前486个模型、257条API路径。

本轮仅构建Web、Control和Runner各一次。Web构建42.69秒，IDE分块体积警告保留，未声称已优化。未新增依赖、业务表或历史兼容逻辑。构建、测试、安装／原生GUI及生产验收分别看待。

## 真实本地执行与界面

独立初始数据库、独立本地Git仓库和测试账号，不使用用户正式数据库。实际自有DSH协议命令通过Runner领取开发节点、在独立工作树修改源码并提交；另一Runner导入机器封存的候选，对相同SHA执行Python三个断言。开发与测试使用不同worker，测试命令带正式测试证据标记；这是一个物理主机的两个Runner，DSH为自有确定性协议实现，没有调用厂商或模型。

候选提交为 `a102a0fa60349db161b3e7aa1ad5724cfb34ae0d`。实际一条运行、两个attempt，开发成功、测试成功，3断言／0失败／0错误。执行完成时仅等待人工验收，下游B没有run。

中文界面实际完成：打开所选任务的交付摘要；打开同一原团队计划并区分验收与依赖等待；打开任务验收并显式退回；查看原退回原因及自动创建的问题；显式转为需求并打开同一草稿；返回依赖图后刷新看到该正式回流引用。只勾选B预演时提示等待前置任务正式验收，没有隐式派发A，也没有启动B。

此界面退回是隔离测试账号的质量路径验证，没有授予验收或执行部署。人工接受和下游解锁由受影响服务端用例覆盖，本次实际界面没有重复授予验收。最终持久化事实为：任务A blocked／版本3，运行与测试仍 succeeded；1条 rejected Review、1个来源问题、1个版本1需求草稿，下游B仍未启动。

最后点击测试节点attempt1后，环境重载结束了隔离服务；已完成的界面操作保留为实际工具观察，不重建成截图。最终HTTP读取返回连接拒绝，随后通过只读SQLite／WAL核对上述计数及原始回执。原Control／两个Runner／辅助进程均已不存在；没有正常结束回执，未确认临时浏览器页关闭，不宣称其已关闭。最终测试详情加载、该新面板英文浏览器及最后截图未完成；英文验证限于成对词条、组件检查和真实Runner双语诊断。用户原有预览和数据库保持不变。

Runner实际只读诊断在同一自有journal上分别执行中英文，返回unknown=1／pending_progress=1；前后文件SHA256一致、文件数仍为1，输出未出现测试植入的秘密正文，无网络或模型调用。

## 来源、归档与后续

参考ZIP和bundle来源沿用已核验的实际包，按现有Go领域适配，不导入旧Python服务、旧版本或Git历史。C206补充为有限本地只读诊断；未完成的持久隔离、人工核对恢复和完整工作台继续保留。新增分享链接先经网页读取、正常浏览器重载及原URL会话解析失败，随后由Firecrawl普通basic页面提取成功返回HTTP200。已获取当前分享可见正文：REQ／TEAM／INT／SCH／QA／KM共64个编号，按[设计采用映射](../integrations/shared-roadmap-adoption.md)与现有领域和证据对应；它是规划材料，不能代替源码和实际验收。页面原文在KM-010后的解释处中断并显示Continue generating，不推测未出现的后续章节，原P0–P7和91项台账保持。来源正文摘要为 `1135aeaa9519c56d5daa95d6d0ae37fda944e0eeb4ecaae7edc361277086fc30`，正文只在忽略缓存中保存。

当前源码清单摘要为 `03e2dd0116b9e7f7760d80efbb13b012243f10097930ff4a632bfcb82e0c38a0`，43个源码／合同文件。私有夹具、数据库与WAL、构建、执行日志、工具回执、检查清单及最终只读审计位于忽略目录 `.cache/e2e/delivery-112/` 与 `.cache/mcp019-deps/`；原文件不删除。Git仅提交必要源码、当前合同、设计指南、精简证据与状态。

完整台账仍为58项限定实现、33项待实现、共91项。物理多机、真实厂商模型、OceanBase／PostgreSQL实集群、长时吞吐、跨平台安装、完整可访问性／移动端矩阵和生产验收不属于本次通过范围。
