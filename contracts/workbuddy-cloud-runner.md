# 正式云工作流 Runner 与持久恢复合同

阶段：T-WORKBUDDY-RUNNER-155；速码1.0.0；状态：合同冻结、实现中，未验收。继续完整研发平台目标，复用[152中央创建合同](workbuddy-cloud-execution.md)、现有Go DAG、正式Worker、机器权限、journal与outbox；不新增第二个任务调度器或供应方创建路径。

## 领取与固定输入

只有`external_workflow`／`plugin=workbuddy-cloud`／`workflow=workbuddy-cloud/v1`精确组合进入此路径。原不可变NodeBinding不修改，也不伪造本机ToolProfile。正式Runner原生声明该能力；声明只表示实现了合同，不能代替当前连接、owner、委派或预算资格。

领取前有界读取与真实公平窗口一致的候选及原claim回执，收集机器、项目／Pool相关组织和所有候选委派owner；一次按既有顺序取得完整身份／组织协调锁。之后重读真实凭据、成员、双方Pool／grant版本；取得项目／run锁后禁止再补owner锁。新owner未在集合内时整笔回滚并明确重试，不跳过其权限检查。

当前claim校验委派active、未过期、尚未消费、准确run/node、spec／request摘要、owner当前项目写资格、连接及grant／scope、原机器能力和两级预算。enforce云硬预算仍拒绝，不能靠没有ToolBinding绕过。

`CloudAssignmentBinding {delegation_id,delegation_version,delegation_sha256}`位于`dag.Assignment.workbuddy_cloud`。SHA覆盖授权不可变身份：委派ID、project、owner、connection、connection_version_at_issue、grant、client binding、run/node、spec/request SHA、expires_at和max_creates；排除state、creates_consumed、updated_at及当前version。领取版本固定，不改Node.Config。

绑定在领取原事务、dispatch回执持久化前完成；正式回放只读原assignment，不能选择最新委派。创建将委派v1消费为v2，只有同一原assignment的既存submission可以解释这一次增长；不能把合法消费误判为需重新绑定，也不能允许撤权或替换。

调度内核提供明确的事务内assignment绑定／回放验证接缝，直接扩展现有claim实现，不新增旧／新版兼容分支。正常节点无需云hook；精确云节点没有受保护绑定时不能取得可执行派发。

为原attempt核对和失联收尾，新增通用`dag_external_assignments`初始表：run/node/attempt为主键，固定fence、worker、dispatch ID、protocol、授权binding身份／版本／摘要、原AssignmentRef JSON及其SHA。只保存执行身份，不复制prompt、OAuth或供应方秘密。在领取同事务写入，不能在响应后补写；`ReadExternalAssignmentTx`以准确范围读取且验证摘要。完整初始schema校验先于任何DDL，拒绝旧／残缺库，不回填、不reset。

## 准确只读恢复

两机器namespace新增`POST /workbuddy-cloud-submissions/lookup`，正文`CloudLookupRequest {project_id,delegation_id,assignment}`。它是闭合只读请求，没有operation key、CAS、供应方ID或替代正文；不写数据库、不解密token、不访问供应方。

`CloudLookupResult {project_id,delegation_id,assignment,state,submission?}`的state仅not_found／prepared／unknown／task_known。found必须带同state、同project／delegation／完整原Assignment的记录；not_found不带submission，且不构成重新创建资格。consumed=1而无记录属于完整性冲突。

核当前机器、原credential hash、owner当前写资格及连接owner／app／client／OpenID／grant／scope、Pool当前双边资格和精确版本；供应方token到期不阻止读已存事实，也不为读取消费预算。相同current attempt／fence／worker允许lease过期、委派撤回／到期或节点waiting／reconciling／终态后取原事实；已被新attempt替换仍拒绝。它不允许再执行或调用供应方。

保存的原Create AssignmentRef不能被心跳后lease/state覆盖。lookup客户端只比较当前Pool context与原请求的不变身份，允许lease/state观测值变化；found仍逐字段核对原请求与保存记录。执行请求的现有严格校验不放宽。

## 正式执行、checkpoint与观察

