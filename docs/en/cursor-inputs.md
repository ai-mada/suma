# Questions and plan confirmation in the original execution

Status: scoped implementation verified; vendor and production acceptance remain separate. suma1.0.0, T-CURSOR-INPUTS-190. Owner: integration agent. See the [contract](../../contracts/cursor-inputs.md), [architecture](../architecture/cursor-inputs.md), [interaction design](../design/cursor-inputs.md) and [scoped evidence](../quality/cursor-inputs-verification.md). The complete31-requirement/P0–P7 goal remains active.

The existing node input panel handles real Codex questions and Cursor ACP questions or plans. Each source keeps its own native identifiers and response vocabulary. Cursor uses exact option IDs, supports single or multiple selection, and offers whole-request skip/cancel. It has no native free-text, Other or secret-answer controls.

Plan confirmation displays the complete original plan and bounded todo/phase lists. Accept, reject and cancel require an explicit current project writer decision, original input ID, expected version and stable operation ID. Acceptance may let the native tool save a document under this attempt's private HOME. It grants no command permission and changes no formal task, requirement, baseline, quality result or deployment approval.

The original control plane, tables, ordinary/Pool assignment, fencing, receipts and Journal are reused. Source identity binds the real session, original Prompt request ID, server request ID, method, configuration and normalized complete payload digest. The answer digest also binds that payload, preventing a constant acceptance response from authorizing another plan.

Saved, observed, write-started, fully-written and execution-closed facts remain distinct. Cursor does not produce a Codex request-resolved acknowledgement. Unknown delivery reconciles the original intent; it never silently creates another session, Prompt or response. A declined plan uses a normal typed result because a protocol error can trigger the native tool's file fallback.

Limits are16 questions,32 options each,128KiB of plan text and256KiB of normalized aggregate payload, with bounded todos/phases and the existing combined32-request interaction cap. Waiting stays within the original live process and deadline; this stage does not claim released execution slots or persistent old-session recovery.

Focused protocol/control-plane checks and representative bilingual GUI evidence are separate from real provider-account, model, installation, distributed-scale and production acceptance. Evidence states what was actually exercised.
