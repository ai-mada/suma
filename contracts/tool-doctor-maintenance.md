# Doctor：准确维护下一步

状态：T-TOOL-DOCTOR-199当前合同；suma1.0.0；完整31／P0–P7目标继续。[原工具管理](tool-management.md)、[准确重检](tool-reinspection.md)、[未知核对](tool-recovery.md)保持唯一执行与授权语义。

## 当前读

原`GET /tool-profiles/{id}/diagnostics?project_id=…`在同一当前授权读事务内返回必需`maintenance_resolution`。原四项`checks`、`next_action`与`tool.available`继续描述派发前提；维护建议独立描述节点维护槽位和原操作，不是新的健康、安装、修复、授权或完成事实。

| 字段 | 约束 |
| --- | --- |
| `profile_id`、`profile_version`、`worker_id` | 准确等于当前`tool.id/version/worker_id`，版本为正整数 |
| `action`、`reason` | 下表固定组合；未知值或多余字段拒绝 |
| `inspection_id` | 仅定位原检查，非空时沿原identifier协议、最长150字符 |
| `recovery_id` | 仅定位原核对；存在时必须有原`inspection_id` |

| action | reason | 原操作标识 |
| --- | --- | --- |
| `inspect` | `maintenance_supported` | 两ID均省略 |
| `view_inspection` | `inspection_pending`／`inspection_claimed`／`inspection_unknown` | 必需inspection，省略recovery |
| `recover_inspection` | `inspection_unknown`／`recovery_expired` | 必需inspection，省略recovery |
| `view_recovery` | `recovery_requested`／`recovery_proof_stored` | 两ID均必需 |
| `open_worker` | `maintenance_expired`／`maintenance_unsupported`／`maintenance_unknown`／`worker_unavailable` | 两ID均省略 |
| `none` | `profile_revoked` | 两ID均省略 |

JSON中的可选ID不能显式为空或null。所有观察沿父`checked_at/expires_at`，截止不晚于相关未来的维护能力期限、pending期限、claim租约或活核对期限；过期时UI重新读取，不能沿浏览器墙钟续期。原期限已过不能变成未来有效期。

## 决策与准确来源

同节点未结束检查优先于新检查。使用既有tenant/project/worker索引读取原槽位，存在矛盾的多个占用或损坏来源明确失败，不将错误解释为空。pending等待原记录；claimed租约失效投影unknown；未领取pending到期可由下一次明确创建处理。GET不落状态、不创建操作、探测、核对或新资源。

unknown先读取准确原recovery：requested／proof_stored查看原进度，proof_stored不释放占用；过期或无核对且原owner的协议能力仍允许时，建议原明确恢复确认。能力不满足时仍保留原unknown并展示原检查。无占用才根据当前节点、维护能力和撤销事实决定新检查或配置入口。

槽位属于worker，原检查可能属于同项目的另一配置。建议中的`profile_id`始终指当前被诊断配置，`inspection_id`始终指真实占用；消费者必须先GET原inspection，再读取其原配置、核对同项目／worker后准确导航，不可把另一配置的检查挂到当前配置下。UI未决的本机原写意图与回执优先，不因新建议改body／operationID或重发新probe。

## 交互与写边界

维护建议提供一个清晰主入口，原四项资格可展开；技术依据默认收起。原检查／核对导航可先读原对象，实际Create、原核对、存储proof及ACK仍沿原权限、purpose、预期版本、原Ref、fence、幂等键和当前机器身份。账号、组织、项目、API caller或来源变化立即清旧建议并隔离迟到响应。

CLI`tools doctor --format json|table`默认保留JSON，table独立显示双语维护建议、准确目标和原ID。命令只GET，不跟随建议发起写入。前端、CLI和OpenAPI使用同一Go类型；源片段、原回执与业务验收分别取证。
