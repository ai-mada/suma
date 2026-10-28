# Desktop installation and packaging

Status: **1.0.0 / T-DESKTOP-108, in progress**, updated 2026-10-28. [中文](../deployment/desktop-installation.md) · [Startup and directory authorization](../deployment/desktop.md) · [Delivery status](../delivery/status.md).

This guide covers x64 and arm64 distribution targets for Windows, macOS and Linux, with one packaging command. Target configuration, successful packaging, actual installation, signing/notarization and production acceptance are separate facts. **Actual Electron startup remains blocked by GPU child exit `0xC0000135`; the missing dependency is unresolved. This guide does not claim successful installation or execution on all six targets, or an available public download.** The full P0–P7 scope and requirements 1–25 remain active, including the complete local development workbench.

## Choose a system, architecture and format

Check the operating system and CPU first. x64 means 64-bit Intel/AMD; arm64 means 64-bit ARM. On Windows, check System type under Settings → System → About. On macOS, About This Mac identifies an Intel processor or Apple chip. On Linux, run `uname -m`: choose x64 for `x86_64`, arm64 for `aarch64`. Prefer the native architecture; emulation is not acceptance on that target.

| System / architecture | `--platform` / `--arch` | `installer` | `portable` |
| --- | --- | --- | --- |
| Windows x64 | `win32 x64` | NSIS `.exe` | Portable `.exe` |
| Windows ARM64 | `win32 arm64` | NSIS `.exe` | Portable `.exe` |
| macOS Intel | `darwin x64` | `.dmg` | `.zip` containing the app |
| macOS Apple chip | `darwin arm64` | `.dmg` | `.zip` containing the app |
| Linux x64 | `linux x64` | `.AppImage` and `.deb` | `.tar.gz` |
| Linux ARM64 | `linux arm64` | `.AppImage` and `.deb` | `.tar.gz` |

`--format all` builds the system's installer and portable targets. `dir` produces an unpacked application directory for inspection or controlled distribution. Linux users choose one of AppImage, a Debian package or the archive. Keep the complete archive/directory contents; copying only the launcher is insufficient. Portable formats skip installation but **still store data under the current user's application data directory**. They do not move the database or directory authorization with a USB drive.

## Obtain, verify and install

Obtain the matching package, `manifest.json` and `SHA256SUMS` from a trusted maintainer or enterprise distributor. Check version, system, architecture and actual filenames first. This stage has no automatic publishing or update service. A SHA256 match proves correspondence to a trusted checksum list; it does not establish publisher identity or replace an operating system signature.

On Windows, open the download directory, then compare the result with the same filename in the trusted `SHA256SUMS`:

```powershell
$artifact = '.\Suma-1.0.0-win32-x64-installer.exe'
Get-FileHash -Algorithm SHA256 -LiteralPath $artifact
Get-AuthenticodeSignature -LiteralPath $artifact | Format-List Status,StatusMessage,SignerCertificate
```

On macOS or Linux, hash the individual file from its directory and compare it with the same entry in the trusted list:

```sh
# macOS
shasum -a 256 ./Suma-1.0.0-darwin-arm64-installer.dmg
# Linux
sha256sum ./Suma-1.0.0-linux-x64-installer.AppImage
```

Use macOS `shasum -a 256 -c SHA256SUMS` or Linux `sha256sum -c SHA256SUMS` only when the complete listed file set is present. When distributing one installer, missing other entries do not establish that the selected file passed verification.

