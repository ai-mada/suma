# Explicit development and independent-test dispatch

Suma 1.0.0 · T-TEST-DISPATCH-BINDINGS-216. The authoritative contract is [test-dispatch-bindings.md](../../contracts/test-dispatch-bindings.md). The full 31 requirements and P0–P7 remain active.

## User flow

After choosing a task, a published workflow, its development node and a development tool, expand the optional independent-test section. Explicitly select tools for the existing tests whose `candidate_from` points to that development node. No first option is selected automatically.

Leaving a test unchanged retains its published definition. This must not be presented as verified independent testing. Discovery lists help find tools; adoption reads the exact current profile and keeps selected items visible across directory pages.

Create the ordinary dispatch preview and review the frozen test node, tool version, configuration SHA, Worker and command before starting. Only the copied plan changes. The published workflow, dependencies, conditions and inputs remain unchanged.

The selected test tool must be enabled, have fresh matching inventory, use a writable fixed command with the complete JUnit report protocol, and belong to a different Worker from the development tool. Different slots within one pool are not different Workers. Admission does not prove that code has been tested or accepted.

## Contract and modules

The existing preview request accepts optional `test_bindings`, with at most 64 explicit selections. Each selection contains `node_id`, `tool_profile_id` and `expected_tool_version`. Explicit null, missing item fields, unknown fields, case-folded replacements and duplicate node selections are rejected. A subset may be overridden; omission preserves the published definitions.

The existing plan freezes six server-resolved fields for each override: `node_id`, `tool_profile_id`, `tool_profile_version`, `tool_profile_sha256`, `worker_id` and `command_id`. Historical snapshots are not refreshed with current metadata.

The control module reuses the original profile eligibility, current authorization, route, transaction, version and operation-receipt checks. It changes only an eligible static candidate test's command and exact tool reference. An exact old `command:<command>` constraint may change with that reviewed command; unrelated capabilities cannot be removed or broadened. Artifact consumers and other producers are not eligible override targets. The original DAG compiler remains authoritative.

Plan reads, atomic Start and the final run-route hook recheck current profile facts and consistency with the actual copied node. There are no new API routes, tables, dependencies or dispatchers. The original preview, plan-read and start endpoints remain in use.

## Recovery and quality

Account, organization, API caller, project, task version, workflow version, development node and development profile version/SHA bind the selection. Scope changes and late asynchronous responses cannot revive old choices. Pending writes retain their original body and operation key; recovery must not fetch a new version and create a different write.

When explicit test overrides exist or selection is unresolved, recommendation creation and new adoption are disabled because that path must not silently omit the choices. Existing pending recommendation operations retain their original recovery and leave protection. Use the ordinary explicit preview to create the new plan.

Code acceptance pairs every actual producer with a successful test of its exact current candidate. Both nodes need nonempty identities and Workers, positive current attempts and fences, and different Workers. The test's source node, attempt, fence and evidence SHA must match that producer, with test base and head equal to the producer's head. Both attempts may legitimately be 1; unequal attempt numbers do not establish independence.

A small source-matching predicate is shared by the pure Run gate and the existing strict database pairing. The pure projection does not attest the database candidate pin, replace DAG completion checks, certify a JUnit report or grant Review authority. The strengthened gate closes the same-Worker acceptance gap while preserving existing human decisions.

## AI engineering SOP and delivery boundary

Read the published source and current task, explicitly select overrides, inspect the frozen preview and start or recover the same operation. For failures, compare the request, six frozen fields, actual copied node and current profile; resolve the real authorization, version, inventory, Worker or route issue. Do not fabricate evidence or publish a temporary graph to bypass admission.

Verification should focus on the changed contract and reuse valid evidence for unchanged inputs. Final counts, interface checks and actual execution evidence belong in the stage quality report; this document makes no advance claim that the integrated journey has passed. Vendor/model execution, physical multi-host operation, other databases, installers, scale, production and the complete platform remain broader acceptance work.

The [stage report](../quality/test-dispatch-bindings-verification.md) records the real Git, separate-Worker same-commit JUnit and fixture Review journey. The UI creates the preview; the Go host starts and reviews the run. Official SQLite recovery checks follow the dependency update. The complete service suite, vendor/model execution and customer signoff remain separate acceptance scopes.
