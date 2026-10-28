# Project risk contract

Status: T-PROJECT-121 implementation contract for current suma1.0.0. Owners: project governance, quality and workbench. Requirements7/10/12/13. [PRD](../docs/product/project-risks.md), [architecture](../docs/architecture/project-risks.md).

`ProjectRisk` owns a formal risk independently of `PlanningAlert`. Manual creation and explicit adoption of a real current-project alert share the same aggregate. The protected alert-to-risk relation is unique. Generic Record fields cannot create source relations, assessment history, review facts or closure authority.

All writes carry a stable idempotency key, and existing aggregates require an expected version. Current project authority is checked before original-operation receipt recovery. Acknowledged or resolved model advice does not close a formal risk. Failed responses preserve the original request for recovery.

Assessments contain title/detail, optional probability and impact1–5, human owner, review date, strategy/plan and real same-project links. A score exists only when both ordinal inputs are known. Server-computed scores do not represent a statistical probability or measured loss.

The lifecycle distinguishes open, mitigating, in-review and closed. Submitting review freezes the current assessment and applicable current linked records, owner eligibility and formal task acceptance/evidence. Closure requires a currently authorized browser reviewer and unchanged submitted basis. Code mitigation requires at least one linked task with the existing current formal acceptance and test evidence gates; a green execution alone is insufficient.

Accepted, avoided, transferred and not-applicable dispositions are explicit human risk decisions with reasons and evidence statements. They are displayed separately from verified code mitigation and never mutate task/test/release status. A returned review remains history. Reopening preserves closure snapshots and starts a new cycle without inheriting old approval eligibility.

Collection statistics come from the full currently authorized project query; loaded-page counts are separate. Detail history is bounded with an explicit total. Missing assessment data and unavailable evidence remain unknown. Public DTO/route details are generated from the current Go implementation; no arbitrary CLI, credentials or remote operations are accepted by this domain.
