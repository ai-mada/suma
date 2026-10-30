# Connect a local desktop Runner

T-MULTICA-DESKTOP-153, suma 1.0.0. The desktop app can retain a private local credential and start the current project's Runner after an explicit **Connect this computer** action. This continues [Quick tool connection](tool-quick-connect.md); the [contract](../../contracts/desktop-managed-runner.md) defines the exact boundary.

## Use the connection

1. In **Add tool**, choose Codex JSONL, CodeBuddy CLI or DeepSeek Harness multica. The app reads connection status without issuing a credential or starting a Worker.
2. If a project folder is required, choose and explicitly bind it through the existing native picker. It must already be a real Git root with a full HEAD commit and an ignored `.cache/runner/worktrees` directory. The app does not initialize, commit or change ignore rules for the repository.
3. Select **Connect this computer**. The app starts the fixed packaged Worker and waits for an actual matching report received after this launch and newer than the previous inventory watermark. Starting a process alone does not mean connected.
4. Once connected, Quick connection refreshes private and shared nodes and selects the exact local worker. **Register configuration** still creates a paused profile. **Enable tool** remains a separate confirmation.

Local management remains accessible when an old registered identity has not reported tools and after a profile is registered. It is collapsed into a secondary area when a fresh remote configuration already serves the main flow.

## Credential and recovery boundary

The main process checks the real current session, project, repository binding, full commit and service origin. It saves the original random credential and issuance operation in an OS-encrypted private journal before registering only the credential digest through a machine-HMAC-protected endpoint. The current endpoint requires loopback, SQLite, a real browser session and current project administration permission.

The fixed control executable receives a private nine-field stdin envelope. It reuses the existing Go Worker, fencing, durable journal, result outbox and central DAG. Credentials do not enter renderer results, arguments or inherited environment. Linux plaintext or unknown storage backends are rejected.

Use **Check the original connection** when a result is uncertain. Explicit recovery retains the original operation and credential; it does not silently issue another node. The original Grant lasts 24 hours. Historical receipts remain historical facts after expiry and cannot authorize a launch or extend access. Explicit renewal and unrecoverable-node resolution still need a complete user flow.

The main process rechecks current access, binding and node eligibility every 15 seconds. Access loss or binding changes take the stop path. **Stop local node** describes only the local node; task outcomes remain in their original runs. An unconfirmed stop must not be presented as a stopped node or cancelled task.

## Address and current limits

The local service defaults to `127.0.0.1:4319`. Its startup process may explicitly set `SUMA_DESKTOP_PORT` to 1024–65535. An occupied port fails instead of selecting a random address. An explicitly changed origin cannot reuse the old journal connection.

This is a three-adapter desktop flow with an embedded local control plane. It does not complete remote SaaS-to-desktop connectivity, arbitrary tools or multi-host management. Tool login, model access, budgets, dispatch, successful tasks, independent tests and human acceptance remain separate checks.

Seventeen actual checks passed in a Windows Electron 35.7.5 session with software offscreen rendering, including real DPAPI, production IPC, digest issuance, standard Go Worker lifecycle, fresh inventory, one paused registration and separate enable, stop, original-node recovery, a graceful whole-app restart and logout-driven stop. Chinese 1366 and English 390 images were visually inspected. There was one worker and one issuance operation; five registration transports retained the same original request, and all five formal Worker processes reached exit 0 and close. Tasks, runs and model calls remained zero. See the [verification record](../quality/desktop-managed-runner-verification.md).

The first GUI attempt failed because the page rejected fractional inventory timestamps. The strict nanosecond parser repair rebuilt only the Web candidate, then continued with the same private journal, worker and issuance operation. The original failed evidence and both source snapshots remain recorded.

The coding-tool fixture supports synthetic probes only. The directory dialog returns a fixed owned Git root; production signing and Go binding are real, while OS chooser UI acceptance is not claimed. Real supplier installation/authentication/coding, forced-crash recovery, macOS/Linux credential storage, installers, multiple hosts, scale and production acceptance remain unverified. Renewal after the 24-hour Grant still needs its complete explicit user flow. Multica informed the behavior and boundaries; no new Multica product source was copied.
