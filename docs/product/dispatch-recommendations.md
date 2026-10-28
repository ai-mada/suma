# 执行工具与模型推荐 SPEC

状态：T-ROUTING-122 实施；产品1.0.0。负责人：主集成／控制面／运行时／工作台代理。关联需求13、14、21、26，P2／P4。验收证据以质量报告为准；本文件不代表全平台完成。

## 用户目标与交互

在原任务派发页按需展开“智能推荐”，基于当前任务、需求、设计和明确选取的已发布知识，对实际在线且满足权限、路由与预算条件的工具及模型进行一次语义比较。先显示候选范围，再选择只读分析智能体和真实模型、参数、预算；明确确认上下文将发送给该模型后开始分析。未展开时保留原简洁派发表单。

结果给出理由、问题与最多3项候选。用户选择并可编辑合同允许的参数，明确采用后打开原任务派发计划；仍由用户在原计划中明确启动。分析成功、计划生成、开发执行、独立测试和正式验收各自保持真实状态。无自动启动、自动审批或另一个调度器。

## 范围与不变量

- 候选只有 `model`、`codex`、`fixed`。原生／Octop经Go网关选择已观察模型；Codex使用官方实际目录及推理配置；其他工具呈现已批准的固定配置。
- 每次最多检查32份工具配置、呈现24项候选，跨配置轮转；范围不完整、排除数量与原因明确。未知成本、性能保留未知，不生成虚假分数。
- 预算受Go原绑定、报价及组织／项目策略约束。需要硬预算时排除仅观察用量的工具；无法证明报价的成本限制不被称为已保障。
- 模型建议只读，固定一次调用、无工具、无仓库能力。固定输入包含当前依据摘要，不发送流程命令、配置、地址或密钥。
- 采用绑定来源、原始输出、完整五字段选择摘要、采用人和时间；原操作恢复不重复分析或建立第二份计划。
- 当前读权限先于历史正文与回执恢复；任务／知识／工具／模型／路由／策略变化会阻止旧建议采用。合法旧版本仍能在授权范围内作为历史阅读。
- 中文与英文完整成对；用户、模型原文和不可变证据保留原文。新语言通过既有目录扩展。

## 最小验收

1. 未分析、分析中、失败、未知、待确认、已采用分别展示，可刷新定位原记录。
2. 一次分析只产生一次Go网关请求，真实绑定与用量可追溯；未知结果检查原操作，不能触发第二次调用。
3. 采用只建立原计划，启动仍需原动作；后续独立Worker测试和正式Review不可省略。
4. 旧输入、无权读取、超预算和伪造候选被服务端拒绝，原操作保持幂等。
5. 页面主体简洁，候选、参数、依据和恢复细节按需展开；窄屏及键盘可以完成只读查看和参数选择。

## English contract summary

A bounded recommendation compares currently authorized execution alternatives. It is one read-only analysis through the existing Go gateway. A human edits and adopts a typed choice into the existing dispatch plan, then separately starts execution. Current authorization, budgets, immutable versions and independent quality gates remain authoritative. Unknown performance, cost and outcome remain explicit. Full Octop GUI/CLI, commercial vendor billing, physical multi-host and production acceptance remain separate backlog items.
