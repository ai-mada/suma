# Exact knowledge in task discussions

T-CONTEXT-145 frozen scope for initial product 1.0.0. The [domain contract](../../contracts/task-conversation-knowledge.md) owns the wire behavior. This document describes the design and does not certify implementation, browser, provider or production acceptance.

Users explicitly select published project memory, specifications or skills in the existing task composer. A message records exact references; a later, separately authorized reply resolves the approved text into its protected source. Opening history, posting a message, requesting a reply and starting development remain separate actions.

## Publication and history

The existing [project knowledge domain](../../contracts/project-knowledge.md) retains draft, publication, retirement and approval authority. A skill is reviewed text guidance, with no plugin installation or additional tool rights.

`TaskConversationAppendRequest.knowledge` optionally contains at most 16 existing `ProjectKnowledgeReference` values: `id`, `revision` and `sha256`, unique by ID. Unknown fields, explicit null, duplicates and caller-supplied trusted content or titles are rejected. The server captures `title_at_post` and `kind_at_post` from the approved revision. Message metadata contains references, not knowledge bodies.

The original append transaction checks current task write authority and recovers an existing operation before validating a new selection. A new message, immutable reference relations, activity and receipt commit together. Recovering the same completed operation does not repost, republish, re-resolve retired references or invoke a model. Current caller authority is still required.

GET `/project-knowledge/{id}/revisions/{revision}?project_id=…` returns the exact immutable revision under current project read authority, with its digest verified. It reads the requested revision directly, with no traversal or fallback to latest. History answers what was used; it does not authorize new consumption.

| Change | Historical record | New consumption |
| --- | --- | --- |
| A new draft is edited | The previous publication and message pin remain | Resolve the existing publication; do not expose the draft |
| Another revision is published | The old pin remains exact | Reject stale pins; require a new explicit selection |
| Knowledge retires or its formal source changes | Keep the old revision and execution facts | Reject current eligibility; never silently replace content |
| The caller loses project access | Do not rewrite history | Deny access and clear protected UI content |

## Assistant dependencies and source integrity

`taskConversationMessageMetadataTx` is the shared hydration path for history, exact messages, reply sources and development sources. Human/API messages use immutable reference relations. An assistant may inherit references only from its real completed backing Reply in the same tenant, project and task. Protected relations, MessageID, sequence, RunID and Source.SHA256 must match. A self-declared `reply_id` or a forged human relation is insufficient.

This preserves dependencies when the original human message falls outside the recent window but the assistant's paraphrase remains. Derivation uses the backing Reply's `Source.Knowledge`; it does not recursively copy nested message histories. Historical metadata is not replaced by current titles, and reading history does not require the original author to remain active. A new consumer must pass current authority and eligibility checks.

Across up to 20 recent messages, exact references are merged in message order. Identical pins deduplicate; conflicting revisions or digests for one ID fail explicitly. The union has at most 16 items. `resolvePublishedKnowledgeTx` supplies approved `ProjectKnowledgeContextItem` snapshots for optional `Reply.Source.Knowledge`. These participate in the original stable source serialization and SHA-256. No-selection fields remain omitted, preserving existing plain-message shapes.

Knowledge counts toward the existing 32,000-byte UTF-8 context and 64,000-byte final request bounds. Each knowledge body keeps its existing 16,000-byte bound. Excess content fails explicitly, with no silent truncation or summary. Token, cost, model window and actual usage remain separate facts controlled by the original model contract.

The original source check re-resolves exact references at creation/commit, controlled claim/heartbeat and publication boundaries. Republish, retirement, changed sources or revoked access can block further consumption or publication without erasing real run, usage or output facts. This does not promise atomic revocation for every byte of an external provider request. There is no new scheduler, default model fallback or tool authority.

