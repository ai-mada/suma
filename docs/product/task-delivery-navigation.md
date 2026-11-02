# 从交付记录进入准确页面与原验收

版本1.0.0；阶段T-TASK-DELIVERY-NAVIGATION-214。状态：限定实现已落，App与代表实际界面验收以[最终质量报告](../quality/task-delivery-navigation-verification.md)为准，本文不提前声明实际通过。负责人：工作台／主集成。关联：[唯一合同](../../contracts/task-delivery-navigation.md)、[交互设计](../design/task-delivery-navigation.md)。完整31项／P0–P7继续。

## 用户目标与操作

任务详情及依赖图的原交付区已能分别说明任务、运行、测试计数、正式验收、团队计划和问题。本轮让用户从这些已有正式引用直接到达准确页面，不再只选中任务却没有打开验收，也不因为嵌入组件缺少回调而失去正常链接。

1. 展开原交付区并读取当前任务记录。查看原运行与测试事实；测试成功数量不是通过验收资格。
2. 点击团队计划、问题或回流需求，打开当前项目内的准确原对象。普通单击可使用原SPA回调；无回调时使用真实链接。
3. 点击“打开验收”，在普通任务详情调用原Review入口。跨视图时携带准确任务版本及Run定位，待目标页核对当前依据后打开原Review。
4. 在原Review界面审查证据和选择决定；是否允许批准、是否满足质量门禁以及提交结果均继续由原领域判断。本轮导航不自动批准或提交。

## 四类准确目标

以下字段均通过统一模型与`URLSearchParams`生成，保留当前`project`，不拼接任意页面地址。

| 正式引用 | 原页面定位 |
|---|---|
| 团队计划 | `view=development`、`sub=runs`、`layout=team`、`panel=expert-team:PLAN_ID` |
| 问题 | `view=quality`、`sub=issues`、`layout=list`、`record=ISSUE_ID` |
| 回流需求 | `view=requirements`、空`sub`、`layout=list`、`record=REQUIREMENT_ID` |
| 验收 | `view=development`、`sub=tasks`、`layout=list`、`record=TASK_ID`、`panel=task-acceptance:TASK_VERSION:RUN_ID` |

验收链接只在原交付记录的任务版本与调用方一致、`run_basis_state=current`并存在准确运行时生成。该条件仅使“打开原界面”可定位，不产生`can_accept`。

## 模块与AI维护SOP

- `task-delivery-navigation-model.ts`统一验证受限标识、准确项目URL与有限验收定位符。
- `TaskDeliveryPanel`使用原正式引用、真实链接及导航guard；`TaskAcceptanceLocator`核当前账户／组织／项目／任务、版本与Run，一次调用原`onOpenReview`，不发额外API请求。
- App只接现有详情、专家团队、问题、需求和Review入口，不复制表单或建立第二套审批状态。

AI修改此链路时先核对正式引用及当前scope，再选择原入口；不得从编辑字段、测试计数或链接猜测审批资格。未确认写入优先沿原恢复处理，不换body／key、不清Journal。版本或Run不符时提示重新读取当前交付记录，不替换为其他运行。回调拒绝不算消费，但不会因重复render循环重试；原blocked解除后才允许再尝试，已消费定位不因数据刷新或A→B→A重新打开。

## 边界

无新增后端API、数据表、生成DTO、调度器或质量门禁。没有新增候选、同提交独测归属或基线证据聚合投影；这些事实及原Review、双CAS基线交接继续沿原实现。加载、拒绝、错误和原pending均阻止当前页导航；在合法可用链接上另开标签不离开当前页，也不提交验收。定向检查及最终代表界面证据分别记录，真实执行、完整App、跨平台及生产验收不由本次导航推定完成。

成功打开后，仅在原账户、项目、任务及定位符仍一致时消费当前路由意图，详情重新挂载不再重复打开。原验收运行选择在异步选项加载期间保持明确初值，人工改选与原未确认请求正文不变。
