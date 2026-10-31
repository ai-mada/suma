# 原云会话观察的明确团队共享

T-WORKBUDDY-HISTORY-SHARE-165；产品1.0.0；2026-10-31。状态：限定实现与验证完成；Windows／SQLite上原owner明确共享、项目viewer读取及原操作恢复已实证，[证据](../docs/quality/workbuddy-cloud-history-sharing-verification.md)保留夹具故障、续验和验收边界。负责人：讨论、云工具接入、工作台与主集成。关联[私有观察合同](workbuddy-cloud-collaboration.md)、[任务讨论](task-conversations.md)及[关注通知](task-subscriptions.md)。完整31项要求、P0–P7和58限定／34待办／92总账保留。

## 用户动作与可见范围

原私有load的当前owner从已保存观察明确选择条目，选择同项目目标任务，预览精确正文、来源状态及“有目标任务读取权限的项目成员可见”，再明确“共享所选”。此动作生成本人human讨论消息。共享是独立不可变副本：此后按当前目标任务权限读取，不要求读者持有原云账号；撤销原云连接不收回已明确共享的消息。预览和提交前均说明此范围。

没有自动共享整段、静默向知识库／模型发布、供应方助手代言、Prompt、权限回答、停止或任务完成。供应方task_id不是平台任务ID；准确平台任务绑定优先，未取得准确绑定时由用户选择当前有权的同项目任务，不能通过名字、最近任务或第一个候选猜测。

## 选择、版本与正文

只允许已终止的`loaded/interrupted/limited/rejected` load中非空白的受支持条目；loading、unsupported、空文字不可选。序号严格递增、唯一，最多8项。服务端重读准确load、当前owner及原来源，每项均属于该load。selected正文保留原字面文字，按序以两个换行连接；沿现讨论上限32000 UTF-8字节／8000 Unicode码点，无NUL、非法Unicode或其他原投影禁止字符。超限明确拒绝，不自动截断。

`expected_load_version`绑定终止版本，`expected_task_version`绑定当前平台任务；当前任务状态不限制正常讨论，也不因此改变版本／执行／验收事实。观察覆盖始终`unverified`，partial／失败来源状态必须保留，不能显示为“完整对话”或“任务成功”。

## 闭合DTO与接口

新增DTO由`internal/workbuddycloudcontract`定义，同源生成TypeScript与OpenAPI，拒绝未知／重复字段、null标量、不安全整数和非法Unicode。所有接口浏览器cookie限定，真人、当前项目write；不接受API／Agent／Runner身份，不发生供应方I/O。

- `CloudHistorySharePreviewRequest`：`project_id,task_id,expected_task_version,expected_load_version,sequences`。
- `CloudHistoryShareRequest`：上述字段加`preview_sha256,idempotency_key`。正文、原ticket、URL、任意消息作者及原RPC不接受客户端传入。
- `CloudHistorySharePreview`：`project_id,task_id,task_title,task_version,load_id,load_version,load_state,coverage,source,entries,body,body_sha256,selection_sha256,preview_sha256,sharing_scope`；entries为选中原`CloudHistoryEntry`，sharing_scope固定`project_task_readers`。source为本人原`CloudCollaborationSource`。
- `CloudHistorySharedEntry`：`sequence,kind,text_sha256`。
- `CloudHistorySharedObservation`：`load_id,load_version,load_state,coverage,source_run_id,source_node_id,source_submission_id,entries,body_sha256,selection_sha256`。不包含原请求摘要、vendor task_id、owner的operation key、正文以外的私有会话、连接／grant／ticket。
- 现`TaskConversationMessage`增加可省略非null的`cloud_observation`，为`CloudHistorySharedObservation`；其`run_id`仍是目标任务当时上下文，不作为原云来源。原消息请求不增加任意来源写入口。

POST `/workbuddy-cloud-history-loads/{id}/share-preview`返回200预览，不创建正式消息、share、receipt或通知；POST同前缀`/shares`返回201的`TaskConversationMessage`，重复原操作仍返回准确原消息，不是新消息。两者请求均无query、闭合JSON，复用现错误信封和安全固定文案。

preview_sha256为服务端规范化预览事实的SHA256，绑定当前租户／本人、准确load／终止版本／原来源、选择及逐项正文摘要、精确合成正文、目标项目／任务／版本和共享范围。selection_sha256绑定有序sequence／kind／text_sha256。提交重读并重新计算，原文、来源、任务或选择变化均需重新预览；不是浏览器时钟token或新的持久预览数据库。

## 授权、原操作与事务

预览和新共享先以当前真人target-task write建立planningAuthority，核对同项目准确load的owner与原来源，沿164 `cloudHistorySourceTx(...,false)`而非外部连接资格，不要求OAuth或旧机器在线。最终提交前再次assert当前权限，所有数据库事实在同一事务内读取／校验。分享时不索取新的供应方ticket。

同key恢复先核对当前目标任务write和原receipt请求摘要，再返回已明确共享的原消息；不得因为原云来源后来变化而逼用户新key重复发布。原请求字段／key不可改写，原task版本已前进也不重复发布。不同body／scope复用key按现commandReceipt冲突处理。消息仍通过当前task读取权限读取，来源引用不额外授予私有存档权限。

新共享在一个数据库事务中完成：当前source／版本／preview核对，现讨论序号CAS，human消息，明确共享来源关系，既有关注通知，固定无正文活动，原operation回执，最终权限assert及commit。任何步骤失败全部回滚。不能先调用自行commit的Append再补来源。Root抽取现讨论事务核心，原Append保持原合同行为；不复制另一套序号或通知逻辑。本阶段不新增提及选择，但既有其他消息提及行为保持。

来源关系采用当前platform基线的新表`suma_workbuddy_cloud_history_shares`，至少绑定tenant/project/target-task/message/owner/load/version与不可变安全来源payload；message_id唯一。初始化前检查完整表／列／键及platform基线，不重置、回填或采用不明schema。已共享消息批量读取来源元数据时只沿目标任务权限，不重新向私有load查询；核对human作者类型及正文摘要，避免伪造来源或跨任务关系。

新增安全错误code为`workbuddy_cloud_history_share_invalid`(400)、`workbuddy_cloud_history_share_changed`(409)；已有401／403／404及原source错误保留准确语义。普通网络失败保留原未知操作；明确来源／权限错误清除本域预览正文，不清除仍有权的原节点事实。恢复存储仅保存原元数据请求，不保存私有预览正文。

## 实现归属与最小验收

控制面代理：新增共享DTO／store／领域／HTTP／元数据投影和定向测试；不改Root拥有的旧讨论核心、主HTTP／database注册及public生成器。工作台代理：独立选择／目标任务／预览／原操作恢复组件与model及中英文；原164区域只加入选择／挂载，讨论消息只增来源呈现，不碰Knowledge／桌面／其他21项未完成改动。Root：旧讨论事务核心、类型／元数据读取接线、HTTP／初始化、API生成、合同与架构及最终集成。共享签名先单独冻结。

验收至少覆盖：预览0正式写／0外连；跨owner／项目／API拒绝；loading／unsupported／超限／乱序拒绝；精确预览hash与task/load变化；消息／share／通知／receipt原子提交与失败回滚；同key丢响应恢复一条消息／一组去重通知；其他当前有权成员读取共享副本却仍不能读取原私有load；当前撤权及旧私有来源失效后的原receipt恢复；现Append原行为。界面覆盖中英、代表窄屏、选择预览、范围确认、失败与恢复；实际协议夹具和真实厂商、物理多机／其他DB／生产分别记录。
