# Shared tool onboarding design

Version 1.0.0 · T-SHARED-TOOL-ONBOARDING-219. See the [domain contract](../../contracts/shared-tool-onboarding.md) and [Chinese product guide](../product/shared-tool-onboarding.md).

## A progressive primary path

Existing installation scans, connection hints and Quick advanced actions continue to use the tool preparation guide. For a shared Worker, the guide retains configuration generation and composition, and replaces the ordinary project identity section with a compact shared-runner prompt. The selected protocol, tool, runner and project continue to the owner workspace. Missing current context or callback requires a new check; it never falls back to ordinary identity preparation.

The shared prompt and Pool card retain responsive layouts, 44px actions and Chinese/English states that explain the next step. The chosen configuration output path can be copied explicitly. The page neither reads remote files nor transfers private paths, and copying does not establish execution. Users must generate and verify files on the target machine and choose the original connection mode.

## Two precise navigation purposes

| Purpose | Source and return |
| --- | --- |
| New tool onboarding | An independent eight-field setup pin fixes Pool/Runner/Worker, tenant/project and exact adapter ID/adapter/protocol. It has no original profile fields. The project rechecks its catalog and shared runner before Quick selects the tool and Worker, without a replacement source. |
| Existing profile recovery | The original recovery pin retains tool ID, version and SHA. It continues through diagnostics, uncertain-operation reconciliation and replacement without inventing a new-onboarding profile. |

These are distinct current purposes, not product compatibility branches. A URL grants no identity, project access or Pool authority. An explicit project change uses the existing generic project-tools entry and drops the original source.

## Modules and currentness

- **ToolSetupGuide** identifies shared targets and selects tools only from the fixed preparation catalog. Existing local configuration generators remain unchanged. A source callback binds caller, project, Worker, protocol selection and monotonic generation.
- **ToolWorkspace and App** re-read the current catalog and shared runner before handing over the public setup pin. Every asynchronous return rechecks its source. Original unconfirmed operations retain priority.
- **PoolToolRecoveryPanel and PoolWorkspace** reuse exact Runner reads, original readiness and current owner permissions. Absence from the first collection page is not absence from the system. Owner navigation does not read a consuming project's private records.
- **The original preparation form** only extends its valid-target predicate. Approved-Scope checks stay intact. Its conservative local 30-second snapshot subtracts request latency and respects credential expiry. Expiry retains draft inputs, disables adoption and requires an explicit refresh; retained inputs do not retain expired authority.
- **The original connection component** offers a project link only when current `can_open_project` allows it. The same setup Scope retains the selected tool/Worker pin; another Scope returns through generic onboarding.

Changes to actor, API, runner, selection or source invalidate old responses and clipboard acknowledgements. A→B→A cannot revive them. Registration still requires complete fresh inventory, original matches and a stable operation key. New profiles start paused. Enablement, task dispatch and quality acceptance retain separate gates.

A current Pool owner may prepare an exact approved foreign-tenant Scope offline. `can_open_project` controls the return link only; preparation does not read consuming-project data or grant execution rights. An explicit 401/403/404 revokes the matching preparation context and unmounts its private form. This matches the current pin/generation only, leaving other dialogs and original write receipts intact. Expiry and transient network errors retain same-source inputs with actions disabled.

## AI SOP and verification limits

Reconcile original uncertain operations first. Then verify shared-identity routing, current public context, exact owner/Scope and the return to Quick. Do not guess remote paths or turn installation metadata into a formal tool profile to reduce input. Reuse evidence for unchanged maintenance, stop receipts, configuration writers and execution kernels.

This stage adds no backend endpoint, table, scanner or dispatcher. Keep focused component checks and production components over synthetic interface data distinct from real target-machine repair, vendor/model execution and complete platform acceptance. All 31 requirements and P0–P7 remain active.

A successful response that explicitly reports a revoked grant, runner or credential also retires the exact old preparation form. Expiry, slow reads, network uncertainty and project visibility remain separate conditions.
