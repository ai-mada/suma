# Automated test plans and case evidence

Scope: suma 1.0.0, T-QUALITY-115. Owners: quality domain, Go scheduler, Runner and workbench. This stage adds versioned automated cases, exact-attempt reports, Review gates and explicit failed-case issue creation. Complete manual, application and release testing and the full defect lifecycle remain pending within the platform scope. See the [Chinese guide](../development/test-plans.md), [contract](../../contracts/test-plans.md) and [architecture](../architecture/quality-evidence.md). Configuration examples and this design document are not verification results.

The [stage verification report](../quality/test-plans-verification.md) and [summary](../quality/test-plans-summary.json) own the actual checks, environments, outcomes and open verification gaps.

## Define and publish a plan

Each authorized project task has one plan with multiple immutable revisions. Saving a draft does not replace the published revision. A draft can have 0–100 cases and incomplete text, steps or selector names. Publication needs at least one complete case with title, nonempty steps, expected result and selector name, plus coverage of every current **task acceptance criterion**. Each case covers at least one criterion when the task has criteria. `covers:[1]` means the first task criterion, not a requirement criterion.

Use a stable case key and an exact `{class_name,name}` selector matching the actual JUnit `classname/name` pair. Nonempty selector pairs must be unique. Arrays and all required nested fields must be explicit; null and unknown members are rejected. Cases occupy at most 128 KiB of canonical JSON, and a request at most 256 KiB. The [contract](../../contracts/test-plans.md) defines the remaining field bounds.

Create and save with the current task version, optional requirement version and operation key. With no linked requirement, explicitly send `expected_requirement_version:0`. The task business-basis hash determines whether the plan remains current: a technical task-version increment from run binding or Review alone does not invalidate it. A real change to the task basis does. The linked requirement is checked separately at its exact version.

Only a currently authorized browser approver can publish or retire a plan. Project API subjects can read and save within their scope, but cannot approve. Neither an Agent nor a Runner can use the plan API for self-approval. Changed sources require review and a new draft; original revisions remain available for traceability.

## Connect an installed test CLI

An administrator configures the Runner repository, fixed baseline, installed executable and isolated test command. The local command must use `adapter:command`, `protocol:text-v1`, `read_only:false`, `test_evidence:true` and `test_report:true`. This capability cannot be combined with planning, model, native-workspace, composition or artifact modes. Its immutable profile declares `test_report_protocol:suma-junit-report-v1`; the current Runner or execution pool must satisfy the existing dispatch checks.

The Runner allocates a fresh report path outside the Git source and exposes it as `SUMA_TEST_REPORT_PATH`. In administrator-owned Args, the literal `{{test_report_path}}` token is replaced in a temporary argument copy. The command starts directly without a shell. Node can use:

```json
{
  "adapter": "command",
  "protocol": "text-v1",
  "executable": "node",
  "args": [
    "--test",
    "--test-reporter=junit",
    "--test-reporter-destination={{test_report_path}}",
    "test/acceptance.test.mjs"
  ],
  "read_only": false,
  "test_evidence": true,
  "test_report": true
}
```

Node's JUnit reporter and destination option are documented in the [official Node.js 22.23.3 test documentation](https://nodejs.org/download/release/v22.23.3/docs/api/test.html#test-reporters). Reporter output can change across versions. Pin the installed version and check representative real output against the supported subset. A pytest command can similarly use `--junitxml={{test_report_path}}` in local Args, subject to its actual version and emitted selectors. This is not a promise of compatibility with every JUnit producer.

The parser explicitly supports the observed Node.js 22.16.0 layout with `testcase` directly under `testsuites`. A redundant testcase `failure` attribute is accepted only when an actual failure child has an explicit message with exactly the same decoded value. Attribute-only failures, conflicting messages, missing child messages, error/skipped substitutions and unknown status attributes are rejected. Direct cases retain the same global selector, counter and resource limits. Support for this layout does not predeclare the actual fixture passed.

Remote workflows cannot supply a report path. Administrator Env/EnvFrom cannot override the reserved environment variable. The argument token is rejected unless report capability is enabled. A capability observation establishes configuration, not successful test execution.

## Execute against the fixed candidate

Use the existing task dispatch and Go DAG. Give the independent `test` node an explicit dependency and `config.candidate_from` referencing the same run's code producer, along with the exact tool-profile ID and digest. The original candidate handoff fixes the successful clean code commit and producer attempt/fence; testing occurs in a new worktree. This stage adds no second scheduler or cross-run baseline dispatcher.

Publish before binding the task run. Binding freezes either the published revision or an explicit absence of publication. Publishing later cannot certify an older run. Retiring the plan cannot remove the old gate. Obtain evidence for a new revision through a newly created and bound run. Runs carrying a published plan need at least one report-capable test node, all test nodes must meet that contract, and dynamic maps, subworkflows and external workflows are currently rejected.

