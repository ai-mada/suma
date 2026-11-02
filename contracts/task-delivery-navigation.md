# 交付记录的准确导航与验收入口

版本1.0.0；阶段T-TASK-DELIVERY-NAVIGATION-214。完整31项／P0–P7继续。关联：[原交付投影](task-delivery-chain.md)、[准确工具交接](tool-task-handoff.md)。

## 实际断点与行为

原TaskDelivery已按当前权限区分任务、运行、测试计数、正式验收、团队计划和问题。本轮接通这些已有正式引用：任务详情和依赖图中，团队计划、问题及回流需求均能打开准确项目内的原页面。没有回调的受控嵌入使用可复制的当前项目链接，不把无回调等同于无权限；加载、拒绝、锁定及错误状态不允许导航。

“打开验收”必须打开原Review界面，不只选择同一任务。原TaskDeliveryPanel增加明确onOpenReview回调，普通任务详情接原openReview；跨视图时使用有界定位符`task-acceptance:TASK_VERSION:RUN_ID`，只包含公开准确依据。要求当前交付任务版本与调用方版本一致、原run_basis_state为current且存在准确运行；它不是通过验收资格，只是查看和决定的入口。

定位符随原`project / view=development / sub=tasks / record=TASK / panel`路由传递。新定位消费者只在当前账户／组织／project／task、准确版本及run均一致时，一次打开原Review；不查询其他对象、不改变Run选择、不自动决定或提交Review。错误、版本变化、无运行、锁定或原pending时不打开，显示可理解的中英文提示。成功打开后沿原路由清除当前定位符，保留任务位置；旧回调、重复render、来源A→B→A及详情卸载后重新读取不能重复打开已消费意图。明确再次访问尚带定位符的原链接是新的访问。

## 模块与权威

复用原任务读取、TaskDelivery、Review、TaskReviewRequestPanel、问题、需求与专家团队路由。无新后端API／数据表／生成合同／调度器／质量门禁。任务、运行、测试、正式验收和基线事实不合并成百分比，也不从测试节点成功数量推导can_accept。候选、不同Worker同提交独测、Review与基线双CAS沿原实现。

共享纯模型负责有限定位符解析及当前项目目标URL。任务交付组件负责使用正式引用、有效链接和原导航guard；验收消费者负责当前scope、一次意图及原pending优先。主集成只在App的现有TaskDelivery和原Review接缝接线，不重写整份App或复制原Review表单。

## 交互与必要验证

使用原组件和原生链接，保留复制／新标签与键盘能力；当前页回调可在通过原canNavigate后走原SPA路由。中文／英文、44像素操作目标、空态／失败／来源变化清晰，不增加复杂工作台。

定向验证：所有正式引用生成准确project／record／panel、阻止不合法标识与锁定导航、Review实际打开且不调用onOpenTask、跨视图准确版本／run的定位消费、当前来源变化／ABA／原pending、无自动写入。代表真实HTTP界面只验证新增导航和Review打开，不为这些未改动的执行、模型、独测或正式批准重复运行。完整原平台验收目标继续。
