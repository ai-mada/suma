# 工具重新检查限定验证

状态：scoped_verified；负责人：主集成代理；suma1.0.0；T-TOOL-REINSPECTION-194。完整31项／P0–P7目标保持active。

[需求](../product/tool-reinspection.md) · [合同](../../contracts/tool-reinspection.md) · [架构与AI SOP](../architecture/tool-reinspection.md) · [交互](../design/tool-reinspection.md) · [English](../en/tool-reinspection.md) · [Multica采用记录](../integrations/multica-tool-reinspection.md) · [摘要及证据索引](tool-reinspection-summary.json)。

## 结果

- 准确工具、版本、摘要与执行节点的持久重检请求，原幂等创建快照与当前head读取分开。
- 普通与Pool Worker通过原库存唯一拥有者执行新探测，原request/claim/fence关联库存与完成同事务保存；周期旧库存不能完成新请求。
- 丢创建／上报回执恢复原操作；旧ACK不当作当前fresh，恢复后以独立无ref周期报告重新开放本地门禁。
- Pool全部槽位忙时仍可重检；单项目403/409只隔离该scope并有界退避，保原记录，其他授权项目与任务继续。
- 简洁中英检查进度、原操作恢复与稍后核对；筛选空页不误称没有接入，本机状态回调不能夺取显式远程目标。

## 验证范围

21个不同顶层检查：17新增、4直接受影响旧检查；Go阈值／故障子例2个。控制面3、Runner6、Web10、Root合同1、实际纵向1，重复执行不重复计数。新正式表1、索引2、HTTP路径6、Schema9；合同必要生成2次，原接口可选引用nonnull和ACK成对约束均核验。原全量类型检查仅1条ReactNode诊断，修后只检查受影响源语义，0诊断；中英9082键配对，生产硬编码0。

真实原Worker／HTTP／SQLite／Journal／Git与自有Node Harness探测完成2次有因旅程：首次即时丢回执通过后，独审发现旧ACK过期时错误就绪，修正后保留原证据并在独立final副本复验。最终3次探测分别是初始、本次请求、恢复后独立当前周期；关联请求只1个，丢ACK后上报原op两次，未重新执行该请求。创建ACK亦丢失并同body/key恢复原v1快照。原工具保持paused，业务run为0，基线提交不变，Journal库存待确认0。

Pool原新测试扩展403／409两故障子例：B占满唯一slot时仍完成重检和原任务；A拒绝后gate关闭、原nonce／记录保留且退避，其他scope不被取消。全局机器身份／session失效仍终止整体身份，不吞错开放权限。

## 原失败与修正

实际普通／Pool HTTP曾接受inspection:null冒充周期报告，RED→GREEN。旧ACK就绪和Pool跨项目取消均保留真实RED及定点修复。前端真实消费者修复筛选空态、准确目标、元数据恢复、版本漂移及恢复弹窗锁。

代表界面共3组，按未变输入复用成功部分：中文1366筛选／清除／唯一入口、准确worker拒绝另一节点回调、英文390深色原创建回执恢复与unknown只读核对。首次native夹具workerID格式不合法被产品正确拒绝，修夹具后只继续剩余组；完整Main＋Panel又暴露恢复关闭锁，修后只复验最后组。3次隔离构建和3个宿主分别留证，0页面错误／意外请求，宿主退出均0；2张最终代表图已视检。所有bridge及API业务数据为合成样例，不冒充真实桌面节点执行。

## 边界与交付

- 只检查已登记配置；不接受任意执行输入，不安装／改模型密钥／启用工具／派发开发任务或代替人工验收。
- 真实Windows本地HTTP／SQLite／Worker／Journal／Git及自有Node Harness协议探测；不是厂商账号、模型、真实Harness服务或多机生产验收。
- OceanBase／PostgreSQL本阶段为初始Schema／方言源码与合同覆盖，未运行真实数据库环境；其他系统、安装包和规模继续独立验收。
- 界面为真实产品组件与显著隔离的合成API／bridge；原错误、原始图及部分成功证据保留，未重已绿且输入未变的业务组。
- 完整原31／P0–P7／58limited／34pending／92total保持active；本阶段不声明完整平台已交付。

原始日志、截图、数据库与Node夹具仅保存在忽略目录并记录摘要，不进入Git。正常阶段commit／push dev的远端一致性由本次交付回执核对。
