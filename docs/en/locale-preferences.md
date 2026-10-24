# Language preferences

This guide covers account and organization display languages. The authoritative Chinese sources are the [preference contract](../../contracts/locale-preferences.md), [interaction design](../design/locale-preferences.md), and [platform internationalization requirements](../design/internationalization.md).

## Choose a language

Open **Organization & project settings → Language & preferences**. This page remains available when no project is selected or accessible. The page shows the effective language and its source, with separate account and organization forms.

The effective priority is:

1. Explicit language selected in this browser.
2. Saved account language.
3. Current organization's saved default.
4. Browser or request language negotiation.
5. Product default, `zh-CN`.

The quick language selector changes the browser override. Choose **Follow defaults** to remove that override. Saving an account language does not remove an existing browser override. A language read failure is unknown, rather than proof that the saved setting is empty.

An account preference belongs to the global user and follows that user across organizations. Organization defaults apply only to the current organization; its owner or administrator can change them. Other members can read the organization default and edit their own account preference. Choose **Inherit default language** to save an empty preference. Other devices use saved settings after reading or refreshing them; immediate device-to-device updates are not promised.

Changing language preserves drafts, selected runs, nodes and attempts. User text, code, tool output, identifiers and immutable evidence retain their original content. Business time zones remain independent.

## Integrate with the API

Authenticated human browser sessions use these endpoints; machine credentials do not gain preference-management permissions:

| Endpoint | Access |
| --- | --- |
| `GET /api/v1/account/preferences` | Current user |
| `PATCH /api/v1/account/preferences` | Current user |
| `GET /api/v1/organization/preferences` | Current organization member |
| `PATCH /api/v1/organization/preferences` | Current organization owner or administrator |

PATCH bodies contain exactly `locale`, `expected_version`, and `idempotency_key`. Supported saved values are `zh-CN`, `en-US`, and `""` for inheritance; supported aliases are normalized. Read the current version before writing. An unsaved preference has version `0`; clearing a saved preference advances its version rather than resetting it.

Both PATCH endpoints require `X-Suma-Expected-User`. Organization PATCH also requires `X-Suma-Expected-Organization`. Account PATCH has global user scope and does not send an organization header, so the same user can recover an account operation after switching organizations. GET does not require these expected-identity headers. These headers bind the intended identity to the authenticated session; they do not grant authorization. `X-Suma-Language` controls an explicit display override, while `Accept-Language` supplies negotiation candidates.

## Keep drafts and recover uncertain writes

Account and organization forms have independent drafts and versions. Background reads do not overwrite edited fields or silently advance their base version. After a conflict, keep the draft, reread the current record, and explicitly prepare a new write.

Before sending, the browser saves and reads back the non-secret recovery envelope in this tab's `sessionStorage`. Storage failure prevents sending. Recovery repeats the original target, body, expected version, operation ID, and expected-identity headers. It does not substitute a newer version or another tab's current cookie identity. Account recovery is scoped to the user; organization recovery is scoped to that user and organization.

A timeout, invalid success response, or refusal during recovery does not prove that the original operation never committed. Keep its envelope and reconcile the original operation. A historical success receipt proves that operation's result, not the current preference value or current permission. Read the current record separately. If the server confirms a write but local cleanup fails, retain the confirmed result and recovery record without allowing a new draft to overwrite it.

## Remaining coverage

Internationalization is still in progress: 18 legacy frontend modules remain pending migration. Their presence must not be presented as complete bilingual coverage. The [Chinese design](../design/locale-preferences.md) explains catalog boundaries and preserved server identifiers; the [internationalization requirements](../design/internationalization.md) define the wider platform scope.
