# Unified input and draft plans

Scope: T-INTENT-114, suma 1.0.0. One natural-language goal produces an editable plan for requirement, design and project-knowledge drafts. Requirement 26 remains in progress. See the [bounded verification](../quality/intelligent-operations-verification.md), [contract — Chinese original](../../contracts/intelligent-operations.md), [architecture and acceptance SOP](../architecture/intelligent-operations.md) and [中文指南](../development/intelligent-operations.md).

## Generate a plan

Open intelligent input in the current project and describe the drafts you need. For example: “Add classification and acceptance criteria for issue handling, propose product, experience and technical designs, and prepare a project specification draft.” Include necessary facts; keep passwords, tokens, private keys and credential-bearing addresses out of the input.

Optionally select 0–8 current project documents, requirements, tasks or issues. Select the existing requirement explicitly when its design should change. The server freezes actual source versions and excerpts; the model cannot retrieve other sources. Reduce the selection or choose an excerpt when it exceeds the context limit. Acceptance criteria are never silently truncated.

Select an available, dedicated read-only intelligent-operations tool. Review its fixed model and endpoint, set the output allowance and explicitly acknowledge sending this goal and the selected content to that model. Each analysis makes one model call. A successful tool probe or enabled configuration does not establish model-account health. Follow tool settings when no eligible tool is available.

Keep the analysis and real run reference after submission. Running, failed and reconciling are distinct states. A successful model run does not mean the proposal is valid or any business draft has been created. Refresh the same analysis instead of submitting another model call to discover its outcome.

## Review and preview

A plan has at most 8 steps. It can create a requirement draft, save a requirement-design draft or create a memory, specification or skill draft. A design can reference a selected existing requirement or the real result of an earlier requirement-creation step. Proposed design tasks remain proposals.

Review the summary, rationale, dependencies, content, criteria and knowledge sources. Original model output and human revisions remain separate. Resolve listed questions by supplying the missing facts and explicitly revising the proposal. Unresolved questions block application. Host labels support Chinese and English; goals, sources and model text retain their original language.

Save revisions and request a preview. Review every target, current version, draft effect, required permission and default-value source. Previewing creates no business resources. Editing the proposal or changing scope invalidates the previous preview and acknowledgement. Changed sources remain available as historical evidence but block application of the stale proposal.

## Apply this draft batch

Explicitly acknowledge the reviewed preview and apply it. This authorizes only this draft batch; it does not enable an ongoing automatic-approval policy. The server rechecks authority, sources and target versions in one database transaction. All local draft writes commit together, or all roll back.

Open the real requirement, design or knowledge draft from the returned results. Results preserve the IDs, versions, revisions and provenance at application time. Later edits do not rewrite that evidence. Publishing requirements or designs, adopting formal tasks, starting development, publishing knowledge and human acceptance remain separate domain actions.

## Conflicts and recovery

| Situation | Next action |
| --- | --- |
| Generation fails or returns invalid output | Inspect the real run and diagnostic; retain the original evidence; no automatic model replay |
| A source or target version changes | Keep edited content, review the current basis, revise explicitly and obtain a new preview |
| Application times out or its outcome is unknown | Retain the original account, project, method, path, body, expected version and operation key; recover that operation and its original resource IDs |
| Access is revoked or the session expires | Restore valid current access; even a successful historical receipt requires current authorization |
| A running analysis needs cancellation | Use the existing commands on its actual run; dismissing a proposal cannot prove the process stopped |
| A resulting draft has changed since application | Keep the original result as history; read the current version for the next edit |

This stage applies local database drafts atomically. It does not execute code, commands, configuration changes, deletion, external messages or deployments. General configuration automation, complete CLI and desktop intelligent journeys, and durable automatic-adoption policies remain in the [full implementation plan](../delivery/implementation-plan.md).
