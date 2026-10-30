# Follow new discussions on a task

T-MULTICA-FOLLOW-154, suma 1.0.0. Status: the scoped implementation has actual HTTP and GUI evidence on isolated SQLite and Windows Electron software offscreen; full platform acceptance remains open. The [frozen contract](../../contracts/task-subscriptions.md) owns the interface; see the [product scope](../product/task-subscriptions.md), [architecture](../architecture/task-subscriptions.md) and [interaction and AI SOP](../design/task-subscriptions.md).

## Personal preference

A person explicitly follows or unfollows the current task beside its discussion entry. New discussions then appear in the existing personal inbox and open the exact original message. Unfollowing affects future message commits and leaves existing notifications intact.

The browser and an authorized current-user API manage only the authenticated person's preference. A project viewer can manage this preference using current task read access; following grants no task write, posting, execution or Review authority. Runner, Agent, Pool and requests acting for somebody else are rejected. Ownership, team membership, past mentions, page loading and reconnects do not create subscriptions. Other followers are not listed.

`GET /api/v1/tasks/{task}/subscription?project_id=P` returns TaskSubscription with project_id, task_id, user_id, version, following and updated_at. With no stored row, the result is version0, following=false and an empty updated_at. This first read creates no preference, message, notification or audit record.

`POST /api/v1/tasks/{task}/subscription/commands?project_id=P` accepts only project_id, task_id, expected_version, action and idempotency_key. Action is follow or unfollow. Path, query and body scope must agree; the authenticated user cannot be overridden. Unknown or duplicate fields, nulls, noncanonical identifiers and invalid numeric forms are rejected.

Commands use CAS on the current person's preference, independently of the task's business version. Only a real state change increments the version; an already satisfied target does not create an empty update. Unfollowing an untouched task can return a side-effect-free version0 receipt. A stored row remains after unfollowing; following again retains its identity and increasing version.

## Original operation and current state

TaskSubscriptionReceipt contains idempotency_key and subscription. The original key is bound to the original body. An exact replay restores its receipt after current authorization is rechecked; another body cannot reuse that key. Recovery precedes a new command's version conflict check. A historical receipt confirms its original submission; current state is read separately.

The interface uses the existing useWriteAction flow. It saves the original caller, identity/task scope, version, body and key before sending. Unavailable storage blocks transmission. An unknown outcome locks further changes and offers explicit reconciliation of that same operation. Refresh, a matching current following value or a new GET cannot confirm the unknown write. A conflict never automatically adopts a new version or sends a new key.

Changing user, organization, project, task or API caller retires old reads and isolates late responses. A current 401/403/404 clears protected content. A failed refresh after a confirmed receipt is reported separately and does not justify resending the command.

## Notifications from the original message transaction

The recipient set includes every person who is following at message commit and still has current project access. It is not silently limited to an inbox page. Following does not authorize access by itself, and opening a page or reading the inbox never backfills old messages.

| Committed source or recipient | Result |
| --- | --- |
| A human or current-user API message, for a current eligible follower | task_followed in the original message transaction |
| The human/API author is also following | No additional follow notification to the author |
| The same message explicitly mentions that recipient | Existing task_mentioned takes priority; no extra task_followed |
| A final assistant message with its complete original reply and result binding | Same-transaction follow notifications, including an eligible following requester |
| Streaming fragments, execution progress, tool logs, tests or acceptance status | No discussion-follow notification |

Existing self-mention behavior remains. Human/API posting and final assistant persistence share an internal projection core and the inbox's existing sequence and unique insertion authority. One source message/subscription/account is projected once. Message or final reply, notifications, private source relations and original receipt commit together; a projection failure rolls back the original transaction. Replaying the original operation creates no second event. There is no new arbitrary notification API or separate message scheduler.

The implementation must read and validate the full eligible recipient set using existing indexes and set operations. It must not acquire fresh identity locks per recipient in an order that reverses the original authorization lock order.

## Private source binding and current access

The logical preference is unique to tenant/project/task/user. A private source relation binds its private row ID to the exact message, recipient, scope, original task version, reverse notification ID and deterministic event digest. The notification uses that private preference row ID as source_resource_id. Public subscription DTOs and receipts omit private IDs and reverse bindings.

Notifications contain references rather than discussion text, task titles or model-output snapshots. The existing inbox returns message_id and the current task only when current task access and every source binding agree. Assistant sources also require the complete original reply binding. A missing or corrupt source retains the private notification and its lifecycle but exposes no body or task navigation. Database execution errors remain errors rather than being disguised as an unavailable source.

