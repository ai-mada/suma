# 本地文件保存的工作区上下文合同

状态：T-WORKSPACE-109／1.0.0，当前实施合同。此合同限定已有本地 UTF-8 文件的读取、CAS 保存及原回执恢复；[桌面目录授权](desktop-workspace.md)、[远程执行保存](workspace-write.md)、完整 P0–P7 与需求1–25继续有效。验证结果由主集成证据单独记录。

## 公共类型与快照

前缀 `/api/v1`，JSON 使用 snake_case。`GET /workspace/file?project_id=…&path=…` 与成功的 `POST /workspace/file` 都返回 Go 公共类型 `WorkspaceFileDocument`，精确包含七个必需字符串：`project_id,path,content,hash,language,workspace_name,context_sha256`。`hash` 是完整文件内容的 SHA-256；`workspace_name` 仅用于显示；`context_sha256` 是不透明的64位小写摘要，不是权限凭据。

控制面从同一当前授权读取中的实际已打开根目录生成上下文，绑定租户、项目、每次正式绑定生成的新 `binding_id` 及该目录的稳定 OS 身份。目录名或文件内容相同不代表上下文相同；A→B→A再次绑定必须改变上下文，即使两次A都是同一个目录。相同持久绑定、相同目录身份及正常重启保留上下文。目录被同路径新目录替换时不得沿用旧上下文。

客户端把上下文与原项目、路径、内容摘要及草稿一起保留。不能从当前工作区元数据、当前目录名、新读取的其他文件或“内容仍相同”推断、补齐旧草稿的上下文。账户、组织或项目切换、迟到响应及重新绑定不能把旧草稿解释成新上下文的内容。

## 保存请求与事务顺序

`POST /workspace/file` 的 `WorkspaceFileSaveRequest` 精确包含六个必需字符串：`project_id,path,content,expected_hash,expected_context_sha256,idempotency_key`；未知字段无效，POST范围只来自正文。上下文须复制原文件GET的 `context_sha256`，`expected_hash` 须为原文件完整摘要。新旧文本至多1MiB，保持既有敏感路径、链接及根限制；操作标识长度为8–128。

每次交互保存和回执读取仍检查原调用凭据的当前项目写授权。控制面在项目锁内先查原操作：

1. 已完成且请求摘要相同：直接恢复持久的原成功文档，包含原 `context_sha256`，不根据当前根重新生成回执，不再次写文件。
2. 同操作标识而请求摘要不同：`409 idempotency_conflict`，不重解释原操作。
3. 已受理、仍为prepared：只允许完成持久的原root、path、上下文和新旧内容；根身份或上下文不再可证明时保持待核对。
4. 原受理记录与journal均不存在后，查原拒绝回执。同一准确请求恢复原拒绝；同标识不同请求仍冲突，不能复用已拒绝的操作标识。
5. 锁内确认该操作既未受理也未被拒绝后，才验证一个新请求并比较实际打开根的上下文及原内容摘要。上下文不符时持久记录终态拒绝，提交成功后才能返回未受理证明；不创建受理记录或改变文件。全部检查通过后才原子持久化prepared／accepted身份，再沿既有恢复路径完成准确文件效果。

## 错误与未受理证明

缺失、非字符串、空值、非64位或非小写十六进制 `expected_context_sha256` 返回 `400 invalid_input`，显示键 `workspace.save.context_required`。其余封闭正文格式错误使用同代码及 `workspace.save.invalid_request`。上下文匹配而原文件摘要不同继续返回 `409 version_conflict`。

只有在当前授权与项目锁内查明原操作不存在，并发现实际已打开根上下文不同，才返回 `409 workspace_context_changed`、键 `workspace.save.context_changed`，业务 `details` 必须精确为：

```json
{
  "project_id": "原项目",
  "path": "原相对路径",
  "operation_id": "原幂等标识",
  "expected_context_sha256": "原64位摘要",
  "not_accepted": true
}
```

这证明该准确原操作未被受理且不会在以后被重新解释为新保存。控制面在项目锁内持久化原调用者绑定的请求摘要及原拒绝身份，并先提交拒绝回执；提交失败不得发出上述证明。客户端可明确保存草稿、重新读取并由用户处理一个新操作。不能把网络失败、任意409、根路径相同、prepared或当前文件恰好等于提交内容当作该证明。

拒绝回执必须先于以后任何新上下文检查恢复。即使绑定ID没变，物理目录A被移走、同路径目录B触发拒绝、后来原A又放回，迟到的同一保存报文仍恢复原拒绝，不会写入A。丢失拒绝响应后的重试也保持原证明；更换内容或上下文必须使用新的操作标识。该拒绝不表示工作区中的其他操作也未受理。

prepared的根上下文发生变化时返回 `409 workspace_reconciliation_required`；不带 `not_accepted:true`。当前原生目录证明无效仍按既有权限错误处理，不能降级到普通配置根，也不能据此认定旧操作未受理。显示语言只影响成对中英消息；业务错误代码、上述details、原请求和持久回执不翻译。

## 初始化与边界

新库的 `suma_workspace_bindings` 持久保存新 `binding_id`；文件操作journal持久保存原上下文；独立拒绝操作表持久保存未受理回执，不与prepared／accepted账本混用。成功、受理及拒绝回执须在承诺重试期间保留，不能清除后允许旧报文变成新写入。仅维护当前1.0.0新库结构，不提供旧字段别名、迁移、回填或缺失上下文自动补齐。普通配置根绑定与桌面绑定都先核验当前授权；原准确操作的幂等回放恢复不可变原ACK，不生成新binding身份、不重新检查或采用原目录，也不覆盖后来已提交的绑定。只有正式新绑定生成新身份。

保存仅代表该文件操作的效果和回执，不自动证明测试通过、任务验收、候选封存、部署或完整IDE完成。创建／删除文件、强制覆盖、终端、LSP／DAP、远程工作区与完整需求23仍按各自合同实施。

## English contract notes

The typed file document carries an opaque context bound to tenant, project, fresh binding identity and the actual opened root's OS identity. Save must retain the original document's context and hash. Equal bytes, the same displayed name, or an A→B→A binding sequence cannot authorize a stale draft.

A completed exact operation replays its immutable original document before current-root checks. Ordinary configured-root and desktop binding retries also restore their original acknowledgement under current authority without rotating the binding identity, rechecking the old directory, or replacing a later binding. Only an absent file operation established under current authorization and the project lock can produce `workspace_context_changed` with the exact `not_accepted:true` detail object above, after committing a terminal rejection receipt. Future exact retries replay that rejection even if the original physical root returns at the same path with the same binding identity. Different content or context needs a new operation ID. A prepared operation whose original context cannot be recovered remains reconciliation-required. No migration, alias or automatic context fallback is defined.
