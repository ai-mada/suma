# suma documentation

Status: implementation in progress. The platform's full accepted scope is tracked in the [implementation ledger](../delivery/implementation-plan.md); a completed local stage does not constitute acceptance of the whole platform.

suma is an intelligent development platform for organizations, teams, projects, requirements, design, execution, testing, human acceptance, releases and issue feedback. Business acceptance, execution state, test evidence and deployment are distinct facts.

The current internationalization work introduces Simplified Chinese (`zh-CN`) and US English (`en-US`) with extensible language catalogs. UI language, business time zone and source content are separate concerns. Original user documents, code, tool output and immutable evidence retain their original language. Unsupported legacy diagnostics are explicitly marked rather than silently replaced by an imprecise translation.

The T-P4-006 baseline verified 671 bilingual frontend keys and 93 backend messages for authentication, common records, shared pools, CLI and desktop foundations. Its two-project fixture completed parallel development, candidate tests, test-account reviews and deduplicated feedback; real vendor models, stakeholder sign-off and production acceptance remain separate. The following stage adds persisted account/organization preferences and domain catalogs for workflows, tools, scheduling, team governance and planning. See the [current localization report — Chinese original](../quality/locale-preferences-verification.md) for exact tested scope and remaining legacy surfaces; the earlier [pool summary](../quality/execution-pool-summary.json) remains historical evidence.

- [Full design index — Chinese original](../README.md)
- [Internationalization requirements and acceptance — Chinese original](../design/internationalization.md)
- [Account and organization language preferences](locale-preferences.md)
- [Generated HTTP/OpenAPI contract](../api/openapi.json)
- [API integration guide — Chinese original](../api/README.md)
- [Current delivery status — Chinese original](../delivery/status.md)

This is an English documentation entry point. Detailed design documents have not all been translated; the links above explicitly identify the authoritative original. Saved account and organization preferences are implemented; immediate device synchronization, every legacy screen, native installers and external channel templates require their own recorded implementation and verification.