Execute在通用external adapter前识别受保护云绑定，仅调用控制面Create／Lookup／Refresh；旧143 Runner直连供应方创建入口若无正式调用者则移除，由正式checkpoint替代，不保留并行fallback。供应方SDK及其协议测试继续由控制面复用。

首次外发之前原子保存control origin、project／worker、原claim／AssignmentRef与其摘要、固定binding、准确Create body及稳定operation ID。不能保存OAuth、ACP ticket或重复prompt。落盘失败零外发；控制面HTTP响应丢失、重启和当前版本读取不抹除原请求。

重启先识别此checkpoint，不能先被普通journal封成不可恢复通用unknown。先读原claim／submission，prepared／unknown绝不重发Create；task_known只沿原ID观察。not_found本身不授权发送；只有尚有效的原领取／委派，以及已落盘的同一Create body／key，可以有界恢复同一个控制面请求，控制面仍是唯一一次外发权威。禁止换key、换attempt或按名称猜任务。

每次Refresh亦先持久化原body／key；恢复沿原操作或明确保留该次未确认的观察，不把后来的记录冒充原ACK。持续执行复用原心跳与进度outbox；只在事实变化时写安全进度。默认有界轮询及总deadline，不自动Prompt最初Create正文，不向Runner传供应方凭据。

## 被验证的等待与资源

`dag.Result.workbuddy_cloud_wait`为`CloudWaitEvidence`：protocol、原claim ID、原AssignmentRef、固定binding、原Create operation ID、可选原submission ID／version及明确cause。它不包含prompt、供应方状态替代值或成功证明。机器始终`Stopped=false`，不提交Tool／Workspace／Artifact／Test／Release伪证据。

控制面在原机器结果事务中读取调度快照及中央提交，验证全部身份／摘要；由可信resolver决定结果，不能接受Runner手填的成功或“没有发生”。已经存在prepared／unknown／task_known时进入既有DAG`waiting`状态及明确cloud原因，完成时间为空；不激活下游、不增加attempt、不重新取得Create资格。释放process类资源和Pool slot，effect／未知费用事实保留。供应方completed只表示等待可验证交付。

只有原中央提交不存在且委派未消费，并在相同协调／run锁下阻止迟到旧Create后，才能确认该attempt未产生云创建；可信resolver可记录已知未开始失败／取消，释放资源。瞬时无效果失败的重试仍受既有明确幂等／退避／次数／deadline约束。存在任何创建事实或完整性矛盾时不得进入此分支。

云lease失联时，内核依据原受保护调度快照转为cloud等待并仅释放process资源，不伪造远端停止；旧Create／Refresh不能在等待态取得新执行资格。随后同原attempt的机器结果可补齐事实；已确认的同operation回执可重放，不能用新operation改写等待。日志及事件保留来源与原未知事实。

Run取消保留远端效果未确认的云等待，不把等待节点直接改成cancelled。通用reconcile的手填success、unknown的通用retry均拒绝；只有经可信resolver确认未创建的失败才可按原策略重试。后续ACP、可信候选／独立测试／正式验收采用新的真实证据推进，不借供应方状态刷绿。

## 界面、验证与完整目标

原节点面板复用父级准确Run／Node事实，派生待委派、待领取、执行节点等待提交事实、原创建待核对、原任务已知、等待交付、取消未确认等文案；不建另一状态机或轮询卡片。浏览器委派／撤销／observe原操作与机器checkpoint分开，中英文、窄屏和键盘同等可用，不提供“再次创建”按钮。

必要验证覆盖私有与Pool正式claim、完整owner锁与CAS、原binding回放、v1→v2合法消费、两级enforce拒绝、真实Worker一次创建、响应丢失／重启原body恢复、未知不重建、等待释放process而保留effect、迟到fence、取消不假停、completed不绿及下游未解锁。实际旅程用正式Worker.Run／原HTTP链，不能用152专用fixture直接Create代替。

本阶段继续完整P0–P7及58／34／92总范围。机器ACP访问／持久权限、云代码候选导入、独立测试／验收、真实供应方、多物理机、其他数据库、安装及生产各按实际证据交付，不以本合同或局部测试宣称完整平台完成。
