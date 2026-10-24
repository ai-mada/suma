# Suma engineering instructions

Read `memory/current_state.json`, `docs/delivery/status.md` and the relevant domain contract before work. The accepted target is `docs/delivery/implementation-plan.md`.

- Product name: 速码一栈式智能研发平台, short 速码, English suma.
- Preserve other agents' files and user changes. Work is divided by `contracts/implementation-contract.md`.
- Business state, run state, test evidence, human acceptance and deployment are distinct facts.
- No fake metrics, silent provider fallbacks, configured-as-healthy indicators or simulated production acceptance.
- Project/tenant authorization is server-side. Never commit credentials, databases, local logs or dependency caches.
- Mutations need versions and stable operation IDs; uncertain external side effects require reconciliation.
- Keep process-local build/test temp/cache under `.cache/`. Never create task directories at drive roots.
- Run meaningful tests for changed contracts; record actual commands/environment and remaining verification in docs/quality/verification.md.
- Keep docs and capability status accurate. A planned interface is not an implemented integration.
- Do not commit, push, publish or delete user files unless authorized in the active session.
- The user authorizes a local commit after each verified functional stage. Include the matching design documents and evidence, use `type(scope): desc [T-id]`, and keep remaining platform work explicit. This does not authorize a push or publication.
