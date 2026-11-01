# Developer tool handoff and remote Runner setup

T-DEVELOPER-TOOL-HANDOFF-182, suma 1.0.0. A reviewed task discussion can enter the existing development dispatcher. A separate local Runner flow prepares command configuration and reports one inventory. Both use the original profiles, authority and quality workflow. See the [contract](../../contracts/developer-tool-handoff.md) and [verification report](../quality/developer-tool-handoff-verification.md).

## Review a discussion before starting development

Select a completed assistant reply whose exact source is still current. Review the development goal and supplemental acceptance criteria, then choose a workflow, its development node and a currently eligible writable profile. Creating the preview does not start work. Review its fixed source and execution facts, then explicitly start the original run.

The ordinary structured development entries are suma, codex, harness, codebuddy, fixed Qoder ACP, Claude, OpenCode SDK and Octop. Eligibility still depends on the exact profile, current inventory, authorization, route and workflow. Detection-only entries such as Cursor gain no execution permission.

Native suma and Octop use their existing model deployment, parameters and budget selection. Codex App Server uses its current catalog, model, reasoning effort and configuration digest; its usage is observed only, and enforced-budget policies remain blocked. Codex JSONL and the other fixed configurations retain their original profile settings rather than inventing another selection. The request accepts optional native or Codex selection, rejects both together, and lets the existing dispatcher decide which selection is required or forbidden.

The exact reply, message, versions, source digest and approved knowledge pins remain fixed. The original resolver checks current publication and access before task context, model bindings and Codex native skill attachments are frozen. The form cannot inject arbitrary knowledge. This stage preserves that existing construction order and verifies a source containing both memory and a native skill.

Edited criteria supplement the engineering objective; they do not replace formal task scope, published tests or human acceptance. The original isolated workspace, candidate sealing and transfer, independent testing of the same commit, formal Review, human acceptance and downstream baseline handoff remain separate steps. A completed model response or process is not task acceptance.

If preview creation or Start has an unconfirmed response, recover the original body and operation key first. Recovery returns the original plan or run; changing tools or keys is not a substitute for resolving an uncertain operation. Changed task, source, tool or catalog facts require review while preserving the draft.

## Prepare local configuration

On the target machine, use:

```text
suma-runner --prepare-tools PATH [--local-tool ID]
```

PATH must be a new local file in an existing directory without linked ancestors. Preparation never creates parent directories or overwrites existing files. It writes the original non-secret command allowlist format, without reading Runner credentials, opening the journal, connecting to the control plane or claiming a task.

Only four directory entries can currently yield automatic configuration: `codex-jsonl`, `codebuddy-cli`, `dsh-multica` and `claude-code-cli`. Each still needs actual installed, launch-supported discovery with a returned command. Configured Codex App Server, Qoder, OpenCode and Cursor remain configuration-required or unsupported observations. No generatable commands means no file, an explicit unprepared result and a non-success exit. Preparation alone does not prove fresh inventory, model login or dispatch permission.

## Report or recover one inventory

The target machine needs an already registered project/worker credential in `SUMA_RUNNER_TOKEN`, its original state directory and the command configuration. Use `--report-tools` with the exact `--control`, `--project`, `--worker`, `--commands` and `--state` values. Writable tools also require the administrator Git checkout, full immutable baseline commit and usable workspace; provide the original `--repository`, `--base-commit` and applicable workspace settings.

The browser guide creates PowerShell or Bash drafts from public location fields and references the credential environment variable without copying its value. Review and run these commands yourself. A loopback control address works only on the same machine; another machine needs a reachable secure control address. The guide does not install tools or run copied commands automatically.

`--allow-exec` authorizes the original fixed executable, protocol and workspace probes. Omitting it reports actual disabled observations. Reporting, continuous connection and explicit profile enablement are separate actions; an acknowledged inventory is not model health or execution authority.

For a new inventory, the original journal lock protects a durable operation before the HTTP report and ACK. If an inventory is pending, recovery reuses its exact operation and body before reading current command or model files. That single invocation returns the original ACK and stops; it does not probe or send another inventory. Preserve unknown results and other pending task records.

The required local `scope_sha256` binds the validated control URL, including its exact API path, project and registered worker; trailing slashes are normalized and credentials are excluded. Missing or different scope is rejected by journal storage, single and continuous reporting, HTTP transport and read-only inspection. It is not backfilled, cleared or replayed under a different endpoint, project or worker. Credential renewal alone does not change the scope.

This mode never enters the Worker loop, claims work or sends task heartbeats. The browser consumes the original inventory through exact profile matching, paused registration and separate explicit enablement. There is no new public endpoint, scheduler or compatibility alias.

## AI SOP and current evidence

AI-assisted work must preserve exact source pins, current authority, versions, original operation receipts and independent quality gates. Recover uncertain writes first; do not turn discussion text into extra shell, path, tool, budget or acceptance permissions. Record configuration preparation, inventory acknowledgement, connection, enablement, execution, candidate, independent tests and human acceptance as separate facts.

Focused evidence covers production HTTP/SQLite/dispatcher behavior with controlled protocol receipts, exact source skills, original Start/Claim/recovery, real local files and journal locks, a Windows directory junction and controlled HTTP reporting. Existing same-commit candidate-test evidence is reused within its original limits. Component checks and representative browser evidence are reported separately.

These checks do not establish a real vendor model/account/bill, a complete authenticated browser-to-remote-Runner journey, a physical remote host, multiple machines, other databases, cross-platform installation or production scale. Those remain separate acceptance work. The full original 31 requirements and P0–P7 scope remain active.
