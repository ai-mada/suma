# Native model execution and attempt budgets

Status: T-P4-036 / 1.0.0, bounded local implementation verified（限定本地实现及验证通过）. This document describes the implemented boundaries; [stage verification](../quality/model-execution-verification.md) owns actual results and remaining checks. See the Chinese [product scope](../product/model-execution.md), [architecture and implementation checkpoints](../architecture/model-execution.md), and [AI SOP](../operations/model-execution.md).

## Selection and authority

An explicitly enabled native Suma workspace tool can select an immutable model deployment in the existing task dispatch flow. The developer chooses the deployment, execution limits and optional attempt budget, reviews the fixed plan, and explicitly starts it. External Codex, Harness and CodeBuddy CLI tools retain their existing configuration; this independent model selection path does not establish which models those external tools execute.

Options come from the approved tool's current, authorized worker observation and the public portion of its administrator-owned local deployment allowlist. Public fields include deployment ID, revision, SHA256, provider protocol, exact model ID, endpoint origin, a digest of the full base URL, and the declared context window. Private endpoint paths, API keys and credential environment references are not published. Existing project, tool activation, worker, route and Pool authorization apply; there is no separate global model registry or model grant marketplace.

```powershell
suma --control https://suma.example --project PROJECT_ID --language en-US models options --tool TOOL_ID --expected-tool-version TOOL_VERSION
```

Use an existing project credential from the current process's `SUMA_TOKEN`. The equivalent read is `GET /api/v1/model-execution/options` with `project_id`, `tool_profile_id` and `expected_tool_version`. Reading options observes inventory and does not call a model. The response must match the original project and complete tool scope.

## Immutable plan and real execution

`model_execution_selection` identifies the deployment ID/revision/digest and the closed parameter set: `max_steps` and `max_output_tokens`. The ordinary dispatch flow defaults to 8 steps and 4096 cumulative output tokens, with limits of 16 and 32768. Other sampling, reasoning, endpoint and environment overrides are rejected. Existing repair limits do not grant repair capability to ordinary dispatch.

The server freezes the deployment, parameter digest, complete binding digest, budget, reservations and matching operator rate into the plan and derived agent node. Start rechecks current authority and deployment eligibility. Claim performs quota admission and attempt reservations atomically. Before workspace/process startup, the Runner matches the exact local deployment and resolves its separate credential reference. The native CLI receives that binding and local execution configuration; ambient model settings cannot silently replace them.

The provider adapter sends the selected model in the real OpenAI-compatible `/chat/completions` request. Each call uses the remaining cumulative output allowance, capped by the per-call allowance and declared context window. Complete counters may reclaim unused output allowance; missing or partial usage retains the conservative allowance. The Runner records binding/deployment/parameter digests, request and response hashes, provider model/request identities and usage coverage.

The returned provider model must exactly equal the selected model. Missing identities and aliases mapping to another returned ID are rejected without automatic translation or fallback. Already reported counters remain facts even when identity validation fails. Code candidate tests, human Review and formal acceptance remain separate.

## Budget and pricing limits

Input reservation is `context_window × max_steps`; output reservation is the cumulative output limit. A token budget must cover their sum. A cost budget requires a matching currency and exact frozen rate, and covers the integer ceiling of the two reservation quantities multiplied by their input/output rates per million tokens.

This is conservative attempt admission using an administrator-declared context bound. The runtime also rejects reported input/output beyond that bound. It is not a model-specific tokenizer or a provider-side hard spending gateway.

Rates are operator-configured, versioned per project/tool. New options read one current rate for that tool and mark only its exact matching deployment as `priced`; other allowed deployments remain `unpriced`. This is not a complete multi-model quotation catalogue. A fixed plan and attempt retain the original rate ID/version when later rates are created. Cost is an estimate under that operator rate, not a provider invoice; cached/reasoning details do not constitute independently priced tiers.

Organization and project enforce policies apply during admission. Unpriced execution cannot satisfy a monetary budget or a policy requiring matching pricing. Unknown cost is never displayed as zero. Partial, unreported, unknown, mismatched or overflowing evidence retains unresolved facts and applicable holds. Existing authorized manual usage confirmation requires an evidence source, reason, expected version and stable operation key, and cannot reduce already reported counters. It does not certify execution or acceptance.

## Read usage evidence from the CLI

Use an existing workbench login session from the current process's `SUMA_TOKEN`. Keep the same project, user and organization when reading the attempt list and evidence.

```powershell
suma --control https://suma.example --project PROJECT_ID --expected-user USER_ID --expected-organization ORG_ID --language en-US usage attempts --month 2026-11 --state confirmed --limit 20
suma --control https://suma.example --project PROJECT_ID --expected-user USER_ID --expected-organization ORG_ID --language en-US usage evidence ATTEMPT_ID --limit 20
```

Copy an attempt `id` from the first command into the second. Each command returns one JSON document. To read another page, pass its `next_cursor` as `--before` with the original scope and filters. Evidence keeps server order and distinguishes the original result, subsequent reconciliation records and manual confirmations. Missing counters stay absent. These reads use archived Suma records without contacting the provider.

## Implementation and delivery boundary

The bounded provider adapter reuses the existing cancellable fetch transport and raw usage evidence. Workspace access reuses the official MCP SDK. No official OpenAI or Agents SDK has been added, and local protocol fixtures do not establish vendor authentication, model quality, invoices or production acceptance.

Integration proceeds through four checkpoints: shared binding and scoped options; exact plan-to-native-CLI execution; atomic attempt admission and frozen-rate settlement/reconciliation; original UI/CLI interactions with actual evidence. Results are recorded once in [stage verification](../quality/model-execution-verification.md).

The separate [developer MCP check](../integrations/developer-connection-check.md) performs a real one-shot read diagnostic. It does not complete remote workspace access or a persistent IDE connection. The [outbound remote workspace broker](../architecture/remote-workspace-broker.md) is a future design and remains unimplemented.
