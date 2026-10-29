# Management and tool discovery implementation plan

**Goal:** Deliver independent management workspaces and reduce manual execution-tool onboarding using current authorized Runner reports.

**Architecture:** Reuse current settings routes, domain components, project authorization, immutable ToolProfile creation and usage ledger. Add one navigation catalog and an independently scoped model directory; expose only already-reported public inventories through current private/Pool authorization. No remote execution or auto-enabling.

**Tech Stack:** Current React/TypeScript, Go control service, SQLite and existing Runner.

**Spec:** `docs/design/management-and-tool-discovery.md`.

## Global constraints

- Preserve unrelated edits; commit each verified phase with its design and compact evidence.
- Keep tenant/project authority server-side, separate configuration/observation/dispatch/acceptance facts.
- Never expose tool paths, arguments, environment values or model secrets in public responses.
- Task builds/snapshots stay under ignored `.cache/` or the task's explicitly writable local artifact root when disk space is limited; deliverable evidence under `dist/quality/`.

## Stage T-MGMT-131

- [x] Add navigation-catalog and actual App tests for independent domain selection, page tabs, guarded navigation and existing links.
- [x] Add management sidebar/page metadata, project directory and current model-directory view; preserve existing governance, tools and billing components.
- [x] Add paired language resources, responsive styling and SVG structure diagram.
- [x] Verify targeted tests, types/languages/build, desktop/mobile and actual navigation; review and commit. Evidence: `docs/quality/management-workspaces-verification.md`.

## Stage T-DISCOVERY-132

- [x] Inspect current inventory storage and authorization; freeze minimal read contract before editing.
- [x] Test current project/private/Pool scopes, expiry, no report, revocation and public data boundary before implementing a bounded read.
- [x] Integrate selected-worker automatic reads with strict parsing, generation isolation, explicit source adoption and file/paste fallback.
- [x] Verify real local probe/Runner report → browser discovery → paused registration → explicit enable, failure recovery and current authority.
- [x] Update contracts, state and compact evidence; independent review and local stage commit. Evidence: `docs/quality/tool-discovery-verification.md`.
- Remote push pending explicit current authorization after automatic review rejected remote transfer; local commits proceed independently.
