# Maintenance-aware tool diagnostics

Status: T-TOOL-DOCTOR-199 scoped implementation. Suma1.0.0. The original31-requirement/P0–P7 objective remains active. See the [contract](../../contracts/tool-doctor-maintenance.md), [requirements](../product/tool-doctor-maintenance.md), [architecture and AI SOP](../architecture/tool-doctor-maintenance.md), and [design](../design/tool-doctor-maintenance.md).

Tool Doctor keeps the original four dispatch prerequisites and adds a separate, short-lived maintenance recommendation. A pending or claimed inspection points to the original progress; an unknown outcome points to its original reconciliation. Stored proof does not mean the worker acknowledged closure. Only after closure can a user explicitly request a new inspection.

A worker may be occupied by another configuration’s inspection. The interface reads that original inspection and configuration before navigating; it never relabels it as the selected configuration. Existing unresolved local write intents retain their original body, key and receipt. Account, organization, project, caller or source changes invalidate old recommendations.

`tools doctor --format json|table` retains JSON by default and adds a bilingual table with precise target and operation IDs. It performs only a read. No suggestion grants installation, repair, task execution, approval or dispatch authority. Existing writes still reauthorize against their original version, reference, fence, owner and operation identity.

The stage reuses existing maintenance, inspection and recovery services. Formal configuration replacement, real vendor setup and models, physical multi-host operation and full production acceptance remain separate deliveries.

[限定验证与剩余范围 / Scoped verification](../quality/tool-doctor-maintenance-verification.md)。
