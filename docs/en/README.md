# suma documentation

Status: implementation in progress. The platform's full accepted scope is tracked in the [implementation ledger](../delivery/implementation-plan.md); a completed local stage does not constitute acceptance of the whole platform.

suma is an intelligent development platform for organizations, teams, projects, requirements, design, execution, testing, human acceptance, releases and issue feedback. Business acceptance, execution state, test evidence and deployment are distinct facts.

The current internationalization work introduces Simplified Chinese (`zh-CN`) and US English (`en-US`) with extensible language catalogs. UI language, business time zone and source content are separate concerns. Original user documents, code, tool output and immutable evidence retain their original language. Unsupported legacy diagnostics are explicitly marked rather than silently replaced by an imprecise translation.

The T-P4-006 baseline verified 671 bilingual frontend keys and 93 backend messages for authentication, common records, shared pools, CLI and desktop foundations. Its two-project fixture completed parallel development, candidate tests, test-account reviews and deduplicated feedback; real vendor models, stakeholder sign-off and production acceptance remain separate. The following stage adds persisted account/organization preferences and domain catalogs for workflows, tools, scheduling, team governance and planning. T-P4-008 then migrates the remaining 18 owned frontend modules, with 2,798 keys per language and 123 backend messages. It verifies structured errors, actual draft preservation and a local Monaco TypeScript edit/save/refresh flow. The bounded backend inventory still contains 409 unmatched source-message candidates; library menus, native surfaces, notifications, installers and channels remain separate work. T-P4-009 adds 447 exact static messages and 10 explicit parameterized keys (580 backend keys total), with strict cause/source binding and a bounded Go AST coverage gate. Actual rejected requests and login errors were verified in both languages; sign-in alternatives now have separate 44px targets. The gate excludes other producers, non-Chinese and raw dynamic diagnostics; this remains partial platform coverage. See the [current localization report — Chinese original](../quality/backend-diagnostics-verification.md) for exact tested scope and remaining legacy surfaces; the earlier [pool summary](../quality/execution-pool-summary.json) remains historical evidence.

- [Full design index — Chinese original](../README.md)
- [Internationalization requirements and acceptance — Chinese original](../design/internationalization.md)
- [Account and organization language preferences](locale-preferences.md)
- [Generated HTTP/OpenAPI contract](../api/openapi.json)
- [API integration guide — Chinese original](../api/README.md)
- [Current delivery status — Chinese original](../delivery/status.md)

This is an English documentation entry point. Detailed design documents have not all been translated; the links above explicitly identify the authoritative original. Saved account and organization preferences are implemented; immediate device synchronization, library and native controls, installers and external channel templates require their own recorded implementation and verification.
