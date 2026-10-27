# Model selection and fixed execution workflows

Applies to **1.0.0**. This guide describes current workbench behavior and capability limits. [中文](../product/model-selection-workflows.md). The full platform scope remains in the [implementation plan](../delivery/implementation-plan.md) and [current status](../delivery/status.md).

## Entry points and choices

| Entry point | Current support | Result |
| --- | --- | --- |
| Ordinary task dispatch | Native Suma tools supporting independent model selection; `codex-app-server-v1` tools | A fixed task preview, followed by separate confirmation to start |
| Expert team dispatch and unstarted-item revisions | Native or Codex execution selected separately for each task | A choice saved per task; role sharing and task overrides are input conveniences |
| Conflict repair creation and revision | Native Suma tools with governed repair capability, including native model configuration | A preview for a separate repair task, or a new revision for the same repair task |

Options come from valid inventory for the current project and exact tool version and digest. Reading options observes the existing catalog without calling a model. It does not establish service health or vendor account authorization. Other tools that configure their own models retain their stated limits; a tool name does not establish support for platform model selection.

For an ordinary task, select the workflow and tool, then choose the model explicitly. Native execution selects an exact deployment revision, maximum model calls, cumulative output-token allowance and optional budget. Codex execution selects a model and reasoning effort supported by its current catalog, plus an optional token notice threshold. A task uses one execution binding; it cannot submit both sets of settings.

For an expert team, assign each task a role, then choose models per role. Enable a task override when it needs different settings; clearing the override adopts the current role choice again. Each item saves its own fixed choice, which later role edits cannot change. Native allowances are reserved and settled per task rather than pooled into one team-plan budget. Dependency waits use no execution slot.

## Fixed facts and explicit revisions

Preparing a preview fixes the task and configuration versions, tool, model choice and related digests. Native bindings also retain allowed parameters, budget and a matching operator rate. Codex bindings retain the catalog and configuration digests, model, reasoning effort and notice threshold. The preview and execution node must carry the same binding. Viewing the preview, changing language or refreshing the page does not start execution.

Review these facts before confirming Start. Preparation, Start and Claim check the relevant current authority, inventory and original plan basis. Failed checks preserve the original facts and block new dispatch. They do not silently change models, adopt a newer quote or fall back to environment configuration.

To change an ordinary task's model, prepare a new preview. An expert plan requires an explicit revision of selected unstarted items and a fresh model choice; started definitions and their original runs remain intact. Conflict repair first creates a separate task from complete conflict evidence. A repair revision keeps that same repair task and fixed sources while creating a new preview and retaining earlier plans and runs. Revision is available only for a server-authorized unstarted plan or a failure certified to have occurred before startup entry. Preparation, revision, Start, independent testing and human acceptance are separate actions.

**Codex repair remains unsupported.** Governed repair currently requires a special native workspace. Codex tools are explicitly unavailable for repair creation and revision; this restriction cannot be presented as supported fixed-tool configuration.

## Native allowances and Codex notices

Native execution enforces platform call and cumulative output limits, with token/cost admission checks and reservations at Claim under organization and project policies. Reservations use the declared context bound and allowed call count. They are not exact token predictions or a guarantee of the vendor's final bill. Costs use the fixed operator rate; a missing matching rate remains unpriced, never free. Missing or partial usage and unknown outcomes retain known facts and reservations requiring reconciliation rather than recording zero.

Codex is fixed to **observe-only**. A token threshold produces a usage notice; it does not guarantee a stop, hard allowance or exact cost. Policies requiring enforced limits reject this tool rather than silently changing to observation. CLI configuration acknowledgement establishes that the settings were accepted; it does not attest the provider's actual model, model quality or vendor charges. Unknown costs remain unpriced.

## Original-operation recovery and verification limits

After a timeout, disconnected response or result that cannot identify the original task and binding, retain the complete original request and operation key and use the recovery action. HTTP 2xx alone cannot clear an unknown outcome. A mismatched model, reasoning effort, notice threshold, native parameter or workflow binding still requires recovery. Do not replace the key to create another task or start another run. Resolve the original operation before explicitly preparing a new preview or revision with changed settings.

The current GUI check is limited to catalog selection, fixed previews, unstarted-item revisions and refreshed readback in isolated fixtures on one local machine. Start was not clicked and no new model requests were made. This checks the interaction and persisted facts, not vendor inference, actual coding, physical multi-host operation, formal acceptance or production integration. Earlier execution evidence retains its own source and scope; this GUI check does not expand it.
