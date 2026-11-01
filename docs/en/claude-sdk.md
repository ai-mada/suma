# Claude Agent SDK collaboration

Status: scoped implementation verified; vendor and production acceptance remain separate. suma1.0.0, T-CLAUDE-SDK-192. Owner: integration agent. See the [contract](../../contracts/claude-sdk.md), [architecture](../architecture/claude-sdk.md), [design](../design/claude-sdk.md) and [scoped verification](../quality/claude-sdk-verification.md).

The SDK integration extends the existing control plane and Runner with native permission callbacks and structured questions. The foreground Claude CLI and reverse MCP client remain distinct supported directions. The SDK does not create another dispatcher, database, acceptance authority or automatic approval policy.

Administrators select fixed Node, SDK-host and native-CLI paths, an exact model and an explicit API-key environment reference. The original preparation command creates a new reviewed configuration exclusively; copying or preparing it does not prove authentication, model health or dispatch readiness. Original reporting, fresh inventory, paused registration and explicit enablement remain separate actions.

Each decision binds the actual SDK session, native request ID, tool-use ID, local callback ID, configuration and exact input digest. Permissions allow or reject only that callback. Questions have no preselected answers, preserve original text and option labels, and never grant command permissions. Local host delivery is not a native acceptance acknowledgement.

The original current authorization, CAS, receipts, lease/fence and durable single writer remain authoritative. Unknown effects require reconciliation rather than replay. Local callback denials and the native final denial summary are separate facts; an absent summary stays unknown. Rejected or cancelled persisted decisions cannot be reported as successful delivery.

Code candidates, independent tests at the same commit and human acceptance remain mandatory independent gates. Controlled SDK/native-fixture evidence does not establish authenticated vendor execution, other operating-system containment, distributed scale or production acceptance. The original31-requirement/P0–P7 platform objective stays active.
