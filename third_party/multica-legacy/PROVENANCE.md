# Multica source provenance

This directory covers the narrowly adapted task grouping algorithm in `apps/web/src/multica-task-view.ts`, not the entire Suma platform.

- Upstream: https://github.com/multica-ai/multica
- Copyright: Copyright 2025 Multica
- Source commit: `e6f840ca115740a6fb80786537405bcf049d6497`
- Source path: `apps/web/features/issues/components/board-view.tsx`, `buildColumns`, original lines77–93.
- Source Git blob: `a33119a4d6bd1ff2a7e6adc9d509f023a7faa8ce`
- Source file SHA-256: `b63b667e53231494e858ee24dbc31a3c7bd9b7effa7c40ab8dd78967c97f703d`
- License Git blob: `23456c21785fc2a7bbb6687b95de17140abc86c7`
- License SHA-256: `062d056e48fee2e6cea78bc63f1a3d33464b4765bd9ab4719d29880860e6cd85`

The accompanying LICENSE is copied byte-for-byte from that historical commit. Its heading is Apache License, Version2.0. Its appendix also contains the upstream patent-counsel sentence and copyright text; it is preserved in full, rather than replaced by a standard template. The source tree at this commit contains no NOTICE or nested license file. Later repository license terms are not used as permission for this historical extraction, nor is this historical file used to relicense newer code.

Suma modifications: replace upstream Issue and grouping types with Suma task records; group by business status, owner resource ID or priority; retain backend page order and distinguish unknown/unassigned values; remove the React/DnD/client-store and upstream status-mutation dependencies. Group counts describe the loaded page window, while the authorized server owns the complete filtered total and pagination. No upstream branding, style assets or runtime permission bypass is copied.

The task query, local named-view validation/storage, authorization, pagination, UI and tests are Suma implementations. The current Multica checkout `139b13a462c9a9757e18a17578a867eecd99851e` is used for feature research only. Its combined Multica License adds hosted-service, commercial-embedding and branding conditions; current saved-view/provider code is not copied under the historical license.

The machine-readable manifest records the actual adapted destination and source hashes. Updating this extraction requires a fresh source and license review; do not replace it from the current upstream working tree.
