# Per-task model choices in expert plans

Status: T-P4-040 / 1.0.0, bounded local implementation. [Contract](../../contracts/expert-team-model-execution.md), [architecture and AI SOP](../architecture/expert-team-model-execution.md), [verification](../quality/expert-team-model-execution-verification.md).

Each create/revision item can carry the existing typed `model_execution_selection`. Role configuration remains unchanged. The UI offers one choice per role and explicit task overrides; the resulting requests always bind choices per task. Each task has its own allowance, not a shared team-plan budget.

Dependency waits and later dispatch reuse the original binding and exact historical rate, while checking current deployment, tool, task, route and credential authority. A new quote requires a new preview or explicit unstarted revision. Started definitions do not change, and missing capability never falls back to the environment.

Current scoped options, unpriced states, frozen model facts, source history and exact request recovery are bilingual. Unusable success responses retain the original operation and budgets until a matching result can be read.

The final owned native protocol execution reported 40 tokens and an operator estimate of57 CNY micro-units, with no outstanding reservation. Across this stage there were2 owned loopback requests, including an earlier combined failed test invocation, and0 vendor requests. GUI preview/revision did not start another task. This is not vendor billing, independent code testing or formal acceptance. Full P0–P7 and requirements1–22 remain in progress.
