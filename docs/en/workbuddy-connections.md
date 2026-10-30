# WorkBuddy project connections

T-WORKBUDDY-CONNECTIONS-147, initial suma product 1.0.0. This page describes the current product and interaction design. The [domain contract](../../contracts/workbuddy-connections.md) owns wire behavior; see the [architecture](../architecture/workbuddy-connections.md), [Chinese product scope](../product/workbuddy-connections.md) and [interaction and AI SOP](../design/workbuddy-connections.md). Design documentation does not certify implementation, browser or provider acceptance.

## Ownership and availability

A connection belongs to one tenant, project and current owner. Current project readers can inspect their own safe connection facts. Starting or completing authorization, refreshing and disconnecting require current project write authority. Reading an original authorization attempt also requires that write authority and the initiating browser session because it may return a private authorization link. Administrators cannot use these reads to obtain another owner's private grants. An external WorkBuddy account ID does not create a suma identity or grant project access.

Configured, authorized and dispatchable are separate facts. This stage manages account authorization and does not enable WorkBuddy cloud task dispatch. WorkBuddy cloud and CodeBuddy CLI remain separate integrations. A connected state does not prove model health, a stopped remote task or production readiness.

## Authorize explicitly

1. Expand WorkBuddy account authorization in the current project's tool settings. It starts collapsed; opening it loads only the current owner's bounded connection page.
2. Choose to connect or reauthorize, then explicitly select a nonempty subset of the operator-enabled permissions: read tasks and create tasks. Nothing is preselected.
3. Prepare the authorization link. The original authorization Start only creates the fixed attempt; it neither grants access nor starts a cloud task.
4. In a normal browser, explicitly open the official WorkBuddy page in a new tab. Desktop users explicitly open an isolated authorization child in the same session. Review the account and scopes, then accept or deny. The link has a visible expiry; it is not opened or renewed automatically, and the original session is not transferred to an external browser.
5. The provider returns to an isolated callback document. The server checks the original browser credential, bound nonce and current project authority, then returns to the safe project connection panel. Querying the exact original connection ID reads the result without completing an unknown operation.

Changing account, organization or browser session cannot consume the old authorization. After expiry, read current facts before explicitly starting a new authorization. Denial cancels the original pending attempt without a token exchange. Granted scopes may be narrower than requested; they must not expand silently. Reauthorization cannot attach another external account to the old connection.

## States, reads and recovery

| State or action | Meaning |
| --- | --- |
| Waiting for authorization | The original attempt awaits official confirmation; use its still-valid link or read current facts |
| Connected | A provider grant was saved; cloud dispatch remains unavailable |
| Refreshing | One explicit refresh was prepared; wait for its original result |
| Authorization or refresh unknown | The provider may have acted; inspect the original connection or recover the original operation |
| Reauthorization required | Current eligibility is insufficient; explicitly select scopes and authorize again when allowed |
| Locally disconnected | suma will not use the grant; remote revocation and task termination are separate |

Refresh list is read-only. Refresh authorization is an explicit token operation. A normal refresh advances the token generation rather than the grant generation. Local disconnect requires an impact statement and checked confirmation, clears local credentials and invalidates old operations so late responses cannot reconnect it. It does not revoke the provider grant or stop remote tasks. Current-authorized local disconnect remains available when application configuration becomes unavailable.

Unknown writes retain the original caller, organization, project, version, operation ID and JSON. A control-plane recovery request may recover its original receipt; it does not obtain another provider token POST permit. A prepared or unknown exchange/refresh is not resent after a timeout, restart or lease expiry. Reading current state does not declare the unknown operation successful. Changing fields or using a new operation ID is not recovery.

The list defaults to 20 items, with explicit previous/next controls and no invented exact total. Pages are fresh authorized reads ordered by stable ID, not a cross-page snapshot. Exact-ID reads preserve identity when the original connection is outside the loaded page. Technical identifiers and generations remain in expandable details.

Loading, empty, unconfigured, read errors, permission loss and version conflicts have distinct feedback. Scope changes isolate late responses; permission failures hide protected facts, forms and old links. Pending writes lock competing edits. Unsaved scope selection and disconnect confirmation use existing navigation protection.

## Isolated callback

The browser cookie remains SameSite=Strict. The first cross-site GET only serves a bridge: it consumes no code, exchanges no token and writes no connection. The bridge has nonce CSP, no third-party resources, no-store and no-referrer, and removes the query from the address bar before same-origin requests.

Code and state live briefly in that document's memory. They do not enter the SPA, browser storage, generic drafts or recovery receipts. The temporary official authorization URL also stays in page memory. The bridge reads the current session and language, then POSTs with the current cookie and original-view identity headers. The server validates the original credential and bound nonce, owner, project, scopes and expiry.

