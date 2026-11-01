# Knowledge projection and currently authorized retrieval

Status: scoped implementation verified. Suma 1.0.0, T-KNOWLEDGE-PROJECTION-197. See the [requirements](../product/knowledge-projection.md), [architecture and AI SOP](../architecture/knowledge-projection.md), and [interaction design](../design/knowledge-projection.md).

Publish or retire reviewed knowledge with a fixed transactional outbox event, project it outside the business transaction and verify an exact acknowledgement. Source kind separates documents from project knowledge. A monotonic event sequence and the immutable publication revision/digest serve different purposes; draft edits preserve the earlier approved publication.

Native full-text and configured vector retrieval retain their actual modes and processing generation. Current tenant/project authority and source eligibility are checked again after external IO. Snippets are reconstructed from the authoritative text. Exact preview locations use Unicode codepoint half-open ranges bound to the same publication reference.

Deletion tombstones, durable input snapshots and bounded generation rebuilds prevent stale replay or configuration changes from masquerading as an empty healthy index. Delivery counts, a provider-local watermark, current coverage and actual task adoption remain separate facts. The user explicitly chooses retrieval mode and publication references.

External memory synchronization, commercial embedding verification, shared physical multi-host ownership and full production-scale acceptance remain independent platform deliveries.

[Scoped verification and remaining boundaries](../quality/knowledge-projection-verification.md).