Later unfollowing does not invalidate a past source. Current project revocation still denies the inbox scope. Opening the exact message is a read operation, separate from marking the notification read, archiving, dispatching or making a Review decision.

New schema components follow the single 1.0.0 initial architecture and existing SQLite/OceanBase/PostgreSQL dialects. Complete domain validation runs before any new DDL. Incomplete existing structures are rejected without backfills, resets or deletion of user data. The [architecture diagram](../architecture/task-subscriptions.md#原消息事务中的投影) records source and data ownership without duplicating physical DDL.

## One Bell control and the existing inbox

| State | Label and interaction |
| --- | --- |
| No successful read yet | Loading follow status; disabled, without pretending to be unfollowed |
| Confirmed not following | Follow task; aria-pressed=false; explicit follow action |
| Confirmed following | Following; aria-pressed=true; explicit unfollow action |
| Saving | Saving; synchronous repeat-click protection |
| Unknown write | Original operation unconfirmed; reconcile the saved operation |
| Read error or version conflict | Visible explanation and explicit reread; no automatic write |

Use one Lucide Bell button beside the current discussion entry, with existing styles and no extra configuration panel. Chinese “关注任务／已关注” matches “Follow task／Following”. Text, pressed state and a visible focus ring convey the confirmed state. Keyboard Enter/Space, at least 44px touch targets, wrapping at 390px, readable errors and reduced motion follow existing conventions. Keep focus after a confirmed action; do not optimistically announce success.

The existing inbox labels task_followed as “New discussion from a followed task”. Open discussion uses `panel=workbench-task:<task_id>` and `conversation_message=<message_id>`, directly reads and focuses the exact message, and preserves editor state and drafts. It does not traverse all history, duplicate an already visible message or mark the event read. Unavailable sources have no active link.

AI may explain a preference or an original-operation recovery. It cannot infer consent from ownership or history, manage another person's preference, or treat notifications as proof of development, testing, acceptance or release success.

## Reference and remaining verification

The second slice in the [Multica priority map](../integrations/multica-priority-map.md) is a behavior reference. This read-only inspection used local Git objects at `890628511e423864d21c694231277805ce9f113c`; local HEAD and FETCH_HEAD both resolve there. The prior local4157 pin is historical. The subscriber.go blob `9729d861a0c514cb2a7ba23761b76e0e5d961b18` and hook blob `17c3070ef0fa6b27241a81c54d685796972e41ad` are unchanged across 890628, prior4157 and the older fe23 investigation. Exact pins, links and scope are in the [architecture reference](../architecture/task-subscriptions.md#multica准确参考与选择).

The hook's successful-read gate and synchronous toggle serialization inform the interaction. Its handler still permits target user/user-type overrides; suma deliberately keeps the frozen self-only boundary. No reference source, style, icon or asset is copied. The pinned full license and existing adoption boundaries remain in force; no fetch or checkout was performed.

The [actual receipt](../../dist/quality/task-subscriptions-154/receipt.json) records16 distinct checks on Electron35.7.5 with production preload and isolated SQLite. Normal HTTP setup and invitation acceptance created two real users, including a project viewer, without direct database writes. The journey covers keyboard follow, another user's formal message, exact-message focus without a read mark, explicit unfollow with no later event, refollow and mention priority, committed-response loss and exact-body/key recovery, a second-task scope change and current revocation. Four preference commands used five transports and ended at version4; three messages produced two notifications, with no execution or model calls.

The [manifest](../../dist/quality/task-subscriptions-154/artifact-manifest.json) pins the candidate and three inspected images: [Chinese1366](../../dist/quality/task-subscriptions-154/followed-zh-1366.png), [English390 inbox](../../dist/quality/task-subscriptions-154/inbox-en-390.png) and [English390 recovered keyboard focus](../../dist/quality/task-subscriptions-154/recovered-en-390.png). The response loss was a real upstream200 commit followed by intercepted response abort, not a mocked successful write. The original request and first-run checkpoint remain private in the cache.

No assistant was invoked or fabricated in this GUI journey. Final assistant projection, full recipient sets, rollback, source integrity and other domain checks are recorded separately by integration; focused suites and builds were not repeated here. Other databases, a visible OS window, physical devices, full theme/touch coverage, multihost, scale and production remain unverified by this evidence. Email/IM, additional channels, cross-project aggregation and noise preferences remain in the original plan. Full P0–P7, requirements1–26 and 58 limited /34 pending /92 total are unchanged.
