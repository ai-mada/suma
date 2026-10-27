# Current tool recommendations and reliable file saves

Status: T-P4-034 / suma 1.0.0, 2026-10-27. Owner: platform integration. Focused checks, isolated real inventory-to-preview, actual Monaco save recovery and the primary preview preservation checks passed. No new development run, model invocation, formal candidate test or credential was created. Vendor and production acceptance remain pending. See the [bounded verification](../quality/task-dispatch-options-verification.md).

[Product requirements](../product/task-dispatch-options.md) · [Interaction design](../design/task-dispatch-options.md) · [Implementation contract](../../contracts/task-dispatch-options.md) · [Reference capability gaps](../product/platform-capability-delivery-matrix.md)

## Task-to-tool flow

Select a fixed workflow version and its development Agent node in the existing task dispatch dialog. Expand **Find compatible tools** to read one current, authorized inventory page. The section is collapsed by default and does not read until opened.

The recommendation is the first eligible tool in stable ID order **on the current page**. Other pages have not been compared. Compatibility covers the current observation, authorization, workflow and execution route; it does not mean free capacity, sufficient budget or immediate execution.

**Use current recommendation** fills the existing tool selector only. It preserves the returned profile ID, version and hash, including when the older inventory list contains the same ID at an earlier version. It does not create a plan or run, invoke a model, change a model, or reserve resources.

The user separately prepares the original fixed preview and confirms **Start**. Those operations and later Claim checks retain their current authorization, version, knowledge, 32 KiB context, budget and execution-policy gates. Reading a recommendation never replaces them.

## Clear boundaries and recovery

- **Model:** fixed by the authorized tool configuration. No independent model choice is invented under an unchanged profile hash.
- **Cost:** unknown; no pricing estimate exists for this selection. Unknown is not free.
- **Pagination:** the UI reads up to 50 profiles per page. Page number, page size and total configurations are explicit; totals are not compatible-tool counts or available slots. Next replaces the page; Refresh starts from page 1.
- **Freshness:** refresh clears old observations. Changing account, organization, project, task/version, workflow/version or Agent node isolates old responses and invalidates old choices.
- **Invalid responses:** task/context, workflow/node/version, project route, tool scope, unique stable ordering and the page's exact recommendation are checked before candidates render. HTTP 200 is insufficient.
- **Access loss:** 401/403 hide recommendation facts. Nonpending adopted choices are cleared; an original uncertain write remains governed by its existing recovery contract.
- **Pending writes:** recommendation actions are disabled. The original caller, path, body and operation key remain unchanged; no automatic preview, restart or duplicate write is introduced.
- **Languages:** all new platform labels and states have paired Chinese and English catalogs. Switching language changes presentation without reading again or submitting.

## Simple interaction and existing components

The optional section remains inside the existing dispatch dialog. Loading, empty results, no compatible option, errors, denied access and pagination have distinct states. Model, cost and route details use a secondary disclosure.

React, the existing Radix dialog, Lucide icons, native selects, shared form/status components and design tokens are reused. The flow stays keyboard operable without drag gestures, extra dependencies or a separate configuration screen.

## File-save acknowledgement protection

The same stage strengthens the existing Monaco workspace save flow. A successful HTTP status does not acknowledge a save unless the returned path, exact submitted content and actual UTF-8 SHA-256 match. Newline normalization must not conceal different bytes.

An invalid or lost receipt keeps the original save operation and current draft. Recovery uses the same request rather than creating another write. A later permission/hash refusal cannot prove that a previously uncertain save never committed. A valid receipt updates the file baseline but preserves edits made after the submitted snapshot. Existing conflict comparison and late-file-read protection remain in place.

This is save integrity within the current editing session and existing service recovery contract. It does not establish cross-restart offline synchronization, terminal PTY, debugger, realtime collaboration or a complete IDE. The Monaco embedded-menu runtime language boundary remains disclosed in the interface.

## Evidence

[Four focused recommendation tests](../../apps/web/src/TaskDispatchOptionsPanel.test.tsx), TypeScript checks and paired-catalog checks have passed. They cover malformed/cross-scope responses, read-only adoption, pagination, CNEN switching, access loss, late results, exact newer-profile preview and preservation of an uncertain original request.

Actual interface, real responses, narrow-screen/keyboard observations and execution evidence must be recorded by the main integration run. Protocol fixtures and local checks must not be reported as vendor, cross-machine or production acceptance.
