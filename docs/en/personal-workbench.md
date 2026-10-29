# Personal work and context navigation

Status: T-MULTICA-136, initial release1.0.0. See the [contract](../../contracts/my-work.md), [interaction design](../design/multica-collaboration.md), [reference adoption](../integrations/multica-adoption.md) and [scoped evidence](../quality/multica-collaboration-verification.md).

Development opens Assigned to me. The server resolves your current canonical human resource and queries its complete task scope; account IDs are not task owner IDs. Search is literal title/description text, with explicit status counts and bounded live pagination. Drafts and other current business states remain visible. An unbound or inactive resource is explained separately. This view does not assign reviewers, receive handoffs or approve results.

Open a task to use its existing discussion, model/budget plan, dispatch and delivery controls, or open its workspace directly. All navigation retains the existing unsaved-operation guard and current server permissions. Reload after assignments or task states change; combined pages are not a historical snapshot.

Ctrl/Cmd+K opens page navigation and current-project object search. Use arrows, Home/End and Enter; Escape closes the overlay without changing the underlying task. Old-query and old-identity responses cannot become actionable results. Errors expose explicit Reload. Recently visited entries contain only in-memory static page IDs, scoped to your account, organization and project.

Chinese and English labels and search aliases use paired catalogs. Names, task text, evidence and custom business states remain unchanged. Source-aware routing never executes arbitrary result URLs.

Multica reference commit139b13a was verified against the local clean checkout and upstream main. Its current license includes additional hosted/commercial and branding terms. This stage independently implements the behavior with Suma's existing React/Radix/i18next components, without copying Multica code or adding dependencies. Full notification inboxes, server-paginated project boards, real provider/multi-host acceptance and browser visual checks remain separate delivery work.
