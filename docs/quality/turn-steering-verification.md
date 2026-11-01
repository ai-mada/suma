# 同一活回合追加指令限定验证

T-TURN-STEERING-188；suma1.0.0；2026-11-01；负责人：主集成。状态：本阶段限定通过，完整平台继续。[合同](../../contracts/turn-steering.md) · [产品](../product/turn-steering.md) · [架构与AI SOP](../architecture/turn-steering.md) · [数据](../data/turn-steering.md) · [交互](../design/turn-steering.md) · [采用记录](../integrations/multica-turn-steering.md) · [机器摘要](turn-steering-summary.json)。

- 原讨论本人human消息旁新增明确发送入口；单一权威context固定原文、准确任务版本、真实活session及历史，选择目标后单独提交原ID与CAS，不重复输入正文。
- 原配置与turn ACK后才由当前机器公布准确live source；普通／Pool共用当前完整Assignment、profile／模型绑定、原DAG与工作区，非任务执行明确返回正常无cap结果。
- 两窄表与原事务回执管理Session及Instruction，同session同message唯一／最多32，当前人资格、原任务运行、权限／预算／pause与lease沿旧成熟边界复核。
- 原Journal、递增client请求ID与单writer接独立steer ACK；保存、观察、开始写、完整写、原生接受／拒绝、关闭和unknown分别记录，未知不native重发、不新建或恢复回合。
- 原native关闭后，closed Poll读取最终尾项并准确保留未派发记录；观察／关闭原HTTP先Result重放，成功结果必须与持久化原指令全量闭合匹配。
- 原讨论正文上限32,000UTF8字节／8,000Unicode字符完整复用；中英界面、原scope恢复、独立busy锁、390px与键盘交互接通。
- Multica采用记录、开发工具矩阵当前Codex行、主架构HTML及产品／架构／数据／交互／English设计同步，原旧表其他产品行明确标历史核查基线。

31个不同定向顶层检查（28新、3直接受影响原例），24个Go子例另计。平台7、Runner8、实际CLI1、Web11、Root共享合同／生成接口／双语4，均有真实绿证据；原失败和命令保留，不把重跑算成新检查。最终全Web类型0、8803对中英文词条及0硬编码，一次隔离生产组件构建、3合成GUI、两张Root查看视口图与正常原生退出0。

实际CLI首遍通过：固定本机Codex与生产driver／Client／Journal，原配置及turn ACK后Open，当前活turn接受唯一steer／同turn ACK；完整写出、native接受与关闭准确留证，后续自有模型请求包含完整原已保存消息。新配置catalog只读进程和活turn进程各1，owned模型HTTP2，首请求被正常steer取消；观察与Close先于Result，原操作重复HTTP只恢复回执。0商业／0用户凭据／无强制超时，原生正常exit0、job Finish nil、PID已不存在。

平台另以真实TaskDispatch预览／Start／普通Pool Claim／HTTP Open／Create／Poll／Observe／Close／Result及回执验证。覆盖当前本人human、他人／API拒发、原回执先于fresh、pause新work门、迟到真实事实、same-session-message唯一与32上限、closed Poll最后尾项、unknown拒成功与明确拒绝可闭合、两张坏schema在Initialize前无其他写入。当前实库为SQLite。

- 平台首次测试遇并行Runtime entry字段中间态编译阻断；其余夹具先触发缺合法managed计划、原协议成功字段及pause明确409预期，均只修准确夹具／预期，不弱化生产门。
- 平台closed-tail夹具显式Rollback后漏释放自有连接，原测试已精确停止并留非0证据；补cleanup后只重受影响3top通过，不称生产死锁。
- 独立Runtime窄审修复1项P2：无效JSON-RPC error.code可误判确定拒绝。固定规范安全整数code及string message，无效ACK保留unknown；只新增原malformed_error子例并复验受影响分支。
- Web新增消费者捕获原操作恢复的刷新门错误，精确修复并只重原2红；32k边界仅重原源与父消费者。首次最终types、词条、隔离构建与GUI均通过，没有重复全套。

代表GUI：Chinese production discussion selects one saved own message and explicit exact live target; one ID-only create, saved receipt before separately read native acceptance, without duplicate delivery；English390px dark keyboard recovery settles the original message/session/body/key before returning to a different message and newer target; no automatic second create or preselection；Closed English390px history separately shows unknown writing, explicit native rejection and never-written instructions, read-only without fabricated acceptance。

- 真实安装CLI使用固定0.166.0-alpha.5.1及原SHA，生产driver／Client／Journal是真实现；Responses模型与控制HTTP均为自有fixture，无商业调用、无用户凭据。平台SQL／浏览器当前授权在另一真实HTTP／SQLite层验证，不能合称一次真实厂商全App验收。
- 实际旅程一次通过，只有一条native turn/steer与同turn ACK，后续owned模型请求含完整原消息；2CLI进程为新配置只读catalog与活turn各1，首模型请求被正常steer取消，不是故障重试。原Open／观察重复HTTP均为同key恢复，未重复native指令。
- GUI使用真实TaskConversation父／Panel／Card，账号、任务与HTTP为合成；恢复及历史明确是父级harness。3代表检查／两张Root查看视口图／正常原生退出，不是完整登录App或客户验收。
- 当前原生steering针对受保护TaskDispatchPlan选定的开发Agent节点，包含沿该计划的讨论交接与专家团队任务；普通非任务或其他节点不伪造能力。其他工具／Harness、持久旧thread／多turn恢复、安全秘密输入等仍需独立交付。
- 原生接受只证明接收，不证明模型遵从、代码质量、独立测试、人工验收或生产发布；追加输入不能改原正式验收基线或授予新权限。
- 未重复SDK、版本／feature／Schema导出、全套或安装矩阵；当前实库为SQLite。OceanBase／PostgreSQL、物理多机、其他OS安装、规模／生产与原31／P0–P7／58限定／34待交付／92总账继续，原171显式停止GUI缺口保持。
- 两份无关本地历史审计原SHA保持且不提交；缓存、数据库、日志、截图、私有home、依赖与二进制不进Git。

原始证据在 .cache/turn-steering-188/{platform,runtime,actual,web,docs,visual}；Root离线Go命令在 .cache/follow155/root-go，正式摘要逐项保存SHA。最终交付清单在 .cache/turn-steering-188/source-manifest.json。原始日志、截图与私有配置不提交。
