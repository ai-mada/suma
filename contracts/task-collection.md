# 有界任务分组合同入口

状态：T-TASKS-137；负责人：规划领域。统一正式合同为[项目任务统一查询](task-workspace.md)。任务行只有 `/task-workspace`；`/task-collection/summary` 只提供相同基础筛选下的完整计数与有界分组页，不提供另一套任务行、状态或派发权限。

实现、错误、游标、当前权限及恢复规则均以统一合同和生成的[OpenAPI](../docs/api/openapi.json)为准；[交互与架构](../docs/design/task-collection.md)、[English](../docs/en/task-collection.md)、[证据](../docs/quality/task-collection-verification.md)。此文档是职责导航，不保留已移除接口的兼容别名。
