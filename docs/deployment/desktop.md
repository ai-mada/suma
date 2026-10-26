# 桌面打包与启动

`apps/desktop` 使用 Electron 主进程启动本机 Go 控制服务；渲染页与浏览器版共用 `apps/web`。2026-10-24 已在 Windows 构建 NSIS 安装器、portable 便携版和完整 unpacked 目录，均未签名；安装、升级、卸载及完整桌面 GUI 尚未实测。

## 构建

需要 Node.js 22+、pnpm、Go 1.26+（固定构建1.26.10）。在仓库根目录安装依赖后执行：

```powershell
pnpm build
pnpm --filter @suma/desktop package:dir
```

准备脚本将当前宿主平台的 `cmd/suma-control` 编译到仓库忽略目录 `.cache/desktop/control`；electron-builder 使用已安装的 Electron distribution，把控制服务和 `apps/web/dist` 复制进应用 resources。可分发构建执行 `pnpm desktop:package`，最终交付物位于 `dist/desktop`。开发启动 `pnpm desktop` 也需要先完成网页与控制服务准备；可先运行 `node apps/desktop/scripts/prepare.cjs`。

本轮使用冻结的网页构建，在独立 PowerShell 进程中执行以下命令。缓存与临时目录只影响该进程及其子进程，全部位于仓库 `.cache`，未改机器或用户环境变量：

```powershell
$packCacheRoot = Join-Path (Get-Location) '.cache/desktop/packaging-recovery'
$env:TEMP = Join-Path $packCacheRoot 'tmp'
$env:TMP = $env:TEMP
$env:GOTMPDIR = Join-Path $packCacheRoot 'go-tmp'
$env:GOCACHE = Join-Path (Get-Location) '.cache/go-build'
$env:ELECTRON_CACHE = Join-Path (Get-Location) '.cache/electron'
$env:ELECTRON_BUILDER_CACHE = Join-Path (Get-Location) '.cache/electron-builder'
$env:CSC_IDENTITY_AUTO_DISCOVERY = 'false'
New-Item -ItemType Directory -Force -Path $env:TEMP,$env:GOTMPDIR,$env:ELECTRON_CACHE,$env:ELECTRON_BUILDER_CACHE | Out-Null
pnpm --filter @suma/desktop package --win --config.win.signAndEditExecutable=false
pnpm --filter @suma/desktop package:dir --win --config.win.signAndEditExecutable=false
```

`signAndEditExecutable=false` 同时跳过应用可执行文件的资源编辑，当前使用默认 Electron 图标。正式发布仍需独立配置图标、版本资源及代码签名。

本轮产物：

| 路径 | 构建状态 | 尚未验证 |
| --- | --- | --- |
| `dist/desktop/Suma Setup 0.1.0.exe` | NSIS 构建成功，Authenticode `NotSigned` | 安装、升级、卸载及其系统副作用 |
| `dist/desktop/Suma 0.1.0.exe` | portable 构建成功，Authenticode `NotSigned` | 便携版 GUI 启动、退出与数据行为 |
| `dist/desktop/win-unpacked` | 独立目录打包成功，内置控制服务和网页齐全 | 完整 Electron 窗口与桌面交互 |

`dist/desktop/SHA256SUMS.txt` 保存主要产物摘要；`packaging-verification.json` 保存目录核验结果、未签名检查及成功/失败打包日志的 SHA-256。原始日志保留在 `.cache/desktop/packaging-recovery`，未将本机日志或依赖缓存纳入源码。

### 本轮环境故障与恢复

依赖收集首次调用真实 `pnpm list --prod --json --depth Infinity` 时出现 `ERR_SQLITE_ERROR: unable to open database file`。检查本机 pnpm 11.25.0 源码和错误栈，确认 `list` 从 `node_modules/.modules.yaml` 读取 `storeDir` 并打开对应 `index.db`；此路径来自安装元数据，单改通用 `cacheDir` 不会更正它。现场元数据仍指向 `D:\.pnpm-store\v11`，而仓库已配置 `.cache/pnpm-store`。

