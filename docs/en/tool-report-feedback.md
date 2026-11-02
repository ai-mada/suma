# Tool report feedback

Version 1.0.0 · T-TOOL-REPORT-FEEDBACK-204 · Owner: integration agent. Status: scoped implementation, real HTTP/SQLite flow and representative bilingual component verification passed.

The existing remote setup guide provides an explicit inventory check and a bounded wait for a new report. Both use the current project, exact Worker and selected protocol. Formal catalog validation, inventory parsing and full Quick capability matching remain shared with the original connection path.

Waiting captures the server's current `as_of` and `observed_at`. A fresh observation must be strictly later than both fixed boundaries to count as new; nanosecond precision is retained. An old operation receipt, an identical timestamp or a clock rollback does not prove a new report. Waiting lasts at most 60 seconds with one read at a time and a three-second interval. Hidden pages suspend reads without extending the deadline.

Changing configuration, target or caller invalidates earlier results. Authorization failures clear observations; a network failure means unknown. Remaining freshness is conservatively reduced by request and monotonic elapsed time, and review eligibility expires even without another poll.

An explicit selection returns the exact adapter, Worker and command to the original Quick path, which reads current facts again. Registration remains paused and enabling remains a separate action. Copying a command, creating a configuration, recovering an old receipt, receiving inventory, registering and enabling are separate facts. Inventory may come from an existing Worker and is not exact causal proof of the copied command or file.

No new business API, database table, CLI mode, event stream or dispatcher is introduced. Production-component validation uses an isolated real HTTP/SQLite service and owned metadata; it does not prove vendor CLI/model execution, a full desktop journey or production acceptance. See the [contract](../../contracts/tool-report-feedback.md), [Chinese PRD](../product/tool-report-feedback.md) and [verification](../quality/tool-report-feedback-verification.md) for the implemented scope and evidence. The complete platform remains active under its original delivery plan.
