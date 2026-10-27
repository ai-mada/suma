# Read the current Codex execution workspace

Status: T-WORKSPACE-103, current 1.0.0 implementation. The workbench extends the approved execution workspace panel without adding a route or wizard. Control and Runner protocols and authorization remain defined by the [observation contract](../../contracts/workspace-observation.md) and [transport capability contract](../../contracts/workspace-transport-capabilities.md). [中文](../design/codex-workspace-observation.md).

## User flow

Choose Execution workspace in Development, refresh the current authorized native or Codex attempts, then browse directories and bounded text. The run link retains the exact run, node and attempt. A closed target, lost authorization, expired observation or changed scope hides previous content; local files never substitute for remote results.

The panel uses the exact returned scope and `can_write`. Codex targets remain read only: no editing component or save request. Native targets expose editing only when `can_write=true`; account write permission alone does not grant access to save on an attempt. The tool profile ID and optional pool ID come directly from the target. An absent pool ID identifies the project runner route. The panel does not infer vendor identity, provider health or host connection health.

Separate directory and file reads can observe different moments. A file digest identifies the original bytes for that read. A page is not the complete file or a consistent workspace snapshot, and reading files provides no test, acceptance or deployment evidence.

## Editing boundaries

The edit action, completion of asynchronous full-file validation and component rendering check the current API identity, exact scope and write eligibility. A native editor is keyed to its scope and original observation ID. Both controls and action handlers prevent target switching or refreshing while that editor remains open. This preserves the draft, submitted content and original operation ID until the user explicitly closes the editor. A late native read cannot open an editor on a Codex target.

Native saves retain the existing complete UTF-8 baseline, CAS and original receipt recovery. This change does not alter the write contract. A native draft cannot become a Codex editing baseline, and a pending save cannot move to another target. Existing submitted-save receipts remain available through Recent saves.

## Inventory import

The importer checks field allowlists against the generated `ToolObservation` and `ToolExecutionProfile` types. A Codex observation can declare `workspace_observation_protocol=workspace-observation-v1` only with `available=true`, `adapter=codex`, `protocol=codex-app-server-v1`, `read_only=false` and a valid closed public catalog. The tool's coding purpose and read-only workbench file access are separate facts.

Codex cannot declare `workspace-write-v1` or native workspace, model execution or conflict repair MCP capabilities. Generic CLIs, mismatched protocols, unavailable observations and missing or invalid catalogs still cannot advertise file transport. Import only prepares a profile for approval; authenticated current Runner inventory and server authorization determine actual eligibility. Import does not establish connection health, provider identity or task success.

## Verification scope

Focused frontend checks cover a Codex read-only target, exact list/read requests, late native responses after switching, draft and pending original-save isolation, inventory read declarations and write rejection, and language changes. Existing editor recovery checks retain coverage of the original key and body, newer drafts and explicit inspection. Raw evidence stays in the ignored `.cache/e2e/codex-workspace-103/ui/` directory; the integration report records this stage's results.

On 2026-10-28, direct TypeScript and internationalization checks passed (4,615 paired keys, zero production hardcodes), together with five affected Vitest files and 52 cases. Web build and browser integration checks remain with the integration stage and were not run in this frontend subtask.

This workbench change does not establish vendor execution, physical multi-host operation, production connectivity, a complete IDE, Codex file saving or a complete Codex conflict-repair lifecycle.
