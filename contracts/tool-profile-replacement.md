# 工具配置换新与不可变交接

T-TOOL-REPLACEMENT-200；suma1.0.0；完整31项／P0–P7目标继续。复用[原工具管理](tool-management.md)、[准确匹配](tool-profile-reuse.md)、原当前批准权、profile状态与操作回执；不建立第二配置权威、派发器或恢复器。

## 明确提交

`POST /tool-profiles/{id}/replacements`是当前browser-project-approve写入，成功201表示已提交一次准备交接关系。Runner／Pool／项目令牌不能调用。请求包含：

| 字段 | 约束 |
| --- | --- |
| `project_id` | 当前项目，与路由scope一致 |
| `expected_version` | 原配置当前版本，1–2147483647 |
| `worker_id`、`profile` | 明确目标节点与完整不可变Profile，可以显式迁移至本项目另一合资格节点 |
| `pause_original` | 必需布尔，false不能因省略被默认；只影响原配置，不证明进程停止 |
| `idempotency_key` | 必需原操作标识，8–128 UTF8字符、无控制或首尾空白；恢复保留原身份／路径／正文／key |
| `name` | 仅新建时必需，非空、已修剪、最多200 UTF8字节 |
| `reuse` | 仅复用时必需且非null，闭形`{profile_id,expected_version}`；此时name字段必须省略，包括空串也不接受 |

当前授权先于回执读取；原回执命中先于新的CAS、库存和匹配检查。回执的hash绑定原actor及完整意图，包括原profile路由ID。确定的新请求才执行以下同一事务：核原配置版本及非撤销状态→当前目标节点资格和新鲜库存→完整EqualProfile且available=true→准确显式复用或新建paused→可选暂停原配置→不可变关系及原操作回执一并提交。

缺少reuse且发现当前paused／enabled完整匹配时返回原`tool_profile_match_exists`冲突，要求重新读取并明确选择，不静默采用第一项。复用核准确ID／版本，拒撤销和自指；目标原来enabled／paused则保持其原状态，不自动启用或暂停。新建目标始终paused/v1。pause_original=false不更新原项；true且原enabled才真实暂停并递增版本，已经paused不空涨。旧Profile摘要、既有计划和运行依据不改写。

## 正式结果与容量

`ToolProfileReplacement {id,project_id,original,replacement,disposition,pause_original,created_at,created_by}`全部必需。disposition仅created／reused，original／replacement是提交时的完整公开ToolProfile快照，created_by是原提交actor。稳定回放保留两快照与actor，不根据新的库存、语言、状态或版本重算；它不证明当前可派发、模型健康或业务验收。

不可变关系表`suma_tool_profile_replacements`仅管理关联和历史，不授予配置权。tenant/project/id为主键，original／replacement各自有scope+时间+ID索引；其写入与原suma_operations回执、profile变更同事务回滚。单条关系序列化上限2MiB，提交前必须核对与后续读取相同的完整编码边界，不能先提交一个无法恢复的回执。未知既有表布局在任何DDL前拒绝，不reset、backfill或引入历史版本分支。

## 只读刷新与历史

`GET /tool-profile-replacements/{id}?project_id=…`读取准确原关系。

`GET /tool-profiles/{id}/replacements?project_id=…&direction=outgoing|incoming&limit=…&cursor=…`返回闭形`{project_id,profile_id,direction,items,next_cursor?}`。默认outgoing、limit20，显式limit范围1–100，cursor最长2048。没有fake total。固定九位UTC时间与ID形成稳定keyset；游标绑定tenant/project/current actor/profile/direction/limit，不是跨请求一致快照。

列表最多100项，实际items另有8MiB分页预算；达到容量边界不能越过尚未返回候选，next_cursor基于最后实际返回项。历史中的“交接时快照”与当前配置资格分别表达；读取不恢复原写操作、不更新观测、不跟随派发或启用。

## 交互、CLI与未知结果

原Quick“调整配置”保存独立准确source快照，目标选择仍可编辑；确认显示旧→新、新建／复用、目标原状态及可选暂停旧项（默认false）。原全局ToolAction／ToolWriteRecovery持久原body/key；未知写入优先恢复，不由重新匹配或读history清除、改版本或换key。成功后分别查看旧、新和交接记录，启用继续明确使用原确认和CAS。

CLI `tools replace --id OLD --file JSON|- --confirm`使用同一正式POST，不自动生成key、预读替换版本、自动重试或追加GET。2xx回执不匹配或传输未知保留原意图，手工原操作恢复；确定4xx保留原服务错误。`tools replacements --id PROFILE`只读分页历史，默认JSON，不把快照视为当前健康。中英文界面、帮助、失败与冲突提示保持一致。
