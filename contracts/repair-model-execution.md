# Native model selection for conflict repair

Stage T-P4-102; suma 1.0.0. Current implementation contract, 2026-10-28.
The complete P0–P7 and requirements 1–22 scope remains active.

## Explicit preparation

`CandidateCompositionRepairCreateRequest` and
`CandidateCompositionRepairRevisionRequest` accept optional
`model_execution_selection: modelcontract.Selection`. A native repair profile
advertising `suma-model-execution-v1` requires this selection. It fixes the
authorized deployment ID/revision/digest, closed parameters and optional budget.
Missing, invalid, unsupported or conflicting selections fail atomically before
persisting a new task, reservation or revision. A fixed native repair tool that
does not advertise that protocol legitimately retains its configured execution
without a selection.

`CandidateCompositionPlan.model_execution` is the exact non-secret immutable
`modelcontract.Binding`, also embedded in its producer node. Preparation uses
the current authorized inventory and freezes the matching rate revision, or
explicit unpriced state. Native limits remain at most 16 steps and 32768 output
tokens. Input reservation remains the conservative complete deployment context
window multiplied by maximum steps; output reservation is the fixed maximum
output budget. Repair does not introduce a second estimate or accounting path.

An existing conflict reservation recovers its exact current plan only when its
frozen deployment, parameters and budget match the requested selection. A
different selection requires an explicit revision and receives a conflict
response from Create. Recovery never selects a new quote. Exact idempotency
receipts retain their original operation and effect.
Create recovery keeps the original version, model binding and run reference
after another view starts the plan; stale receipts expose no fresh actions.
Current GET exposes the current run, and current revoked authority still denies
receipt recovery.

## Frozen execution and current gates

The final repair workflow includes the fixed task context and repair goal before
model binding. The repair definition digest covers that context, complete
workflow and goal, explicit binding, ordered sources, original conflict identity,
tool, route and operation. The public binding and producer binding must agree.
The managed run producer must equal its frozen producer specification, including
goal, parameters and disabled automatic retries.

Read/Refresh/Start retain the reviewed binding and its historical rate revision;
they recheck current authorized deployment availability and profile/route/source
gates through the existing native model validator. Claim and fresh source access
repeat those checks. A changed inventory blocks work without model substitution,
repricing, retry or a new usage reservation. Exact historical result validation
and receipt recovery remain separate from permission to perform fresh work.

Ready and certified-before-workspace startup failure revisions can explicitly
choose another model. They keep the same protected task, original conflict and
ordered accepted sources, and preserve the old plan/run/attempt facts. Revision
creates a ready preview; its separately acknowledged Start creates a new managed
run. Native attempt accounting, frozen rate, budget admission, startup-proof zero
settlement and unknown/partial usage semantics remain shared.

## Supported boundary

`candidateConflictRepairProfile` remains restricted to the native suma JSONL
repair workspace lifecycle. Codex conflict repair is unsupported; the creation
and revision interfaces do not accept a Codex selection. The UI must explain
this boundary and require an eligible native repair profile. No Runner or SDK
contract, database table, external provider call, independent test or human
acceptance gate changes.

Focused tests cover explicit choice/atomic rejection, frozen quote and native
ledger reservation, current inventory and producer tampering gates, ready and
certified startup revisions, HTTP request/response shape, and CLI original-body
recovery. These are local typed protocol facts without inference, vendor tasks
or production acceptance.
