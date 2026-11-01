# Immutable tool configuration handoff

Status: T-TOOL-REPLACEMENT-200 scoped implementation and bounded verification complete. Suma1.0.0. The complete31-requirement/P0–P7 goal remains active. See the [contract](../../contracts/tool-profile-replacement.md), [requirements](../product/tool-profile-replacement.md), [architecture and AI SOP](../architecture/tool-profile-replacement.md), and [design](../design/tool-profile-replacement.md).

Prepare an explicit old-to-new configuration handoff in one approved transaction. Fix the original profile version, choose a fresh exact target and either create a paused profile or explicitly reuse an existing profile/version. Reuse preserves its current enabled/paused state. Pausing the original is an explicit choice; it neither rewrites existing task plans nor proves a process stopped. Activation remains separate.

The original actor, body, path and operation key bind recovery. Current authorization precedes receipt lookup, and an existing receipt precedes new freshness/CAS checks. Target creation, optional original pause, immutable relationship and receipt commit or roll back together. A result exceeding the later read limit must be rejected before commit.

Outgoing and incoming keyset history show immutable preparation snapshots and the original committing actor. They have no fake total, grant no execution authority and do not re-observe current health. The existing Quick UI and global write recovery remain the only interaction path.

`tools replace --id OLD --file JSON|- --confirm` submits the original intent without automatic keys, retries or pre-reading replacement versions. `tools replacements --id PROFILE` reads bounded history. New-profile activation, real tool execution, vendor/model health and full distributed production acceptance remain separate facts.

See the [verification report](../quality/tool-profile-replacement-verification.md) for the 2 MiB relationship bound and remaining real-provider, database, distributed and production acceptance.
