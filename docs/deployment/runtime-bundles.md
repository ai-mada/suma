# Go 运行时交付包

2026-10-29 清理说明：按用户要求，`dist/runtime` 各目标仅保留当时最新成功批次及其对应的 `builds` 收据；更早运行程序和批次目录已移除。本文件及其他历史报告中的旧路径保留为当时的记录，不保证仍有对应程序。历史批次的清单、源码清单和 build-info 已核对并集中保存在 `dist/delivery/runtime-history-receipts-20261029.zip`，移除清单及 SHA-256 见同目录 `runtime-history-cleanup-20261029.json`；归档不包含旧程序本体。

`scripts/build-runtime.ps1` 构建控制面 `suma-control`、Runner `suma-runner`、本机静态发布提供方 `suma-deployer` 和 CLI `suma`。脚本需要 PowerShell 7 及本机已安装、满足 `go.mod` 的 Go 工具链。使用 `CGO_ENABLED=0`，不调用目标平台二进制。

## 构建

在仓库根目录的 PowerShell 7 中执行：

```powershell
# 默认只构建当前主机的平台和架构
& ./scripts/build-runtime.ps1

# 全部六种组合
& ./scripts/build-runtime.ps1 -All

# 显式选择；也接受 linux/amd64 写法，重复目标只构建一次
& ./scripts/build-runtime.ps1 -Targets @('linux-amd64', 'darwin-arm64')
```

支持 `windows-amd64`、`windows-arm64`、`linux-amd64`、`linux-arm64`、`darwin-amd64`、`darwin-arm64`。`-All` 与 `-Targets` 不能同时使用。命令行通过 `pwsh -File` 调用时，可传 `-Targets 'linux-amd64,darwin-arm64'`。

构建依赖必须已在仓库的 `.cache/go-mod-runtime` 中。脚本关闭网络依赖获取及自动 Go 工具链下载。首次可显式从已有 Go 模块缓存复制缺失文件：

```powershell
& ./scripts/build-runtime.ps1 -All -ModuleCacheSeed 'C:/Users/Administrator/go/pkg/mod'
```

将示例路径替换为实际缓存目录；该目录需含 `cache/download`。复制仅补充缺失文件，不修改来源或覆盖已有文件；链接条目被拒绝。随后执行实际 `go mod verify`。缺失、损坏或版本不满足会失败并返回非零退出码，不能据此宣称已构建。脚本不修改 `go.mod`、`go.sum` 或系统环境。

## 文件与证据

每次生成独立 UTC 时间戳及随机后缀 `<stamp>`，保留历史文件：

```text
dist/runtime/
  windows-amd64/<stamp>/
    suma-control.exe
    suma-runner.exe
    suma-deployer.exe
    suma.exe
    *.buildinfo.txt
    manifest.json
  linux-arm64/<stamp>/...
  builds/<stamp>/
    manifest.json
    source-manifest.json
    linked-modules.json
```

每个平台先在 `.cache/runtime-build/<stamp>/targets/<target>` 编译全部四个程序。只有四个程序全部成功、目标架构检查通过且构建前后源码清单相同时，才将该次平台目录原子移动到交付位置；不复用旧二进制。某个平台失败时继续尝试其他平台，最终仍以非零退出码结束。全局清单的 `status` 为 `succeeded`、`partial_failure` 或 `failed`，逐平台记录结果和错误；失败暂存与日志保留在本次 `.cache` 目录。

逐文件清单包含 SHA-256、字节数、程序入口、Go 工具链版本和完整 `go version -m` 内容。每个文件另有可读的 `.buildinfo.txt`。宿主 Go 读取构建信息并校验 `GOOS`、`GOARCH`、`CGO_ENABLED=0`，不启动跨平台程序。

`linked_modules` 记录每个二进制实际链接的第三方模块路径、版本、校验和及替换信息；全局 `linked-modules.json` 去重汇总所有成功平台，并列出使用它的目标和二进制，可作为第三方许可盘点输入。它不包含 Go 标准库，也不代替许可审核。

源码清单覆盖 `go.mod`、`go.sum` 及四个程序入口目录、`internal` 下所有非测试 Go 源文件。程序入口目录直接由同一程序清单派生，当前包括 `cmd/suma-control`、`cmd/suma-runner`、`cmd/suma-deployer`、`cmd/suma`；全局收据的 `programs` 保存名称与包路径。逐文件保存哈希与大小，并以有序 JSON 的 UTF-8 字节计算 `source_sha256`。脚本、文档、测试文件的编辑不会改变这一源码哈希。此范围是保守运行时范围，可能包含特定二进制未使用的 Go 文件。当前无 `go:embed`；若加入嵌入资源，脚本会拒绝构建，须先扩展资源清单范围。模块缓存以 `go mod verify` 检验。

