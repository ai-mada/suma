# Human answers during an original agent turn

Status: implemented for suma1.0.0; owners: control plane, Runner, Web; requirement: T-TOOL-INPUTS-187. [Contract](../../contracts/tool-inputs.md), [architecture and AI SOP](../architecture/tool-inputs.md), [verification](../quality/tool-inputs-verification.md).

An explicitly enabled local Codex connection can present one to three native questions in the existing run node. Users select original options, provide supported text, or explicitly skip. Answers supplement the original turn and never grant command, filesystem, dispatch, merge or production authority.

The native Default-mode feature remains experimental and is disabled by default. Enabling it only changes the attempt's isolated configuration. Public capability is advertised after supported local probing and is bound to the exact profile. Secret questions are rejected before persistence; ordinary settings remain the credential path.

The platform checks current browser authority, original execution identity, version, deadline and native closure before saving an answer with its receipt. Concurrent answers use transactional CAS. Recovering the original receipt does not authorize a fresh native write. Runner checkpoints separately record answer observation, write intent, write start, complete write and native closure. Unknown writes are reconciled without automatic re-answering or restarting the process.

Both ordinary and Pool clients replay original observations before results. Approval and question IDs share one typed native registry and a combined32-request limit, but their domain contracts and choices remain separate. Evidence contains identities and delivery facts, not question or answer bodies.

The node panel distinguishes saved, observed, writing, written and closed states. A nonblocking question does not pause the task, and native cleanup is not a human answer. Original-scope recovery survives a new attempt while preventing an old attempt from issuing a new answer. Chinese and English labels share the same behavior.

Focused protocol fixtures, real HTTP/SQLite transactions, controlled native CLI/loopback observations and representative synthetic UI checks are reported separately. They do not establish production-provider, physical distributed deployment or full-platform acceptance. The original31 requirements and P0–P7 remain active.
