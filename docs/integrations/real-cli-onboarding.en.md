# Real CLI onboarding and read-only diagnostics

T-REAL-CLI-ONBOARDING-226, suma 1.0.0. This guide covers an installed tool's discovery, metadata probes and paused registration. The [stage contract](../../contracts/real-cli-onboarding.md) defines the exact wire fields. [中文](real-cli-onboarding.md) · [Troubleshooting SOP](../operations/real-cli-onboarding-sop.md).

## Observed installation

The following Codex candidate was read from the local OpenAI application directory. No copy of the executable, installer or account file was needed.

| Field | Observation |
| --- | --- |
| Path | `C:\Users\Administrator\AppData\Local\OpenAI\Codex\bin\22cfb1d302613eae\codex.exe` |
| Size | 368970544 bytes |
| SHA256 | `3097c909fb6d5cd63510d44aceb1d42df9f6e85722a3e447e477def1ecf60e16` |
| File metadata | Ordinary file with the Archive attribute; no readable PE version resource |
| Embedded certificate subject | OpenAI OpCo, LLC; certificate metadata only, without chain, revocation or signing-timestamp validation |

These facts identify a candidate, not its actual CLI version, protocol, account or model readiness. PATH differs between processes; a missing PATH result does not prove that a tool is absent. A Cursor editor launcher is not evidence of an agent or ACP runtime. Small task-owned test programs in repository caches are excluded from supplier installation evidence.

## Discovery to paused registration

1. Select the exact tool and node in Tool Management. Use the existing local discovery or remote installation scan. Discovery grants no installation, configuration-write or task permission.
2. Version, help and fixed protocol probes use the original process controller. Without an explicitly trusted internal context, a private scope provides HOME, account configuration directories, temporary directories and the working directory. An arbitrary HOME in a command configuration is not trusted.
3. Existing Codex, SDK and Cursor helpers explicitly pass their owned directories. In particular, Codex feature-list checks keep their prepared configuration rather than having a maintenance collector replace CODEX_HOME. Versions come only from bounded stdout; stderr remains bounded and redacted and cannot become the version's first line.
4. When preparation is needed, explicitly approve configuration and the machine write through the [existing restricted preparation flow](../architecture/managed-tool-preparation.md). The original factory, immutable generation and source checks remain authoritative. A `prepared` result still awaits its inventory acknowledgement.
5. After the exact associated inventory and ACK complete, use the original Quick flow to save a paused profile. Enabling and dispatching remain separate actions with their own current authorization and source checks.

Discovery, file creation, generation acceptance, inventory acknowledgement, registration and enablement are distinct facts. A generated file digest is not a Profile digest or proof of current inventory. When an acknowledgement is lost, reconcile the original operation and exact body; do not regenerate files or use a new operation key as a retry.

## Actionable diagnostics

A classified failure exposes only `diagnostic: {stage, code, next_action}`. Stages are `resolve`, `source`, `version`, `help` and `protocol`; the [contract table](../../contracts/real-cli-onboarding.md) defines valid combinations. The UI renders fixed Chinese or English text, without machine paths, argv, environment values or raw stderr.

| Action | Meaning |
| --- | --- |
| `guide` | Open the existing guide for the exact tool and target node, then address the observed problem |
| `scan` | Explicitly recheck locally or request a single-tool remote scan when current authority, source and maintenance state permit it |
| `none` | Process stopping is unconfirmed; retain the original state and reconcile it before another probe |

Permission classification requires an actual system error, not a message-text match. An unresolved installation source is `scan_incomplete`, not “not installed.” An absent diagnostic means no classified failure is available; it does not mean healthy. An unknown operation, pending receipt or busy state takes precedence over a suggested action.

## Limits and evidence

The complete onboarding evidence currently covers Windows. On Unix, an empty process group does not prove detached descendants stopped. The original collector cannot complete tree-stop proof, and consecutive probes sharing a trusted scope are blocked after its first return; directories are retained. Standalone probes now also return typed stop_unverified when full-tree stop proof is unavailable; captured text is not reported as success. This known degradation requires a separate mature containment and reconciliation implementation; see the [report](../quality/real-cli-onboarding-verification.md).

Private probe directories prevent metadata startup from consulting existing account directories. They are not an operating-system sandbox or proof that all built-in tool behavior is restricted. Independent directories are removed only after reliable original wait/stop facts. Unknown stops retain directories and maintenance records; collector-owned state follows the original Journal lifecycle. A missing PID or elapsed deadline is not stop proof.

The installed Codex returned `codex-cli 0.166.0-alpha.5.1`. One original HTTP/SQLite/ordinary maintenance Worker run completed scan, preparation, bound inventory acknowledgment and paused registration. All 18 focused checks and the two original representative groups and one additional history-protection group passed; see the [evidence report](../quality/real-cli-onboarding-verification.md) for exact scope and limits. Owned test programs cover isolation, stdout and safe diagnostic projection separately from supplier acceptance. Model execution, login, billing, independent code testing, human acceptance, physical multi-host and other operating systems require their own evidence. The full 31 requirements and P0–P7 remain active.