所有 Go 子进程独立设置仓库内的 `GOCACHE`、`GOMODCACHE`、`GOTMPDIR`、`TEMP`、`TMP`、`TMPDIR`，并隔离 Go 工作区、持久环境配置和继承的 Go flags。Go 1.24 的 `TEST_TELEMETRY_DIR` 指向本次仓库缓存目录，目录内模式设为 `off`，避免在用户目录记录 telemetry。`GOAMD64=v1`、`GOARM64=v8.0` 固定基础架构要求。父进程、用户及机器环境变量不被改写。参数使用进程参数列表传递，支持带空格的 Go 安装路径和仓库路径。脚本不清理缓存、旧交付包或用户目录。

## 验证边界

编译通过与构建信息检查不表示目标系统安装、启动、数据库连接、Runner 执行、终端交互、平台签名或部署已经验收。Windows 文件未签名；Linux/macOS 文件部署后需设置执行权限。桌面安装包另见 [desktop.md](desktop.md)。

## 历史三程序实际验证

以下批次于 P5 发布控制面与静态发布提供方加入前构建，保持原目录与收据不变。它不含 `suma-deployer`，也不能作为后来加入的制品、发布审批、真实部署与恢复代码已打包的证据。新四程序批次使用独立时间戳与源码清单，见 [P5 四程序构建记录](release-runtime-bundles.md)。

2026-10-24（Asia/Shanghai），Windows amd64 宿主、Go `go1.24.5`、PowerShell 7：实际 `-All` 成功，18 个二进制；38 次 Go 子进程均返回 0。收据为 `dist/runtime/builds/20261023T163759813Z-4b7523bf/manifest.json`，逐平台目录使用同一时间戳。构建脚本 SHA-256 为 `8816d8d1dc07d5a44461b57d251f07d5723b5a8baadf9f1c870cdaf802225caa`。

| 平台 | 控制面 / Runner / CLI | 三个文件合计字节 | 链接模块并集 |
|---|---|---:|---:|
| Windows amd64 | 3/3 编译与构建信息检查通过 | 43,762,688 | 35 |
| Windows arm64 | 3/3 编译与构建信息检查通过 | 41,188,352 | 35 |
| Linux amd64 | 3/3 编译与构建信息检查通过 | 42,716,828 | 33 |
| Linux arm64 | 3/3 编译与构建信息检查通过 | 40,730,529 | 33 |
| macOS amd64 | 3/3 编译与构建信息检查通过 | 42,764,240 | 34 |
| macOS arm64 | 3/3 编译与构建信息检查通过 | 41,008,710 | 34 |

源码清单前后相同：`bcbd66b6732f635d8850ad9b3aa80d6d1ea3df40ed4f9a617f0313c7062e70a8`。跨平台第三方模块并集为 36；相对 Windows 的 35 个模块，Linux/macOS 实际额外链接 `github.com/google/uuid v1.6.0`，应加入第三方许可盘点。精确的逐文件 SHA、模块版本/checksum、平台归属均在本次 JSON 收据中。

附加实际检查：

- 非法目标被拒绝并退出 1。
- 通过仓库内临时 junction 提供带空格的仓库路径，默认目标确为当前 Windows amd64；三个程序实际构建成功，源码哈希相同。Go 本身也安装在带空格的 `D:/Program Files/Go`。
- 本地独立失败探针临时将 Go 可执行文件替换为仅返回原生退出码 23 的工具夹具。脚本退出 1，收据记录该退出码，未发布任何目标目录；之前的 18 个二进制哈希保持相同。此夹具只检验失败传播，不作为实际编译证据。
- 检查子进程缓存与临时目录均在仓库 `.cache`，继承的错误 `GOFLAGS` 被隔离，父进程相关环境变量保持相同；本次 telemetry 目录仅有 `off` 模式文件。

探针原始收据保留在 `.cache/runtime-path-space-b55c3769/verification.json`。默认路径空格构建另有 `dist/runtime/builds/20261023T163937857Z-a1eb64aa/manifest.json`；失败探针为 `dist/runtime/builds/20261023T163936756Z-790199e9/manifest.json`。临时 junction 在完成清单和哈希记录后解除，缓存、探针文件与旧产物均保留。未运行重复 Go 全量测试；本轮验证只涵盖交付脚本和实际跨平台编译，未启动上述目标程序。