恢复时先备份并记录旧元数据摘要，确认仓库内现存 `.cache/pnpm-store/v11/index.db` 通过 SQLite `quick_check`，再只将本机生成元数据的 `storeDir` 更正为该现存仓库。随后根目录和桌面目录的真实依赖列表均成功。这是本地安装元数据修复，没有重装依赖、改依赖版本、替换包管理器或绕过依赖校验。源码配置仅在 `pnpm-workspace.yaml` 增加 `cacheDir: .cache/pnpm-cache`，用于将通用 pnpm 缓存留在仓库内。

修复后，完整构建在沙箱中进入 NSIS 阶段，但下载官方 GitHub 工具时被网络权限阻止（`connect EACCES 20.205.243.166:443`）。一次经过自动审查的网络权限重试成功，electron-builder 从官方 `electron-userland/electron-builder-binaries` 下载 NSIS、7zip 和 NSIS resources，使用工具内置校验摘要并缓存到仓库 `.cache`；NSIS 与 portable 均完成。随后再次独立执行 `package:dir` 成功，恢复了首次失败时被部分覆盖的 `win-unpacked`。

| 平台 | 已配置目标 | 发布前仍需完成 |
| --- | --- | --- |
| Windows | NSIS、portable | Windows 真机安装/升级/卸载验证与代码签名 |
| macOS | DMG、ZIP、hardened runtime | macOS 宿主构建、开发者签名、公证、系统权限验证 |
| Linux | AppImage、deb | Linux 宿主构建、发行版与 sandbox 验证 |

准备脚本按宿主系统编译服务；请在对应操作系统构建。不能把 Windows 的 Go 二进制打进 macOS/Linux 包并声称跨平台可运行。仓库不包含签名密钥、开发者证书或自动公证凭据。

## 服务与数据

每次启动选择一个 loopback 空闲端口，执行打包的 `suma-control --addr 127.0.0.1:PORT`。传入 `SUMA_DATA_DIR=<Electron userData>/data` 与 `SUMA_WEB_DIR=<resources>/web`，等待 `/healthz` 就绪后才打开窗口。数据位于操作系统的应用用户目录；未显式配置数据库时使用该目录下的 SQLite 数据库。没有找到服务或网页文件、服务提前退出、健康检查超时都会显示启动错误。

主进程持有服务句柄，关闭应用时发送终止信号并等待退出，超过宽限后强制终止。单实例锁避免重复启动桌面数据实例。异常结束后数据库通过自身事务/WAL 恢复；Runner 的 inbox/outbox 使用独立目录，桌面当前不会自动启动 Runner。

桌面绑定本机 loopback；服务器、多节点和远程 Runner 的部署使用独立控制服务。桌面本机启动不等同于公网服务。`SUMA_DB_DSN` 等显式环境配置由控制服务解释，若把多个桌面实例指向同一远程数据库，需按服务部署要求管理租户与连接。

## 渲染器边界

`contextIsolation:true`、`sandbox:true`、`nodeIntegration:false`；启用 webSecurity，禁止混合内容，所有新窗口被拒绝，页面导航/重定向限制在所启动服务的 origin。摄像头、麦克风等 Electron 权限默认拒绝。

preload 只暴露不可变的平台和版本信息，没有文件、进程执行、通用 IPC 或密钥读取入口。登录使用控制服务的同源 HttpOnly 会话；模型凭据不通过 preload 交给渲染页。

## 验证状态

已在 Windows 执行主进程/preload 语法检查与生命周期测试：健康就绪、启动进程提前退出、实际子进程退出确认，以及 origin 导航限制。使用 electron-builder 26.15.3 / Electron 35.7.5 完成上述 NSIS、portable 与目录打包。目录核验确认内置网页逐文件与冻结的 `apps/web/dist` 一致，控制服务与准备产物一致，ASAR 中主进程、生命周期和 preload 源码一致，内置控制服务 `--help` 可执行。安装器、便携版、目录应用和内置控制服务的 Authenticode 状态均为 `NotSigned`。

本轮未执行安装器、未启动桌面 GUI、未进行安装/升级/卸载测试；内置服务的 `--help` 核验不代表完整桌面启动通过。Go Runner 的 Windows Job Object 超时测试独立通过。正式代码签名、公证以及 macOS/Linux 实机验证仍需单独记录。
