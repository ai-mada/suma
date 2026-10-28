# Read requirement acceptance traceability

Current product baseline: 1.0.0. [中文](../development/requirement-traceability.md) · [Contract — Chinese original](../../contracts/requirement-traceability.md) · [Scoped verification](../quality/requirement-traceability-verification.md).

Expand **Acceptance traceability** in a requirement's details. The panel reads the current formal requirement, published design, protected task adoption and exact linked execution. Opening it creates no tasks and starts no tools. Each criterion shows the number of mapped tasks and how many currently have formal acceptance. Expand it to inspect task binding, execution, test-node facts and human acceptance, then follow the actual task or run references.

A criterion is identified by its requirement ID, formal version and 1-based index. Its position has no identity across requirement versions; equal text can still represent two entries. A changed requirement never inherits mappings from the old design's positions. Publication, adoption, execution success, test success and human acceptance remain separate facts. Test-node counts are neither test-case counts nor a coverage percentage.

| Displayed situation | Next action |
| --- | --- |
| No publication or no mapped task | Complete the design draft and use the existing authorized publication flow; ordinary task associations cannot establish reviewed coverage |
| Proposal not adopted | Review the complete task-change preview and explicitly adopt it before using the existing dispatch flow |
| Publication, requirement or task basis changed | Review and adopt the current basis; the old run remains traceable but cannot accept the new basis |
| No linked run | This task is not currently dispatched; it does not mean the project has never executed |
| Successful test nodes without formal acceptance | Inspect the exact candidate and independent tests, then use the existing authorized human review action |
| Source unavailable, unknown run or read failure | Check current sources and access, then explicitly refresh; do not infer success or create replacement work |

Node and test drill-down lists each contain at most 20 entries, with full counts and an explicit completeness flag. Visible entries cannot stand for an incomplete list. Test summaries cover only `type:test` nodes in the exact current run. Full test-case plans, automatic criterion-level test coverage, application version sets and release traceability remain pending.

**Refresh facts** hides previous facts before reading again. Collapsing the panel or changing account, organization, project or requirement cancels the previous read and isolates late responses. An authorization failure hides protected references; explicitly retry after access is restored. If the requirement version has changed, explicitly choose **Refresh requirement** before using current traceability. Version mismatches and permission failures never trigger an automatic parent refresh loop.

This panel is a read. Design saves, publication, adoption, dispatch and human review retain their existing current authorization, expected versions and original-operation recovery. Recover an uncertain write using its original request and operation ID. User text and immutable evidence keep their original language.
