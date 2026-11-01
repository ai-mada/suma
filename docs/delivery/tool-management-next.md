# 工具管理后续纵向切片

版本1.0.0；完整31项／P0–P7目标进行中。T193–T195已实现简洁接入、准确重检与未知核对；T198补齐准确节点高级配置和具体原因操作入口，不能重复列为缺失CRUD。

## 优先1：Doctor统一准确下一步

现有GetToolProfileDiagnostics／collectToolProfileObservationLockedTx仍使用静态next_action，尚未整合维护能力、原inspection占用及recovery进度。下一切片读取原索引，返回紧凑、准确profile／worker／version和固定动作：有资格且无占用才建议原CreateToolInspection；unknown建议原CreateToolInspectionRecovery；进行中查看原操作；能力不足进入准确节点配置。GET只建议不执行，写沿原授权、CAS、operationID、proof及原回执。现有重检与核对不是待开发能力。

## 优先2：配置换新正式交接

现有CreateToolProfile／matches与CommandToolProfile启用、暂停、撤销完整；换新仍需数个独立操作，缺少准确旧档案→新档案交接回执。后续采用明确确认的原子操作：核旧expectedVersion与新fresh完整配置，复用准确matches或建paused，明确是否暂停旧项，保存两者关联。不得改写历史hash或原计划，不自动启用，不声称外部进程已停止。原revoke保留历史，不新增物理DELETE。

上述两项本阶段未实现，需各交付独立纵向流程与受影响检查；成熟SDK和现有门优先，不新增平行配置权威。实际未安装工具／供应方账号／真实模型、物理多机及完整规模／跨平台验收仍按原能力矩阵记录。
