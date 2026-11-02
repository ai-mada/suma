# Local tool discovery experience

Stage: T-TOOL-DISCOVERY-202. Product: suma 1.0.0. Owner: main integration agent. Status: scoped implementation and verification are complete; see the [evidence report](../quality/local-tools-discovery-experience-verification.md). See the [contract](../../contracts/local-tools-discovery-experience.md), [Chinese PRD](../product/local-tools-discovery-experience.md), [architecture](../architecture/local-tools-discovery-experience.md), and [interaction design](../design/local-tools-discovery-experience.md).

## Outcome and scope

The existing scan lists nine adapters, so missing installations dominate the first screen. The same connection flow should first answer what was detected locally, what needs attention, and where to continue. Reuse the current passive scan, Quick connection, existing connection lookup, and remote node entry. This stage adds no dependency, API, permission, probe, installer, dispatcher, or second configuration authority.

| Observation | Presentation |
| --- | --- |
| `installed=true` | Primary list |
| `installed=false` with any reason other than `not_installed` | Primary list with the original explanation |
| Selected adapter, even when not installed | Primary list |
| Not installed, exactly `not_installed`, and not selected | Other tools, initially collapsed |

Both groups preserve original order and row references. CLI and SDK protocols remain separate. Detected count includes only `installed=true`; attention count includes only `installed=false && reason_code!==not_installed`. These are installation observations, not connected nodes, authenticated models, or dispatch eligibility.

## Architecture and interaction

The existing parser remains authoritative. The pure `projectLocalToolsScan(items, selectedAdapterID?)` projection returns `primary`, `other`, `detectedCount`, and `attentionCount`; it does not mutate or repair input. The existing `LocalToolsScanPanel` renders both groups with the same row actions and accurate protocol/version details.

Use native `details/summary` for Other tools, closed initially. Empty primary results receive a short explanation and the existing node entry when provided. Missing or ambiguous adapter descriptors must not produce guessed actions. Expanding and selecting cause no probe, installation, configuration, or HTTP request; only the existing explicit recheck forces a scan.

Preserve cache, expiry, busy, failure, desktop bridge, identity/project/caller generation, and receipt semantics. A scope change resets disclosure state; late responses cannot restore the previous scope. When an explicit selection moves its row between groups, preserve focus only while that original action still owns it. External selections must not steal focus.

Installation, SDK assets, credentials/model authentication, node connection, paused registration, explicit enablement, execution, testing, and acceptance remain separate facts. An unsupported Claude SDK version keeps the existing precise version explanation. Unknown operations keep their original recovery path and request identity.

## Language and accessibility

Maintain paired zh-CN/en-US entries in the existing catalog and format counts with the selected locale. Long names and protocol labels wrap. Native summaries work with Tab and Enter/Space, keep visible focus, and do not contain nested action buttons. Interactive targets are at least 44px. Decorative icons are hidden from assistive technology; text conveys state without color alone.

At 390px, use a single column with wrapping actions; at 1366px, retain a compact reading order. Avoid page overflow, automatic scrolling, and forced animation. Expired observations and failures must not appear as current healthy connections or zero installations.

## Reference and implementation SOP

The local Multica reference is pinned to `5a2cc891ac338eed1919aa11a1eef5957cd82cec` at `D:/codes/github/multica`; the workspace was observed clean and left unchanged. `use-runtime-picker.ts` preserves manual selection across refetches; `step-runtime-connect.tsx` distinguishes scanning/found/empty and explicit refresh; `derive-health.ts` separates observation from derived presentation. The exact collapse rule here is a suma decision, not a claim about Multica parity.

Multica's `LICENSE` combines Apache 2.0 text with additional conditions, including hosted/embedded use. This stage references patterns without copying restricted source, branding, or assets. See the [existing provenance note](../integrations/multica-claude-sdk-connect.md).

AI implementers first read the frozen contract and parser, respect separate projection/UI/document ownership, reuse native disclosure and original actions, and keep bilingual states aligned. Run only the new projection checks, directly affected component checks, and integration-owned type/i18n and representative UI verification. Preserve compact evidence; do not repeat unchanged SDK/vendor checks or commit caches.

## Acceptance status

Verified within this stage: mixed grouping/counts, all-missing empty primary state, selected missing adapter visibility, no extra I/O on expand/select, original protocols/actions, scope and late-response isolation, keyboard/focus, and representative Chinese/English 390/1366 component layouts. A desktop control-height token conflict was fixed locally; only that affected group was rerun while the original passing English evidence was retained. The report preserves the exact scope and failures. Full Electron, vendor-model, multi-machine and cross-platform installation acceptance remain separate. The complete 31-item/P0–P7 platform scope stays active; this stage does not complete it.
