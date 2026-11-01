# Published knowledge discovery

Status: scoped implementation and verification complete. Suma 1.0.0, T-KNOWLEDGE-DISCOVERY-196. See the [contract](../../contracts/knowledge-discovery.md), [product requirements](../product/knowledge-discovery.md), [architecture and AI SOP](../architecture/knowledge-discovery.md), and [interaction design](../design/knowledge-discovery.md).

Discover currently eligible published memory, SPEC and skills, preview their exact approved revision and provenance, then explicitly select the existing id/revision/sha256 reference for a task. Management drafts, historical readability, current adoption eligibility and actual model use are separate facts.

The current-project keyword reader matches parsed publication title, summary and content. A bounded current-read keyset scans up to 200 candidates, returns at most 20 hits and keeps encoded responses within 48KiB. An empty page with a continuation means more candidates remain; it is not a project-wide empty result. Refresh begins a new traversal, while authorization, publication and source eligibility are checked again on every read.

Web, CLI and the native read-only agent tool share this discovery contract. The original task preview/start transaction resolves selected publications again. Selection survives search and pagination without silently upgrading versions. Unauthorized, retired or changed sources cannot become fresh task inputs. Source text remains unchanged; Chinese and English interface states share the same gates.

Semantic projection, external memory synchronization, executable plugins, physical multi-machine verification and measured model use remain separate deliveries within the complete platform objective.
