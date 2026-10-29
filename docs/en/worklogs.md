# Manual human worklogs

Status: T-WORKLOG-135, initial release1.0.0. Owner: planning domain/workbench. See the [contract](../../contracts/worklogs.md), [architecture](../architecture/worklogs.md) and [verification boundary](../quality/worklogs-verification.md).

Development → Planning → Worklogs shows explicitly registered human minutes for the selected week. The exact total covers the selected filters, not just loaded rows. Voided entries and immutable corrections remain available. This is manual reporting, separate from estimates, machine execution, acceptance, payroll and billing.

Select a current human resource and task, then register an explicit date and integer minutes. Corrections require the current entry and task versions plus a reason; the person cannot change. Wrong-person entries must be voided and recreated. Acting for another person requires current browser management authority and explicit acknowledgment. Project API users can report only their own time; Runner/Agent credentials cannot report human worklogs.

Active entries for one organization/account/date across all projects cannot exceed1440 minutes. Writes, daily totals, revisions, scope version, activity and the original-operation receipt commit together. Unknown write results retain the original request; stale permissions cannot be restored by replay. Changed list scopes require reloading the first page.

The existing `suma worklogs --help` command provides bilingual list/get/create/correct/void/history instructions. Lists require an inclusive date pair spanning at most31 days; pages contain at most50 entries. Use explicit operation keys and expected versions. No automatic time inference, HR approval or inferred provider cost is introduced.
