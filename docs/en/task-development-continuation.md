# Continue development from the previous candidate

Status:1.0.0; owners: control plane, Runner and Web; requirement: T-DEVELOPMENT-CONTINUATION-186. [Contract](../../contracts/task-development-continuation.md), [architecture](../architecture/task-development-continuation.md), [verification](../quality/task-development-continuation-verification.md).

A reviewed new task discussion can explicitly select sealed code from the same task's latest terminal run. The platform freezes the current task version, original producer, run version and complete candidate evidence in the original dispatch plan. Preview and Start remain separate. Failed old tests or a rejected old review do not prevent adopting code as development input; they never become proof of new acceptance.

The protected DevelopmentContinuationBinding is separate from an accepted cross-task CodeBaseline. A workflow marker, transfer ID or locally available commit cannot grant access. Current consumer authority and historical task/run provenance are checked through the original code-input broker, before and after artifact IO.

The Runner creates a fresh isolated attempt at M, produces N and retains B as the bundle prerequisite anchor. It still downloads and verifies an authorized input when M already exists locally. New independent tests consume exactly N, followed by current human review. Tools, models and budgets are selected anew.

The existing task discussion, source selector and development preview provide the user journey. This is code continuity; Codex still starts a new thread/turn per attempt. Physical multi-host execution, real vendor models, long-lived tool sessions and full platform acceptance remain separate evidence scopes.