After confirmed process-tree completion, the Runner checks file and parent identity, a fresh regular single file, hardlink restrictions, size and read stability. The original UTF-8 XML and SHA-256 are retained unchanged. The parser accepts a bounded JUnit subset: `testsuite/testsuites`, nested suites, test cases and limited outcomes/metadata; at most 256 KiB, 1000 cases and 16 XML levels. DTDs, namespaces, unsupported members, duplicate selectors and contradictory counters/outcomes are rejected. Projected messages are bounded; original XML is never truncated into a purported original.

Actual process exit, protocol terminal, report errors and case outcomes are separate facts. A zero-exit process with failed/error cases still fails the test outcome while retaining exit code 0. A known nonzero process can retain valid XML. Missing, invalid, unsafe, stopped or unknown execution cannot manufacture passing evidence. Configured secrets found in raw or decoded report content prevent upload and leave the local original; this limited check is not a general DLP guarantee.

## Combine source and package tests on a shared pool

Register the application source scope and choose an authorized execution pool. Configure fixed commands for the build, JUnit and package checks. The build and package check require approved artifact profiles on different runners. The package check depends on the build and consumes its sealed manifest. Keep at least one JUnit node with an original report. Bind the run to a separate delivery task to preserve the acceptance that backs the application source.

Open the run in the quality workbench's test plan. JUnit coverage and package gates appear separately. Open a package attempt or its verified artifact to inspect the underlying evidence. When JUnit finishes first, wait for the package proof before approving the task in Review. See the [execution pool contract](../../contracts/execution-pools.md#应用构建制品与独立包测试).

## Read evidence and resolve failures

The server binds reports to the actual tenant, project, run, node, attempt, fence, Worker, fixed profile and clean source commit. Old attempts cannot replace current evidence. Every selected code head needs exactly one matching current result for each planned selector. Missing evidence is `not_run`; duplicate matches are `ambiguous`; unresolved execution is `unknown`. Skipped, missing, invalid, ambiguous or unknown evidence cannot pass Review. Unmapped failures/errors and a nonzero process exit also block it.

Execution states are `pending/passed/failed/incomplete/unknown/source_changed/retired/run_changed`. Open the linked node, attempt, commit and report rather than treating a graph-level success as sufficient. A passed automatic plan remains distinct from the original human Review decision, which rechecks current authority and all task, requirement, candidate and quality evidence. There is no API for manually assigning a case `passed`.

Ordinary execution responses contain report references and case projections, without XML. Opening the report fetches its original through fresh project authorization and displays it as text. Internally, exact-attempt workspace-proof checks still select the complete private attempt-result JSON before extracting the proof; this is response/projection separation, not a claim of zero XML database reads. Controlled labels and diagnostics support Chinese and English; test text, CLI output and original XML retain their own language.

For a current failed/error case, explicitly confirm issue creation with a reason and current plan version. The protected `(plan,run,case_key)` relationship deduplicates repeated creation; editable issue metadata cannot forge that origin. Task delivery joins these issues with the existing IssueIntake and follows the existing issue-to-requirement path. Creating or closing an issue does not change the failed original. Full defect close/reopen management remains pending.

For a confirmed package check failure, create an issue from the package row and enter a reason. The issue retains the exact attempt and the original sealed artifact. Repeating the confirmation for that attempt returns the existing issue. Open it from the package row or task delivery. For a disconnected runner, an unknown outcome or missing current evidence, open the attempt for diagnosis.

## API and recovery reference

Routes share `/api/v1`. GET requires `project_id`; writes carry the same project in their body. Current browser/project API access is checked for every read, write and receipt recovery; publish/retire remain browser-approval-only.

| Operation | Result |
| --- | --- |
| GET/POST `/test-plans` | Page / created plan, 200 / 201 |
| GET/PUT `/test-plans/{id}` | Current plan / saved revision, 200 |
| POST `/test-plans/{id}/commands` | Publish or retire, 200 |
| GET `/test-plans/{id}/executions` | Existing bound-run page, 200 |
| GET `/test-plans/{id}/executions/{run}` | Case evidence projection, 200 |
| GET `/test-plans/{id}/reports/{report}` | JSON with original XML, 200 |
| POST `/test-plans/{id}/executions/{run}/defects` | Explicit associated issue, 201 |
| POST `/test-plans/{id}/executions/{run}/package-defects` | Issue linked to the exact failed package attempt, 201 |

Pages default to 20 and allow at most 100 items, using a current-scope `before` cursor. Filter plans by exact `task_id` when needed. Save the original account, project, method/path, complete body, expected version and operation key when a response is lost. Recover that operation before trying a new write. The same key with changed content conflicts. Historical receipt content is preserved while allowed actions and basis eligibility are derived from current authority and state.

Task model context includes only plan title, revision, digests and case count. Full cases and XML are not automatically injected into each model request; fetch the needed authorized scope explicitly. Reports establish facts asserted by a trusted Runner, not test quality, the honesty of a compromised Runner, supply-chain integrity, production readiness or human acceptance. Directed checks and actual HTTP/CLI/browser fixtures are recorded separately; this guide does not predeclare them passed.

Publication and retirement advance a private governance revision; draft saves do not. Each run freezes that revision, including an explicit unbound plan. Publishing and then retiring a plan cannot restore eligibility for an earlier unbound run. A future run freezes the current retired policy.