```mermaid
sequenceDiagram
    actor User
    participant Discussion as Existing discussion
    participant Knowledge as Knowledge authority
    participant Reply as Existing reply execution
    participant Development as Existing dispatch and Start
    User->>Discussion: Post text and exact pins
    Discussion->>Discussion: Current authority; recover original receipt first
    Discussion->>Knowledge: Validate new selection
    Discussion->>Discussion: Commit message, refs, activity and receipt
    User->>Reply: Explicit window, model and budget
    Reply->>Discussion: Hydrate messages and verified assistant dependencies
    Reply->>Knowledge: Resolve current approved snapshots
    Reply->>Reply: Freeze source hash and enforce size bounds
    Reply->>Knowledge: Recheck at existing execution/publication gates
    Reply-->>User: Protected reply and original execution facts
    User->>Development: Review reply and create preview
    Development->>Knowledge: Forward exact Source.Knowledge refs
    User->>Development: Explicit Start
    Development->>Knowledge: Original Start checks current eligibility
```

Development forwards `knowledgeReferences(reply.Source.Knowledge)` through the existing `TaskDispatchPreviewRequest.Knowledge` and task context compiler. It does not merely append text to `reviewed_goal`. The user selects new development tools, model and budget; the original DAG, candidate capture, independent testing, formal Review and issue feedback remain authoritative.

## Interaction and AI SOP

The composer keeps knowledge selection collapsed by default and loads bounded search/pages only when expanded. Users select explicitly. Selected IDs hydrate separately; unavailable pins remain visible for removal, with no automatic upgrade. Message cards show the title at posting and exact revision, fetching historical bodies only on expansion. Reply and development panels display server-frozen sources rather than another editable selector.

Drafts store text, mentioned resource IDs and exact knowledge pins, without cached bodies or model credentials. Scope changes isolate late responses; authorization failures clear protected expanded content. Unknown writes retain the original caller, scope, version, operation ID and JSON. Recovery does not add newly selected fields. Chinese/English, narrow screens, themes, keyboard/focus and existing unsaved-change protection follow the original components.

The SOP is: review and publish knowledge; explicitly post exact pins; authorize one reply with a fixed window/model/budget; inspect the real reply and execution facts; create a reviewed development preview; explicitly Start through the original authority. Knowledge is untrusted project data and cannot grant commands, change models, bypass tests or approve its own delivery.

## Reference scope and evidence boundary

Local Multica reference is pinned to `2083970b`. Its exact source locations and limits are recorded in the [architecture source table](../architecture/task-conversation-knowledge.md#参考来源与采用边界):

- `server/internal/handler/project_resource.go` (`resolveClaimProjectContext`) and `server/internal/daemon/execenv/context.go` (`writeProjectResources`) share workspace-scoped claim context and materialize a project resource manifest. A manifest does not prove referenced bytes were read.
- `server/internal/daemon/types.go` (`ChatAttachmentMeta`) and `server/internal/daemon/prompt.go` list attachment metadata. `server/cmd/multica/cmd_attachment.go` (`runAttachmentDownload`) and `server/internal/cli/client.go` (`DownloadFile`) separately fetch actual bytes. Download does not prove model parsing or OCR.
- `server/internal/service/source_context.go` and `server/internal/handler/source_context.go` (`cloneSourceContext`) capture historical context and copy selected attachment objects. Such history is not Suma's current publication eligibility.

Multica's `LICENSE` combines Part I conditions with Part II Apache terms; it is not unrestricted Apache alone. This stage uses behavior/interface reference and an independent Suma implementation, with no source/UI copied from that revision. Previous licensed-source provenance remains separately recorded in [the existing provenance](../../third_party/multica-legacy/PROVENANCE.md).

Required evidence includes original receipt recovery, transaction rollback, publication/draft separation, stale or retired sources, exact history, revocation, corrupted backing relations, assistant-only windows, size limits and propagation into the real fixed request and original development Start. Protocol or component fixtures do not establish binary attachment parsing, OCR, vendor acceptance, installation, multiple physical hosts or production readiness.

[Product](../product/task-conversation-knowledge.md) · [Architecture](../architecture/task-conversation-knowledge.md) · [Interaction and AI SOP](../design/task-conversation-knowledge.md)
