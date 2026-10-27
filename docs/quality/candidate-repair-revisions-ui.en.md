# Repair revision UI: focused checks and actual UI verification

Stage T-P4-033, suma 1.0.0, 2026-10-27. Initial focused checks, 14 isolated warm-journey checks and representative final cold UI checks pass. Another 21 checks verify the primary preview after warm reads. Actual, component and unverified coverage are distinguished below; this is not full-platform or production acceptance. See the [verification report](candidate-repair-revisions-verification.md) and [summary](candidate-repair-revisions-summary.json). Complete P0–P7 work continues.

See the [contract](../../contracts/candidate-repair-revisions.md), [interaction design](../design/candidate-repair-revisions.en.md) and [Chinese report](candidate-repair-revisions-ui.md). Existing 032 Git/SDK, execution and formal-test evidence remains in its [original report](candidate-conflict-repair-verification.md); it is not counted as new 033 acceptance.

## Completed frontend checks

The following commands ran from `apps/web` against installed tools and exited 0. The first pnpm entry stopped at workspace-structure verification before tests started; no dependencies were reinstalled.

```text
node node_modules/vitest/vitest.mjs run src/CandidateRepairRevisionPanel.test.tsx src/TaskRunHistoryPanel.test.tsx src/candidate-repair-revision-wire.test.ts src/task-run-history-model.test.ts src/candidate-conflict-repair-wire.test.ts src/write-action-recovery.test.tsx src/CandidateConflictRepairPanel.test.tsx src/CandidateCompositionPanel.test.tsx src/TaskCodeBaselinePanel.test.tsx src/UsageWorkspace.test.tsx
node node_modules/typescript/bin/tsc --noEmit
```

From the repository root, `node apps/web/scripts/check-i18n.mjs` passed language, placeholder, plural, markup and hardcode checks for 4,335 paired keys. These are source-freeze results. Two later discoveries each received one targeted regression: observed-system-reason localization and reading the current run when opening Review. Isolated cuts were built; current type checking passes. Unchanged catalogs reuse their evidence rather than repeating all component checks.

| Scope | Result and limit |
|---|---|
| 10 affected files, 37 tests | Passed, including affected 032 regressions; these are not 37 new features or formal candidate tests |
| Revision interaction | Folded configuration reads, explicit selection, same-task/new-plan response, exact URL, separate Start,409 draft retention and renewed acknowledgement |
| Recovery and authority | Malformed 200 retains the original operation across remount and a historical parent; original validator/caller/scope preserved; write 403 hides child facts |
| Failure eligibility | Missing certification blocks; certified context supplies exact recovery digest. Component fixtures do not prove actual host execution |
| History and usage display | Paging deduplicates by run ID; exact links and nullable observed facts survive. Known zero tokens do not turn null unpriced cost into zero |
| Types and language | Generated contracts and 4,335 paired labels pass. Full device, authority-channel and offline matrices remain untested |

Actual public Service response fixtures establish wire shape, not browser acceptance:

- `apps/web/src/test-fixtures/candidate-repair-revision.json` preserves actual create/context/revise envelopes. The four original JSON files, concatenated in capture order, have SHA-256 `57bb3b7a52fdef29144ea1fa0d0eff551d6529c3b67c5036c179b4872c7d18f3`.
- `apps/web/src/test-fixtures/task-run-history.json` preserves the actual list response, including bound version 8 and nullable earlier facts. Original SHA-256: `e9894b5219674e37bf9a6f97fa1e8dc0ee02728bc98952daca6ce748aa92f77f`.

Identifiers and digests are deterministically sanitized. Field presence, strategy, versions, source order and cross-envelope relationships remain. Originals stay in ignored stage evidence; credentials, databases, logs and screenshots are not committed.

## Actual browser results and remaining coverage

The isolated 033 journey uses matching assets and preserves user tabs and accepted A/X inputs. Three new runs, four formal assertions once, one explicitly user-authorized local fixture-owner Review and unstarted downstream D have independent execution/database evidence. Screenshots alone do not establish those facts, and refreshing did not repeat models, formal tests or the old 032 journey.

| Step | Action and expected evidence | Status |
|---|---|---|
|1. Exact location and disclosure | Exact task/plan links, fixed configuration/conflict links and keyboard history expansion were exercised. Folded zero-read and route distinction are component evidence, not an invented browser-network assertion. | Representative UI verified; zero-read covered by components |
|2. Prepare a new preview | Explicit B-to-A ready revision keeps task and ordered sources, returns the exact new URL, creates no run and clears Start acknowledgement. Refresh retains the old definition and disables historical Start. Wire/component checks cover ineligible profiles and command nodes. | Actual and focused checks pass |
|3. Certified failure and replacement | The actual typed A startup failure is visible. Explicit healthy-B revision keeps the failed run until separate Start. A independently tests B's exact candidate, then explicit Review precedes binding to unstarted D. | Actual journey passes |
|4. Original-operation recovery | Real HTTP or focused components cover original path/body/key, malformed200, historical-parent recovery,409 drafts and401/403 hiding. No additional browser network-loss, revoked-access or lost-receipt fault was manufactured. | HTTP/components pass; browser fault matrix pending |
|5. History and usage | Two actual bound records retain the P2 failed and current P3 run/plan links. Ledger evidence keeps certified zero distinct from unpriced cost and unknown healthy usage. Nullable legacy observed facts come from actual Service/component fixtures. | Actual bound history/accounting verified; nullable facts have focused evidence |
|6. Chinese/English, keyboard and cold readback | Representative locales, keyboard history, exact historical URL refresh and D baseline pass. Fixed Review opening shows the current successful run; cold verification opens without resubmitting. No Runner/model/formal retest; complete device/narrow-width coverage remains pending. | Representative warm/cold passes; full device matrix pending |

The original warm receipt retains the stale Review-summary discovery. Final cold checks verify its correction without replacing that earlier receipt. Final `web-delivery` contains 135 files; index SHA-256 is `cf059de98de9627fe5edf1ca2faa0e7bf15e375820e4ac149f2cf56d50c873d6`. Browser text does not manufacture host proof. Revision, running, testing, Review, target integration and baseline binding remain separate facts. Separate primary warm evidence verifies all 135 served artifacts, all 186 table schemas, 182 complete non-lock tables and only 34 lock sequences increasing by 24. Vendor, physical multi-host and production acceptance remain incomplete.

## Minimum acceptance record

Record matching source/build identities, actual task/old-new plan/run IDs and versions, exact URLs, selected fault branch, original-operation digest and necessary before/after reads. Keep screenshots and console/request receipts in ignored stage evidence, retaining failures and subsequent corrections. A screenshot or label alone cannot prove transactions, independent tests, billing or acceptance.

Originals stay in ignored `browser/warm-verification.local.json`, `browser/cold-verification.local.json`, `cold-readback/closed.local.json` and `primary-upgrade/warm-verification.local.json`; the [machine report](candidate-repair-revisions-summary.json) records their digests. Cold Control exited normally 0 with selected business schema/rows/accounts and original non-DB artifacts preserved; no login, model, formal retest or business command was repeated. The bounded local stage is closed; full-platform implementation continues.