| Format | Install and start | Remove the application |
| --- | --- | --- |
| Windows NSIS | Open the installer as your regular user, choose a directory in the wizard, then launch Suma from Start | Use Suma in the system's Installed apps settings |
| Windows portable EXE | Put the verified file in a normal directory writable by the current user and open it | Quit Suma, then remove the portable executable |
| macOS DMG | Open the DMG, copy the complete `Suma.app` to `~/Applications` or an approved application directory, eject the image, then open it in Finder | Quit, then move the installed `Suma.app` to Trash |
| macOS ZIP | Extract the complete `Suma.app`, move it to the chosen application directory and open it in Finder | Same as DMG |
| Linux AppImage | Make the verified file executable and run it as a regular user; see the [official AppImage instructions](https://docs.appimage.org/user-guide/run-appimages.html) | Quit, then remove the AppImage file |
| Linux deb | Install on Debian/Ubuntu using the package manager and trusted dependency sources; launch from the application menu | Remove the Suma package using the package manager |
| Linux tar.gz / unpacked directory | Extract into a user application directory, retain all resources and permissions, then run the `suma` launcher inside | Quit, then remove that application directory |

Windows NSIS is configured to default to the current user, allow directory selection, avoid requesting elevation, and avoid automatically launching after installation. It offers a Chinese/English language selector. Choosing all-user installation requires previously approved rights; data still belongs to each running account. These are configuration facts; actual bilingual installation and removal await acceptance. See the [official NSIS options](https://www.electron.build/v26/docs/api/app-builder-lib.interface.nsisoptions/) for their meanings.

Linux examples use filenames from the actual manifest. Installing a deb needs administrator approval; run Suma as a regular user:

```sh
chmod u+x ./Suma-1.0.0-linux-x64-installer.AppImage
./Suma-1.0.0-linux-x64-installer.AppImage
# Debian/Ubuntu: select the deb matching your system first
sudo apt install ./Suma-1.0.0-linux-x64-installer.deb
```

If SmartScreen, Gatekeeper or enterprise policy blocks the app, stop and verify its source, signing, notarization and organizational approval. Do not disable system protection, remove quarantine attributes, or pass `--no-sandbox` / `--disable-web-security`. Signing, notarization, distribution dependencies and minimum operating system versions still require target-machine acceptance. Generating a package does not remove these requirements. The [electron-builder signing guide](https://www.electron.build/v26/docs/features/code-signing/) distinguishes Windows signing from macOS signing and notarization.

## First launch and use

Suma starts its bundled Go control service on a dynamically allocated `127.0.0.1` port and opens the shared workbench after a successful health check. There is no fixed desktop port to find manually. Follow the interface to create an account, organization and project on first use, or sign in to existing data. Quitting Suma stops its child service; reopening uses the same user data. It currently does not start a Runner, install external tools or automatically restart a stopped control service.

In the current project's development workspace, choose a project folder, review the project and path, then explicitly bind it. Choosing alone does not bind; cancelling leaves the project unchanged. File, directory, Git and save operations still check server permissions and the project root proof. Resolve drafts or uncertain saves before changing directories. The complete native GUI journey awaits actual acceptance; see the [desktop guide](../deployment/desktop.md) for the protocol and evidence.

The workbench supports Chinese and English preferences. Startup diagnostics use this process's `SUMA_LANGUAGE=zh-CN` or `en-US`, then the system language, falling back to Chinese. This changes display language, not business content or execution logs. The native directory dialog uses the workbench's current language. System text, unknown errors, third-party controls and the complete installer language experience require separate acceptance.

## Data, backup and removal

Packaged apps use Electron's `userData` directory. With the current product name Suma, the usual locations are below; respect actual system redirection. [Electron's path documentation](https://www.electronjs.org/docs/latest/api/app#appgetpathname) defines these platform defaults.

| System | Current user's application data |
| --- | --- |
| Windows | `%APPDATA%\Suma` |
| macOS | `~/Library/Application Support/Suma` |
| Linux | `$XDG_CONFIG_HOME/Suma`, or `~/.config/Suma` when unset |

- `data/suma.db` and accompanying business files hold local state. In automatic key mode, `data/automation-ingress.key` protects saved ingress secrets.
- `desktop-workspace.key` is the 32-byte local directory authorization key. Database root proofs also identify the directory's operating system identity.
- Project source files remain in their selected directories. Protect model credentials, external CLI configuration and project repositories according to their own storage rules.

Quit Suma normally and confirm its child service has stopped before securely backing up the complete `userData` directory and project originals. Do not copy a live SQLite file alone. Keep keys and database together with restricted access; do not put them in Git, tickets, logs or public manifests. After restoration, check accounts, projects, permissions, originals and hashes. A different machine or replaced directory may have a different operating system identity and require explicit selection and binding again; matching path text does not restore authorization. There is currently no general data import, database upgrade or key rotation migration wizard.

Removing the application and erasing business data are separate operations. NSIS is configured with `deleteAppDataOnUninstall:false`; removing portable/archive/directory files does not erase `userData`. Do not delete the database or authorization key as a repair step. For intentional data erasure, verify the backup and separately confirm the exact data directory first; retain bound project originals. Data preservation during target-system uninstallation still requires installation acceptance.

## Build from source with one command

Work from the repository root with Node.js 22+, the repository-pinned pnpm and Go toolchain; `go.mod` currently declares Go 1.26.0 / toolchain 1.26.10. Prepare dependencies from the lockfile. The Electron SDK must match the target system and architecture. End users installing a complete package do not need these build tools.

Inspect a plan before building. Plans do not compile or package; all six can be inspected on one host:

```sh
node apps/desktop/scripts/package.cjs --platform win32 --arch x64 --format all --plan
node apps/desktop/scripts/package.cjs --platform win32 --arch arm64 --format all --plan
node apps/desktop/scripts/package.cjs --platform darwin --arch x64 --format all --plan
node apps/desktop/scripts/package.cjs --platform darwin --arch arm64 --format all --plan
node apps/desktop/scripts/package.cjs --platform linux --arch x64 --format all --plan
node apps/desktop/scripts/package.cjs --platform linux --arch arm64 --format all --plan
```

After the plan permits the build, run the matching command on the target system's host. For example, Windows x64:

```sh
node apps/desktop/scripts/package.cjs --platform win32 --arch x64 --format all --signing off
```

Omitted platform/architecture use the host values. The default format is `installer`; this stage supports only the default signing policy, `off`. The script explicitly sets the Go target system and architecture and builds the control service and web assets in separate staging directories, preserving an active preview's `apps/web/dist`. Output goes to `dist/desktop/<platform>-<arch>`. Working files, caches and temporary files stay in `.cache/desktop/<platform>-<arch>`; environment overrides affect only the build process.

| Optional argument | Purpose and limits |
| --- | --- |
| `--electron-dist ABS` | Use a prepared target Electron distribution directory with matching version, executable format and architecture; the default retrieves the correct target SDK rather than reusing another host architecture |
| `--out-dir ABS` | Use a fresh absolute output child of this repository's `dist/desktop` or `.cache`, without overlapping this target's staging directory |
| `--language zh-CN` / `en-US` | Select packaging diagnostic language; this does not establish installer language acceptance |
| `--maintainer "Name <email>"` | Set the public Debian package maintainer, overriding `SUMA_DESKTOP_MAINTAINER` and the repository default |

Builds reject a nonempty output directory. For another verification build, select a new output child and preserve previous artifacts/evidence rather than deleting user files.

Linux `installer` / `all` includes deb and needs a valid maintainer. The root `package.json` now sets `config.desktopMaintainer` to the approved public contact `zhaoyingchao <zyc2012518@126.com>`. Precedence is CLI `--maintainer`, this process's `SUMA_DESKTOP_MAINTAINER`, then the root configuration default. The current default needs no additional environment variable. Enterprise redistributors can explicitly supply their actual maintainer, for example:

```sh
node apps/desktop/scripts/package.cjs --platform linux --arch x64 --format installer --maintainer "zhaoyingchao <zyc2012518@126.com>" --plan
```

Linux portable / dir needs no maintainer. Missing deb maintainer information disallows the build plan; invalid formatting rejects the argument. Maintainer details are public package metadata: do not enter secrets, invent a contact or automatically take a private Git identity.

Cross-system installer/portable execution is rejected. macOS `dir` also requires a macOS host; other cross-system `dir` builds still need a matching Electron SDK and validated inputs. A plan's `build_allowed` means the system/policy permits proceeding; it proves neither resource validation, packaging nor installation. Prefer native builds and actual acceptance for each target system/architecture; macOS signing requires macOS. See [electron-builder's multi-platform restrictions](https://www.electron.build/v26/docs/features/multi-platform-build/).

Current Windows, macOS and Linux delivery supports only `--signing off`. `auto` and `required` are explicitly rejected before preparing resources, calling the SDK or using certificates; they are not functional signing paths. Production code signing, signed-artifact verification and macOS notarization remain pending implementation and acceptance. The current manifest records unsigned status, with installation, GUI, signature and notarization verification all unproven. Do not disable system protection to substitute for formally signed distribution.

On success, `manifest.json` and `SHA256SUMS` describe actual outputs. Names are `Suma-1.0.0-<platform>-<arch>-installer.<extension>` or `-portable.<extension>`, and `Suma-1.0.0-<platform>-<arch>-dir/` for an unpacked directory. The manifest records delivered files; checksums cover regular files and `manifest.json`, while symbolic links have separate manifest entries. Preserve the complete directory structure and permissions during distribution. Distribute the corresponding current outputs; a plan or previous file is not a newly built artifact. Before distribution, test installation, first launch, native selection/cancellation/binding, Chinese/English, restart, exit and data-preserving removal on the target machine.

## Offline and enterprise use

A complete desktop bundle includes Electron, the Go control service and web assets. The local workbench does not require Node.js/Go or a persistent download connection after installation. Configured external models, tools, knowledge services and remote collaboration need their own network access and authorization. Single-machine desktop operation uses the current operating system account, SQLite and local directory authorization. It does not automatically create an enterprise shared service, cross-device synchronization or SaaS tenant operations. See [server deployment](../deployment/server.md) and [container deployment](../deployment/containers.md) for shared services; explicit remote connection management remains pending.

An enterprise can build and verify packages on a controlled machine, then transfer packages and manifests into an isolated network. To reuse prebuilt inputs, pass absolute paths; replace these examples with actual controlled directories:

```sh
node apps/desktop/scripts/package.cjs --platform linux --arch arm64 --format portable --web-dir /approved/suma/web --control-binary /approved/suma/suma-control --control-manifest /approved/suma/suma-control.manifest.json --plan
```

Remove `--plan` to execute. The web input passes file-tree checks and is copied into staging. A prebuilt control binary requires its default `<binary>.manifest.json` sidecar or an explicit `--control-manifest`. It must match current source hash, version 1.0.0, target and binary hash, and pass actual executable-header architecture checks. Normal source preparation generates the sidecar and `prepared.json`, for example `node apps/desktop/scripts/prepare.cjs --platform win32 --arch x64`, under that target's `.cache/desktop` staging directory. To reuse them, put complete inputs and the sidecar in a controlled location outside the target staging directory. There is no command to fabricate provenance for an unknown binary; do not hand-edit a manifest to bypass mismatches.

Prebuilt inputs avoid recompiling the web and control service, but do not guarantee network-free packaging. There is no `--offline` switch. Prepare locked dependencies, the target Electron SDK and the selected format's tool caches, and verify offline building separately.

## Troubleshooting

| Symptom | Action |
| --- | --- |
| Plan disallows build / architecture mismatch | Check system, CPU, Electron SDK and input manifests; use a suitable native host rather than altering hashes or forcing another system's package |
| Missing control service/web assets, or service not ready | Preserve the startup diagnostic and verify the complete package and hashes; development validation uses isolated `.cache` resources described in the [desktop guide](../deployment/desktop.md) |
| Key or data directory unavailable | Check permissions, ordinary file types and real paths; preserve the original key and data rather than resetting them |
| Directory authorization invalid / binding conflict | Explicitly refresh the current binding, then select again; reconcile an uncertain original binding before creating a new operation |
| AppImage reports missing FUSE / system libraries | Follow distribution and AppImage requirements, using approved dependencies or matching deb/tar.gz; do not disable the Electron sandbox to replace a dependency fix |
| GPU child exit `0xC0000135` | Known actual startup blocker; retain the original error/environment, resolve the dependency and repeat the native journey. Passing Node/web checks does not prove a fix |

When reporting an issue, include version, package filename, hash, system/architecture, format and a redacted error. Do not upload databases, workspace keys, tokens, complete private paths or project source. [Delivery status](../delivery/status.md) tracks remaining installation, signing and full-platform work.
