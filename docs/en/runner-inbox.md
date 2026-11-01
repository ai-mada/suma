# Pairing inbox and saved identity reuse

Status: 1.0.0; owners: control plane, Runner and Web; requirement: T-RUNNER-INBOX-185. [Contract](../../contracts/runner-inbox.md), [architecture](../architecture/runner-inbox.md), [verification](../quality/runner-inbox-verification.md).

Current project administrators can search and resume existing pairings in the original tool registration dialog. The collection provides literal search, logical-state filters, full filtered counts and bounded keyset pagination. Each page reauthorizes and uses current database time; it is a live view rather than a frozen snapshot. Selecting a row rereads its exact detail. Pending writes and scope changes remain protected.

Browser-only reason codes describe current observations; they confer no authority and do not appear in machine responses or original operation receipts. An operator URL change still blocks approval and machine operations. A current administrator may close an unexpired pending pairing through its original CAS reject command.

The public `--paired-runner ID` selector resolves an existing default identity. The loader checks the exact pairing ID before reading the system credential. Explicit control, project and worker assertions are retained. Custom identity paths remain a separate advanced option. No new credential, SDK or plaintext fallback is introduced.

Preparation, one-time inventory reporting and continuous Worker startup have separate copy actions. The browser does not execute them. Approved identity, fresh inventory, online execution, enabled profiles and business acceptance remain separate facts.

Focused source and representative UI verification are recorded separately from the previous Windows system-vault check. Physical distributed hosts, real vendor models, other database instances, cross-platform installations and the complete original31/P0–P7 platform remain delivery work.
