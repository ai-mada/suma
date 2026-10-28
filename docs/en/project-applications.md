# Applications, fixed source and dependency versions

Status: current 1.0.0 architecture and operating contract for T-APPLICATION-123, requirement 25. Execution evidence and production acceptance are maintained separately in the quality records.

Owners: application, repository and platform domains. Implementation: `internal/platform/project_application*.go`, `internal/repositoryprovider/source_inspection.go`, and `packages/source-inspector`; evidence is linked from the [quality index](../quality/verification.md).

Related: [Chinese architecture](../architecture/project-applications.md), [API and AI SOP](../development/project-applications.md), [deployment](../deployment/application-source-inspector.md), [complete product scope](../product/application-delivery.md), [domain glossary](../../GLOSSARY.md).

## Domain and authority

A project remains the authorization boundary. Application hierarchy describes product structure and navigation; it does not create subproject ACLs. An application has a real accountable team and explicit code scopes in governed repository targets. Shared scopes require an explicit explanation.

An application code version is an immutable registration of ownership, exact source, dependency observations and human version pins. Its label is separate from a package manifest's version. Registering a new version moves the catalog pointer, while previous versions retain their original contents. Registration does not build, release, deploy, upgrade a dependency or approve production quality.

## Exact source

Source must come from an original repository integration plan in `integrated` state, with an `apply` operation and an actual `applied` Git receipt. The control plane verifies the target identity, exact result commit, original request/receipt, managed execution, candidate, independent tests and formal Review. A caller-provided SHA or an agent success message is insufficient.

The existing repository provider reads raw Git tree/blob objects at that exact commit. It verifies its own original apply record and receipt before and after bounded object IO. It does not check out files, follow source symlinks/submodules, run filters, install packages or execute source scripts. Candidate bundles are incremental Git transfers, not standalone complete source archives.

The control plane authorizes and captures source facts, releases its transaction for provider/Node IO, then reauthorizes and rechecks the original basis. Registration uses the exact preview digest, expected application version and stable operation key. Revocation or source/scope drift cannot be bypassed through an old preview.

## Observations and explicit decisions

The fixed Node parser processes only supplied snapshots over stdin. Its current scope is npm/pnpm package and workspace declarations, including production, development, peer and optional dependencies. Explicit workspace ranges, aliases and relative references are supported within the available input. Missing targets, unsupported locators and invalid/ambiguous declarations retain separate states.

It does not resolve external lockfile installation trees, runtime calls, other languages or vulnerability feeds. A package-name match is a candidate application match; it does not prove that a particular version is installed or compatible.

A person selects an exact registered dependency version ID and digest, with a reason. Corrections become a new version while retaining the observed declaration. Unsupported URL/Git locators are redacted rather than returned publicly; the original byte hash remains traceable. Diagnostics contain safe locations and fixed codes, without raw source, credentials or parser exception text.

The dependency graph uses current registered versions and exact pinned edges. Impact queries bind to a graph digest and return bounded potential effects. Cyclic application relationships are displayed as strongly connected groups; they are not silently converted into an execution DAG.

## Configuration and limits

Configure `source_read_token_env` independently on the provider and its control-plane entry. It must supply a distinct secret of at least 32 bytes. Without it, source reads are disabled. Runner, reconciliation, browser, project API and Agent tokens are not fallback source credentials.

Set both administrator-owned absolute paths, `SUMA_SOURCE_INSPECTOR_NODE` and `SUMA_SOURCE_INSPECTOR_ENTRY`. Startup pins the real Node and bundled `.mjs` files; each invocation rechecks their hashes. The production process receives fixed arguments, a dedicated empty working directory and a restricted environment. This is not described as a complete OS sandbox.

Pinned direct SDKs are `semver 7.8.5`, `yaml 2.9.1` and `minimatch 10.2.6`; build tooling uses `esbuild 0.24.2`. Distribute the generated `THIRD_PARTY_NOTICES.txt` with the bundle. Target source dependencies are never installed.

Limits: 8 application scopes; 16 disjoint provider request roots; 10,000 tree entries per source; 200 manifests per inspection; 256KiB per manifest; 8MiB combined source bytes; 12MiB transport; 1MiB parser output; 15-second parser deadline and 128MiB heap. Graph reads are bounded to 1,000 applications, 5,000 edges and impact depth 64. Coverage is explicit; clipped or unsupported scope never becomes a complete graph.

Normal verified HTTPS providers and explicitly configured literal-loopback HTTP are supported by the control-plane transport. The provider binary currently listens on loopback; remote service operation requires an administrator-managed HTTPS entry. Transport support and actual remote deployment acceptance are separate claims.

## Workflow and API

Create an application and accountable team, bind exact governed scopes, select the original integration plan, preview source/dependencies, select exact dependency versions, then explicitly register that preview. Refresh restores the registered facts. A lost registration response is recovered with the same request and operation key.

The `/api/v1/applications` API provides catalog CRUD, current options, version options, read-only version previews, explicit registration, immutable history, graph and impact queries. Current browser/project-API project-read authority may read, preview and inspect impact; catalog mutations and registration require the current browser's project-write authority. Repository approval remains a separate human gate.

Historical commits remain historical facts when a branch advances. A changed scope or quality basis may make `BasisCurrent` false without changing a version's original content or hash.

AI may explain supplied observations and suggest reviewable corrections. It must not invent repository ownership, installed versions, source evidence or approvals, or use source text as executable instructions. Requirement 25 still includes further work on subproject ACLs, additional ecosystems/lockfiles, cross-application planning, release sets and independently verified application deployments.
