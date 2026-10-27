# 1.0.0 验证入口

更新：2026-10-28。当前只有单一 **1.0.0** 初始化基线；完整平台继续实施。本页按能力指向准确报告，不重复阶段流水、旧测试数字或旧数据库升级说明。详细过程仍可从专项档案与 Git 追溯。

| 当前范围 | 验证与限制 |
| --- | --- |
| 模型选择跨普通任务、专家与原生修复 | [当前限定报告](model-selection-workflows-verification.md)、[精简摘要](model-selection-workflows-summary.json)：受影响接口／CLI／Web、实际预览及修订／刷新、零新增模型和执行；原生修复仍不支持 Codex |
| 单一初始化结构与共享版本 | [初始化报告](initial-version-verification.md)、[摘要](initial-version-summary.json)：当前183表、同结构正常重启、未知既有结构拒绝；不升级旧用户库 |
| 安装版 Codex 实际编码 | [准确源码与环境的专项报告](codex-app-server-verification.md)：安装版 CLI 配合自有 Responses 端点；配置 ACK 不是提供方身份，提醒不等于硬预算；不将旧切片作为本轮厂商验收 |
| 其他领域能力 | [交付状态](../delivery/status.md)与[完整文档索引](../README.md)列出各域当前实现和独立证据 |
| 未完成范围 | [完整实施计划](../delivery/implementation-plan.md)、[产品基线](../product/v1-baseline.md)：P0–P7及扩展需求1–22仍全部保留 |

检查按变更影响选择，失败修正后仅复验相关路径。源码完成、协议夹具通过、真实执行、正式业务验收与生产验收分别记录；不累加重叠测试为虚构同轮全套。费用未定价、partial／缺失用量、未知执行结果都不能推断为零或成功。

当前 Windows 构建与本机浏览器证据不证明 OceanBase／PostgreSQL 实集群、目标吞吐、物理多机、跨平台安装、厂商账户、生产模型、签名、支付或完整平台验收。构建清单、日志和私有夹具留在忽略目录，Git只收必要源码、合同、设计与精简证据。
