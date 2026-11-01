# 工具维护建议 PRD

状态：T-TOOL-DOCTOR-199阶段实现；suma1.0.0；负责人：平台集成；关联[合同](../../contracts/tool-doctor-maintenance.md)、[原工具管理](tool-management.md)、[架构与AI SOP](../architecture/tool-doctor-maintenance.md)、[交互](../design/tool-doctor-maintenance.md)。完整31项／P0–P7目标保持。

用户在查看工具诊断后，应能直接进入准确的维护下一步。已有检查正在运行时无需反复发起；结果未知时先核对原操作；同节点另一配置占用时，应能找到原配置及原记录。配置、维护能力、可派发、实际执行及验收分别表达。

| 编号 | 用户目标 | 可观察验收 |
| --- | --- | --- |
| D199-1 | 选择维护动作 | 原四项派发检查保持，独立建议准确绑定当前profile/version/worker与短期限 |
| D199-2 | 跟踪原检查 | pending／claimed显示原进度；租约未知不提供无条件新probe |
| D199-3 | 处理失联 | 原unknown先定位原检查与核对，proof_stored仍等待机器ACK，关闭后才可明确新检查 |
| D199-4 | 理解同节点占用 | 当前配置A能定位同节点B的原检查；先读原对象，不伪挂为A的检查 |
| D199-5 | 维护能力不足 | 缺失、过期或不支持分别提示，进入准确节点配置，不宣称已修复 |
| D199-6 | 使用CLI | JSON与中英文table显示相同维护事实，只GET、不自动执行建议 |
| D199-7 | 安全刷新与交接 | 权限、caller或来源变化隔离旧响应；本机未知写意图仍沿原操作和回执恢复 |

不新建派发器、恢复服务或配置权威。安装、模型调用、任务开发、质量验收、配置换新原子交接与完整生产规模，继续由原领域和后续交付分别证明；详情见阶段验收报告。

[限定验证与剩余范围 / Scoped verification](../quality/tool-doctor-maintenance-verification.md)。
