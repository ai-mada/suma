# Resource work calendars

Current initial release1.0.0, stageT-CALENDAR-134. The [contract](../../contracts/resource-calendars.md), [architecture](../architecture/resource-calendars.md) and [verification](../quality/resource-calendars-verification.md) define the exact scope.

Open Development → Planning → Resource capacity, select a week, then open a resource calendar. Define Monday through Sunday and optional dated exceptions. Empty means unknown;0 explicitly means unavailable. Applying the workweek template edits a draft only. Review and save explicitly.

A calendar has its own version. Conflicts preserve the draft. Recover an uncertain save using the original operation; never create a second write merely because its receipt was lost. Current authorization and resource eligibility are checked again before reading or replaying.

Planned effort is distributed only when both task dates are explicit. Available-hour weights over the complete interval determine the selected week's share. Unknown days or a zero denominator leave positive estimates undistributed. A deadline alone does not imply one day of work.

The incomplete-schedule count covers this resource's project tasks. The undistributed amount is the complete estimate of overlapping tasks, not additional weekly hours; neither should be summed across weeks. These figures are planning estimates, not timesheets, live availability or machine quotas.

The platform does not import holiday data, approve leave or change a task merely because the calendar is saved. Calendar changes invalidate affected formal planning input snapshots and require a new preview. Complete HR integration, actual worklogs, cross-project capacity and production acceptance remain in the full platform plan.