Only a fixed local tool-panel route with safe project and connection IDs is returned. Failures offer safe wording and a return link, without raw provider errors or credentials. Proxy logging must treat callback query parameters as credential-bearing data; weakening cookies or origin checks is not a recovery method.

## Desktop session and return

Desktop uses a narrow IPC carrying only `project_id`, `authorization_id`, `user_id` and `tenant_id`. The main process checks the original parent/mainFrame, reads current identity and the exact authorization through the parent's Session, then validates the server-owned account, project, pending state and expiry. Optional local client/scope restrictions only tighten that authority; the renderer cannot supply arbitrary URLs or credentials.

The child explicitly shares the parent Session, with no preload or Node privileges. Other popups remain denied. Only the original state callback and safe project/connection return are accepted. Completion closes the child and focuses the parent without loading a new parent URL or discarding its document and drafts.

Opening, closing or returning does not prove connection success. Focus may GET the exact original connection only when no form draft, unconfirmed write or opening operation is active. Concurrent focus reads are merged; results must match that connection ID and current caller. There is no automatic reload or inferred connected state. If the native capability is unavailable, only the platform address is shared so a normal browser can sign in and start its own authorization; cookies, state, code and tokens are not transferred.

The server reuses mature `golang.org/x/oauth2`; encrypted grants remain in the control plane, with distinct grant and token generations. Unknown exchange/refresh effects never obtain another provider POST permit.

## Operator setup

Configure the control process environment, never a user form or ordinary API payload. This guide contains no live credentials.

| Setting | Requirement |
| --- | --- |
| `SUMA_PUBLIC_URL` | Validated public service origin; the callback is derived as `/api/v1/workbuddy-oauth/callback`, never from request Host or forwarded headers |
| `SUMA_WORKBUDDY_CLIENT_ID` | Registered provider application ID |
| `SUMA_WORKBUDDY_CLIENT_SECRET` | Server-only application secret |
| `SUMA_WORKBUDDY_SCOPES` | Explicit nonempty subset of `user.task.readable` and `user.task.invokable`, separated by spaces or commas |
| `SUMA_WORKBUDDY_MASTER_KEY` | When provided, canonical standard Base64 decoding to exactly 32 bytes; shared databases require the same explicitly managed key |

If CLIENT_ID, SECRET and SCOPES are all absent, the integration stays unconfigured and creates no unnecessary key file. Partial or invalid configuration fails explicitly. With SQLite and no explicit master key, the service creates a separate 32-byte `workbuddy-connections.key` in its persistent data directory. An existing key must be a complete regular file; it is never silently repaired or replaced. Shared databases do not generate independent per-host keys. Preserve the correct key when backing up or restoring. This is not a KMS or key-rotation claim.

Tokens stay encrypted on the server; public projections contain safe allowlisted facts. Application secrets and encryption keys are distinct responsibilities.

## Interaction, AI SOP and remaining work

Chinese/English labels, light/dark styling, responsive wrapping, visible keyboard focus, reduced-motion support and at least 44px action targets follow the existing components. Primary text is 14px and supporting text 12px. IDs, scope values and original facts are not translated. These are implementation/design properties; actual touch, keyboard and visual behavior require separate browser evidence.

AI may explain safe states, scopes and recovery, but must not choose permissions, open authorization, refresh or disconnect without the user's explicit authorization. Users explicitly authorize with the provider. Recovery inspects the original connection and operation; it does not move code/state/tokens through logs, clipboard or browser storage.

The [connection report](../quality/workbuddy-connections-verification.md), [summary](../quality/workbuddy-connections-summary.json) and [native-window evidence](../quality/workbuddy-desktop-oauth-verification.md) distinguish existing module, bridge, component and affected rechecks without adding them again. A local native protocol fixture and 18 actual GUI/HTTP checks against the exact final Go/Web candidate with production preload/controller were verified separately. Synthetic client configuration blocked external provider requests: one authorization POST, the same-session child, unchanged parent document, failure without a connected grant, English/390px DOM and read-only recovery were observed. Earlier fixture success-callback evidence has its own scope; this run proves no successful real provider grant. Visual images, a real provider account, commercial models, physical multihost and production acceptance remain unverified. Cloud dispatch, ACP progress/input/permission requests, execution budgets, cloud artifacts, exact code candidates and independent tests remain to be delivered.

Full P0–P7, requirements 1–26 and 58 limited / 34 pending / 92 total remain active in the [implementation plan](../delivery/implementation-plan.md).
