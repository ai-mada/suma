# Explicit mentions in task discussions

Status: T-MENTIONS-140 implementation baseline. The [contract](../../contracts/task-conversation-mentions.md) defines the current scope. Verification and browser acceptance are recorded separately.

Write a plain-text message and explicitly select people beneath the composer. Typing `@name` alone is ordinary text. Up to20 distinct human resource IDs can be selected; this feature does not broadcast to teams, agents or all members.

The directory shows active, canonically bound people who currently have read access to the selected project. Project viewers can receive mentions. Search and pagination cover the current authorized directory. Selected people are rechecked when sending; one invalid recipient rejects the whole message rather than silently dropping that person.

The original message, ordered mention references, private inbox events and receipt commit together. Failed delivery rolls back the message and does not consume an inbox sequence. Mentioning yourself records the actual event. Posting a mention does not start a model, run, approval or acceptance action.

History stores the server-resolved resource ID, user ID and name at posting time. Later renaming or revocation does not rewrite the message or its frozen assistant context. The current directory remains separate: a saved draft rechecks selected IDs and marks unavailable selections explicitly.

An inbox mention opens the exact discussion message in the task workbench, including messages older than the latest20. Opening it does not mark it read automatically. An unavailable source retains the personal event but has no discussion link or cached message body.

Keep the original account, organization, project, task, version, operation ID, body and recipients when recovering an unconfirmed send. Confirmed success clears the draft. Pending recovery and unsent content retain the existing navigation guard.

Only explicit human/user-API posts create mention relationships. Protected assistant output remains a bounded plain-text reply. Teams, broadcast, notification preferences, email/IM delivery and cross-project aggregation remain planned work.

The dedicated `discussion` CLI supports `list --task`, `people --task`, `message MESSAGE --task`, and `post --file JSON|-`. Posting preserves the explicit original project, task version, operation ID, body and ordered recipient IDs without adding query parameters. It does not change the generic API client. Use the existing `inbox` commands for personal read/archive state. Unconfirmed sends must reuse the original request exactly.

The current Multica checkout informed behavior only. Its combined license includes hosted-service, commercial-embedding and branding restrictions; no restricted implementation or assets were copied for this feature.
