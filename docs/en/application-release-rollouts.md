# Ordered application release rollouts

Status: current 1.0.0 contract, T-ROLLOUT-125, requirement 25. Owners: application, scheduler and release domains. Evidence and production acceptance remain in the [quality records](../quality/verification.md). Related: [architecture](../architecture/application-release-rollouts.md), [operating SOP](../development/application-release-rollouts.md), [API](../../contracts/application-release-rollouts.md).

A release set freezes reviewed application versions and candidates. A plan group maps them to original release plans. An ordered rollout is a separate explicit authorization to progress those plans through the existing Go DAG. It grants no plan approval and introduces no second dispatcher or deployment protocol.

Before starting, approve every exact original plan separately. Preview the current set/group, dependency order, source and quality bases, approvers and environment generations, then explicitly start using that preview digest. At most 16 applications are supported. Dependencies run before their dependants; a cycle blocks automated ordering rather than being silently removed.

The scheduler uses protected bound child workflows with the original reserved run identities. Only a ready member creates its original deployment and environment reservation. A downstream application remains unstarted until its prerequisites have both successful original child runs and matching actual artifact/effect/health facts. Copying a binding ID, receiving HTTP 200 or seeing an agent's completion message grants no authority.

Pause stops new starts while running children drain. Failure, uncertainty or a stale gate pauses further progression. A paused parent does not prove every process has stopped. For an unknown deployment, inspect and reconcile the original provider operation; retain its unknown Runner result and actual effects. Never create another deployment to hide a lost acknowledgement.

Fact reconciliation does not resume a paused rollout. Request a fresh resume preview and explicitly resume with current authority. Healthy original children remain intact, without another authorization or deployment. If all children are already healthy, resume may only finalize the parent. A known failure needs a new reviewed and approved recovery plan rather than a generic retry. Rollback remains an explicit original per-environment operation with a new generation.

The interface shows the parent and each original member plan, child run, deployment, effect, health and waiting reason. Partial, failed, unstarted and unknown facts stay distinct. Application names and user reasons remain original data; fixed states and blocker codes use the shared Chinese/English catalog.

AI may explain blockers and propose reviewable actions. Source text and tool output are not permission to approve, obtain secrets, change policy or run extra commands. Preserve the original project, request body, expected digests and operation key when recovering a lost response. Report deterministic local tests, vendor execution and customer production approval separately.

Current deployment coverage remains `static_site_v1`. Dependency ordering is not cross-environment atomic rollout, coordinated automatic rollback, traffic canary management, generic container publishing or completed production full-stack deployment.
