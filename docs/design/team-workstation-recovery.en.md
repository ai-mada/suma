# Shared workstation recovery design

Version 1.0.0 · T-TEAM-WORKSTATION-RECOVERY-218. See the [domain contract](../../contracts/team-workstation-recovery.md) and [Chinese product guide](../product/team-workstation-recovery.md). The complete 31 requirements and P0–P7 remain active.

## A single primary path

The original tool actions and Doctor remain in project tool details. A shared runner offers **Manage shared runner** only while its exact source is current. The owner workspace shows a compact recovery card, then opens the original preparation dialog and connection guide after an explicit choice. It adds no second runner console or dispatcher.

The recovery card and new form actions target 44px controls, support Chinese and English, and work with keyboards and narrow screens. Reading, restricted access, incomplete reads, expiry and unconfirmed operations have separate messages. Registration, grants, inventory and execution must not collapse into a “connected” or “repaired” label. Existing shared dialog controls are outside a claim that every application control now meets 44px.

## Separate source, draft and eligibility

The public pin fixes the Pool, Runner, Worker, tenant, project, original tool ID/version/SHA and adapter/protocol. It contains no private paths, commands or credentials. A URL is navigation intent only. Both destinations re-read current authority and the exact source; an A→B→A change cannot revive an old asynchronous response.

| Object | Retention and adoption |
| --- | --- |
| Current reads | The exact Runner GET and original readiness response must agree on the runner, credential generation and project grant. Absence from the first page is not absence from the system. |
| Local freshness | At most 30 seconds, reduced by request latency and credential expiry. This controls UI eligibility only, creates no server authorization lease and does not poll automatically. |
| Private form | Inputs stay in the original in-memory form. Same-source expiry retains them, removes the preview and disables generation, copying and continuation. An explicit refresh inside the dialog can restore current eligibility. |
| Return link | Current `can_open_project` is required. The original project tuple retains the exact pin; another project uses the existing generic entry. A changed original tool requires a new choice. |

The retained form basis serves display only. It cannot authorize adoption after the current read expires. Changes to actor, API, Pool, runner identity or source continue to invalidate the old context. Original unconfirmed writes and unknown inspections take priority.

## Module responsibilities

- **ToolWorkspace and Root navigation:** create and validate the public pin; re-read the original tool and diagnostics before handing it to the original Quick flow.
- **PoolToolRecoveryPanel:** read current owner-authorized facts, bound their display lifetime and re-read on explicit preparation. Restricted access does not open an ordinary project identity guide.
- **PoolWorkspace:** retain original write receipts and dialogs, separating the form's display basis from current eligibility.
- **Original preparation and connection components:** preselect public project IDs from the approved grant; retain original private configuration, Shell quoting and connection modes. Current authority controls the project link.
- **Original Pool control plane:** the exact Runner read reuses existing administration permissions and the public DTO. No new table, credential authority or dispatcher is introduced.

## Desktop departure and shutdown

The native controller defaults to **Stay** and keeps a single pending decision. For captured close, quit or exact same-origin main-frame navigation, it cancels unloading and asks asynchronously. Explicit **Leave** permits one exact replay only while the window, document epoch and intent still match. It never acts on an expired event after an asynchronous return or installs a permanent bypass.

Reloads and other cases without a known exact destination use the official synchronous confirmation within the current event. This dialog briefly blocks the main process; the design is not entirely asynchronous. Only a real window `closed` event starts the original managed-runner, tool and service shutdown chain. Stay keeps services and IPC intact. Leave does not establish a stopped task, clear a Journal or accept business delivery.

## Implementation and AI SOP

Reconcile original uncertain operations first, then verify current source and permissions. Focus checks on exact reads, same-source draft retention with expired adoption disabled, restricted owners and the return locator. Reuse unaffected execution evidence. Keep production-component tests over synthetic HTTP, real desktop events, injected dialog choices and actual target-machine execution clearly separate. This stage does not prove real remote repair, vendor execution, human OS button interaction, Worker teardown, cross-platform operation or complete platform acceptance.
