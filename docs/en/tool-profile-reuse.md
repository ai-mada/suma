# Reuse an existing tool profile

T-TOOL-PROFILE-REUSE-180 · suma 1.0.0. See the [frozen contract](../../contracts/tool-profile-reuse.md), [product flow](../product/tool-profile-reuse.md), [architecture and AI SOP](../architecture/tool-profile-reuse.md), and [interaction design](../design/tool-profile-reuse.md). See the [scoped verification report](../quality/tool-profile-reuse-verification.md); full production acceptance remains pending.

Quick connect finds existing registrations for the current project, exact worker and complete immutable Profile. Reopening the form or reconnecting a worker should not require another registration of the same configuration. Names are display labels, not configuration identities.

## Explicit user flow

1. Select a tool and worker with an exact, fresh inventory observation.
2. Automatically read the complete set of matching profiles under current authorization.
3. Explicitly use an existing profile, or explicitly create one when no reusable match exists.
4. For a paused profile, use the original enable confirmation. For an enabled profile, open its original details and current availability.

An unresolved original create or enable operation takes priority. Restore that operation with its original body and stable key before performing a new match flow.

## Separate facts

| Fact | Meaning |
| --- | --- |
| Matching configuration | The worker and full Profile match; this does not establish health |
| Current availability | Original `Available` and `Reason` reflect current authority, inventory and worker conditions |
| Explicit reuse | The user adopts the original profile identity without copying or editing it |
| Explicit enable | The original versioned confirmation grants dispatch authority independently |

Paused and enabled profiles are reusable. Enabled does not imply currently available. Revoked profiles are history only and cannot be revived; if no paused or enabled match exists, the user may explicitly create a new paused profile. Multiple reusable matches require an explicit choice rather than selecting the first entry.

## Read contract

`POST /api/v1/tool-profiles/matches` accepts the closed request `{project_id,worker_id,profile}`. Current browser-project-read authorization precedes lookup, and full Profile validation applies. Runner and Pool tokens are not accepted. The query creates no business records, operation receipts, approvals or tickets.

HTTP 200 returns `ToolProfileMatchList {project_id,worker_id,profile,items,total}`. Scope and full Profile must match the request. The collection is complete, unpaginated and untruncated; `total` equals `items.length`. Lookup compares all records for the exact tenant/project/worker using `toolcontract.EqualProfile`, retaining the existing 1,000-profile project limit.

A failed or incomplete response cannot be treated as an empty collection. Names, partial digests and the first 200 list entries cannot establish an exact match or its absence.

## Creation races and recovery

Ordinary Create still creates a paused profile. Within its original authorized transaction, it restores the original operation receipt first, then rechecks full matches before INSERT. An existing paused or enabled match causes `409 tool_profile_match_exists`, not another operation's 201 or receipt. Revoked history does not prevent explicit creation.

The UI retains the name draft and reads matches again after this conflict. Existing organization/project authority locks serialize concurrent creation. Original worker and authority gates remain; no new table or JSON uniqueness index is needed.

Unknown writes retain their original body/key and pending journal. Read matching cannot clear or replace them. Enable keeps the original `expected_version` CAS. Authority, state, worker or configuration changes require current facts to be read again, without automatic enable.

## Interaction, CLI and scope

While lookup or an original pending write is active, parent switching and new registration are blocked. Late responses cannot restore an old selection after account, organization, project, adapter, worker, full Profile, inventory deadline or API caller changes. Loading, errors, empty results, multiple choices, revoked history and version states remain visible in both languages with keyboard and narrow-screen access.

`suma --project PROJECT tools matches --file REQUEST_JSON` uses the same bounded request and original client; `--file -` reads standard input. It checks the project, adds no operation key or confirmation, and never creates or enables a profile. A timeout is a read error, not an unknown write.

This slice reuses the existing connection, Worker, inventory and execution chain. Matching or reuse does not establish model execution, candidate delivery, testing, human acceptance or completion of the original 31 requirements/P0–P7 scope. Implementation checks and actual acceptance evidence are recorded separately.
