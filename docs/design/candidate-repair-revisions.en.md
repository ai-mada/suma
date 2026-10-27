# Repair revision and execution history interactions

Stage T-P4-033, suma 1.0.0, 2026-10-27. Focused components, actual response shapes, types, 14 isolated warm checks and representative final cold interactions pass. The 21 primary warm checks also pass. See the [bounded verification](../quality/candidate-repair-revisions-verification.md). Full device, vendor, multi-host, production and full-platform acceptance continue.

See the [frozen contract](../../contracts/candidate-repair-revisions.md), [032 interaction design](candidate-conflict-repair.md), [UI checks and pending browser steps](../quality/candidate-repair-revisions-ui.en.md) and [Chinese version](candidate-repair-revisions.md).

## A new preview for the same task

The repair child’s composition details retain the existing compact single-column layout, Field controls, icons and surface/border/muted tokens. The original conflict remains a failed fact. Revision keeps the same repair task, original conflict identity and complete ordered sources, and creates a new plan ID.

The summary shows the server-fixed plan version, revision ordinal, workflow version, producing agent, native profile version and route. Earlier plans retain their exact run and results, with explicit links to the current, previous and initial plans. An old URL stays on that plan.

Author, reason and definition digest are disclosed on demand. Author identity comes from the safe server projection, never the current viewer. An observed earlier definition shows its observation time, without inventing original preparation time or missing authorship.

## Explicit revision, separate Start

**Revise repair plan** starts folded. Without a pending recovery, folding performs no revision-context or configuration reads. Opening first reads eligibility; eligible forms then load real workflows, native profiles and the current project route.

Fields appear in this order: workflow version, native profile, producing agent, current route, reason and acknowledgement. No first option is selected automatically. Only a root agent with independent candidate tests qualifies. Disabled, unavailable, read-only or repair-incapable profiles cannot be selected. Changing workflow or profile clears the producer; changed inputs or basis clear acknowledgement.

The current route is read-only and separate from the old plan’s fixed route. The server fixes it in the new preview and rechecks it at Start; the browser cannot submit an arbitrary route.

| Chinese action | English action | Effect |
|---|---|---|
| 修订修复计划 | Revise repair plan | Open eligibility and configuration |
| 准备修订后的预览 | Prepare revised preview | Prepare another plan for this same task |
| 找回原修订 | Recover original revision | Recover the exact original request and key |
| 任务执行历史 | Task execution history | Inspect exact run/plan associations |

A successful revision opens only the returned plan ID using `record=<same task>&panel=composition:<returned plan>` and clears Start acknowledgement. Revision invokes no model and creates no run. Starting remains a separate explicit action.

An unstarted ready plan may be revised. Replacing a failed plan requires server-certified failure before workspace and main-process entry for every relevant attempt. Error prose, missing output and timeouts cannot establish eligibility. The panel discloses the predecessor run, certified attempt count and recovery-basis digest. Unknown outcomes retain original-operation recovery.

**Refresh fixed preview** remains a separate reasoned expiry extension for the same definition. It does not change workflow or profile. Accepted plans cannot be revised.

## Draft and receipt recovery

A 409 preserves selections and reason, and requires fresh plan/context/configuration reads plus renewed acknowledgement. A 401/403 immediately hides protected child facts and form.

Before sending, the exact path/body/key is persisted within user, organization, project and original-plan scope. A successful HTTP status is acknowledged only after validating response shape, task, sources, parent and selected definition. Malformed 200 responses and lost receipts retain the original operation across refresh or reopening. A historical parent with pending recovery still exposes that recovery.

Scope changes isolate prior drafts and abort old reads. Late results belong to the original scope. Recovery never selects an inferred latest plan or starts work.

## History, usage and language

The separate folded **Task execution history** section pages and deduplicates by run ID. Each row links to its exact run and any recorded plan, distinguishing current from earlier associations. Preparing a replacement does not change the failed current run; replacement Start performs that binding.

Actual bindings show recorded task version and binding time. Observed earlier associations retain their observation time and nullable historical version/time. Reading history creates no rows and does not reconstruct earlier facts from today’s task.

Usage source `host_startup_proof` displays **Certified startup failure / 已认证的执行启动前失败**. Known zero tokens remain separate from unpriced cost: null cost and unreported counters stay unknown.

System labels and controlled blockers follow the current Chinese/English locale. User reasons, task text, code, paths and immutable evidence stay literal. Inputs have explicit names; primary controls retain 44 px targets, long identifiers wrap and narrow layouts remain single-column. Actual keyboard, narrow-width and refresh acceptance is still pending in the [UI checklist](../quality/candidate-repair-revisions-ui.en.md).
