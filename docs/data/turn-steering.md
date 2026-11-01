# 活回合与补充指令数据

状态：1.0.0当前实现中；负责人：控制面；需求：T-TURN-STEERING-188。依据：[合同](../../contracts/turn-steering.md)、[领域架构](../architecture/turn-steering.md)与 `internal/platform/turn_steering_store.go`。

两张窄事务表保存活目标和显式指令；原讨论仍是消息来源，原命令回执保存幂等结果。不使用权限授予表、原生问题表或日志充当队列。

| 表 | 正式身份与索引 | 作用 |
|---|---|---|
| suma_steering_sessions | 主键id；tenant/project/worker/operation唯一；tenant/project/run/node/attempt/fence唯一 | 绑定准确task、原Assignment、profile及native thread／turn；version、state、数据库期限与原Open正文摘要 |
| suma_steering_instructions | 主键id；session/message唯一 | 原消息与作者、准确session、version、单调观察及未送达／未知事实 |

原消息按不可变ID、序号、作者与正文摘要核对；发送接口不接收新的正文或原生地址。每session最多32条指令，先在同一执行作用域事务中检查数量和唯一性，再提交指令、session版本与原操作回执。32,000 UTF-8字节／8,000 Unicode字符与原讨论生产者一致，不裁剪中文正文。

Open只在当前机器、完整Assignment和原受保护任务运行核对后登记。正常非任务工作流返回明确 `not_task_execution`，不写假session。Create需要当前浏览器权限、本人human消息、预期任务／session版本和真实活目标。观察以原生client请求ID、确定参数摘要与单调sequence绑定原记录；完整写出和原生接受不合并为单一成功状态。

当前身份与原scope先核验，已提交操作的准确回执先于fresh lease／状态检查恢复。session关闭后的Poll只用于当前有权机器读取最终完整指令和尾项；它不会恢复新写资格。新指令、已开始投递事实、原生关闭及Result各自按相应规则提交，unknown不能自动重发或变成成功。

初始化对当前注册、列和关键唯一约束进行写前检查；缺失或未声明结构先拒绝，不重置数据或回填历史形状。SQLite实际验证与其他数据库适配／迁移证据分别记录，不能从方言源码推断OceanBase或PostgreSQL已验收。正文及私有Assignment按租户敏感业务数据保护；公共执行证据只保存原身份、摘要和投递事实。
