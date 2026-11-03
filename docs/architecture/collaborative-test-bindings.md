# 协作入口测试配置与 AI SOP

速码1.0.0 · T-COLLABORATIVE-TEST-BINDINGS-223 · 负责人：控制面组 · 状态：限定验证通过，范围见验证报告。接口源为三个原请求DTO；生成[OpenAPI](../api/openapi.json)和TypeScript，不新增API路径、表、队列、调度器或依赖。

讨论CreateRequest与推荐AdoptRequest增加可选test_bindings。推荐的模型Choice和分析来源摘要不变；AdoptedTestBindings保存采用时原Plan的六字段，provenance比较完整快照。

ExpertTeamPlanItemRequest使用可选非空指针数组：创建省略使用发布定义；修订省略保留同上下文原选择；显式空数组恢复默认；非空替换。null被HTTP拒绝，省略与[]形成不同操作摘要。保留限定原任务、角色、流程版本、智能体、资源和开发profile。源重建恢复三字段并比较冻结六字段；不查询当前目录来补历史。

HTTP先核验原始字段存在性、闭形、整数及64项上限，再转换DTO。业务层沿原builder／事务权限／CAS／回执／路线／库存检查。前端共享原选择器和冻结事实组件，来源epoch隔离迟到响应。

AI SOP：1.读取当前准确计划和权限；2.明确用户测试意图；3.预览并检查不同Worker及版本；4.审阅冻结输入；5.以稳定原操作标识提交；6.未知结果先恢复原回执；7.执行后收集同提交JUnit；8.交付独立人工Review。工具启用不代表已执行或已验收。

流程：协作入口 → 原TaskDispatchPreview → 冻结测试绑定 → 原Start复验 → Go DAG → 独立Worker测试 → 原质量与Review。

```mermaid
flowchart LR
 A[讨论／明确推荐采用／团队计划] --> B[原派发预览与冻结绑定]
 B --> C[原Start重验当前来源与权限]
 C --> D[独立开发Worker]
 D --> E[另一Worker同提交测试]
 E --> F[原质量门禁与人工Review]
```

后台详情重读只暂停展示与操作，不取消同源诊断读取。真正写锁、来源变化或卸载取消在途请求；已结算失败不被列表轮询静默重试。
