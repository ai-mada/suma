# 工具管理后续纵向切片

版本1.0.0；完整31项／P0–P7目标进行中。T193–T195已实现接入、准确重检与未知核对；T198补齐准确节点高级配置及具体原因入口；T199实现Doctor统一维护建议、原进度与跨配置准确定位；T200已实现配置换新的正式原子交接。参见[维护建议验证](../quality/tool-doctor-maintenance-verification.md)与[换新合同](../../contracts/tool-profile-replacement.md)，不能把这些已有能力重新列为缺失CRUD。

## T200已实现：配置换新正式交接

沿原CreateToolProfile／matches与CommandToolProfile领域实现明确确认的原子换新：核对旧档案expected_version、当前项目与目标节点资格及新鲜完整配置；用户明确选择准确ID／版本的已有目标，或新建paused／v1目标。pause_original必须显式提供，可在同一事务中暂停原配置；复用保留目标原有状态，不自动选择匹配项或启用新配置。

正式关系保存准确旧／新档案快照、created／reused结果、原暂停意图、创建人及时间；关系快照总大小受2 MiB上限约束。只读outgoing／incoming历史按当前项目授权分页读取，保留不可变交接事实，不重新观察或冒称当前健康。关系不构成新的配置权威。

未知写入沿原账户、组织、项目、路由、正文和idempotency_key恢复原回执；不换新操作标识、不刷新版本后重复交接。CLI提供明确确认的tools replace与只读tools replacements，界面沿原写入恢复机制处理；不自动重试、改写历史hash／原计划、迁移任务或声称外部进程已经停止。原启用、暂停、撤销命令独立保留，revoke保留历史，不新增物理DELETE。

## 继续交付与验收边界

工具能力与模型、安装／探测、启用／派发、业务／质量仍分别取证。完整31项／P0–P7目标继续；真实厂商账号与模型、物理多机、OceanBase／PostgreSQL真实数据库矩阵、完整规模、跨平台及生产验收仍按原矩阵推进，T200不代表全平台完成。成熟SDK、原权限与回执优先，不重复建设已交付入口。
