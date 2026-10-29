# Personal assignment inbox

Stage T-INBOX-138, initial product1.0.0. Owner: collaboration/integration. [Contract](../../contracts/notification-inbox.md), [architecture](../architecture/notification-inbox.md), [interaction and AI SOP](../design/notification-inbox.md).

Development → Inbox records real task-assignment events for the current human account. Active canonical human-resource bindings determine recipients; labels, email guesses, agents and tools do not. Creating or changing an assignment projects its notification in the same business transaction. Self-assignment remains an actual event.

Inbox, Unread and Archived are distinct filters with current server counts. Viewing a task does not mark it read. Explicit Read/Unread/Archive/Restore actions affect only the current person's notification; project viewers do not gain task-write or approval authority.

Task titles come from currently authorized source tasks, never cached notification content. Unavailable sources have no title or link. Old assignment events remain personal history and do not imply current ownership. Immutable sequence strings support bounded paging without losing BIGINT precision.

Version conflicts require an explicit reread. Unknown write results retain the original operation and expected version for recovery. Scope changes and current authorization failures clear protected reading data, while another filter cannot discard an unresolved command. Chinese and English use the existing design system, keyboard access and narrow-screen layout.

This stage covers task assignments only. Mentions, review requests, alerts, external-channel delivery, notification preferences, cross-project aggregation and real distributed/database-scale acceptance remain in the full platform plan.
