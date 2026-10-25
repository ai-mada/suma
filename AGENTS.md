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

## Efficient implementation and validation

- Prioritize parallel feature delivery with explicit file ownership; keep shared interfaces under one integrator.
- Run focused checks for changed behavior. Reuse existing results when relevant inputs have not changed; do not repeat unrelated full suites, builds, source snapshots or reviews.
- Run a complete suite at an integration milestone, or when shared contracts, dependencies or unresolved failures justify it. After a fix, rerun affected checks first.
- Preserve meaningful authorization, transaction, migration, concurrency and idempotency regressions; do not add tests that merely repeat the implementation or validate reversible copy edits.
- Use browser verification for changed interactions and layout; choose representative affected states. Reserve the complete device/theme/journey matrix for release acceptance.
- Keep one compact evidence summary per stage. Read current state and relevant source sections; load historical verification only when needed. Summarize logs instead of repeatedly dumping them.

### 活动预览与构建隔离

- UI 验证构建先确认输出目录是否被活动预览使用；优先向本次任务的忽略目录传 `--outDir`，不能把未联调的新前端覆盖到已验收旧后端的静态目录。
- 若发生覆盖，保存新入口，按已提交摘要核对并恢复旧 HTML／资产；不要重启服务或修改数据库来掩盖版本混用。
