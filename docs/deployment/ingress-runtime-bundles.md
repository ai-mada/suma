# T-P4-003 签名事件入口四程序运行包

日期：2026-10-24（Asia/Shanghai）。交付批次：`20261024T033010763Z-94b39f3b`。六目标各四程序已实际交叉编译，24 个文件的摘要、目标格式和宿主 Go 构建信息均已复核。目标系统运行、安装、签名和生产接入各自验收。

## 冻结源码与功能范围

本批次基于 `9e061ab1930362646bf7ddbb0a1bd86d9536cd5b` 加本阶段工作区，精确绑定 101 个运行源码文件，`source_sha256` 为 `3c971b45a835621efb1cc8e9f7d16c638df02e6d6cd3d67b2d81bf6198178727`。构建前、构建后及独立复核时的逐项清单一致，`source_unchanged=true`。该摘要来自有序紧凑逐文件 JSON，和带排版的 `source-manifest.json` 文件摘要不同。

源码范围是 `go.mod`、`go.sum`，四个入口 `cmd/suma-control`、`cmd/suma-runner`、`cmd/suma-deployer`、`cmd/suma`，以及 `internal` 中的非测试 Go 文件；没有 `go:embed` 资源。浏览器资源、Electron、Node 知识服务、合同生成器、测试文件及外部工具安装均不在本运行包内。

签名入口合同见 [事件入口合同](../../contracts/automation-ingress.md)，部署密钥与协议使用见 [接入说明](../integrations/automation-ingress.md)。本批次包含 GitHub 原生签名、Suma 通用 Git/CI/监控签名输入、持久防重放、规则修订与当前权限门禁、AES-GCM 来源密钥存储，以及 CLI 发送协议；源码存在及编译完成不代替业务验收或真实厂商账户接入。

相对上一自动化批次 `20261024T023250966Z-97012787`，新增 8 个运行 Go 文件、修改 5 个、删除 0 个；逐项前后摘要见 [源码差异](evidence/runtime-ingress-20261024T033010763Z-94b39f3b/cut-difference.json)。新增文件分布于 `internal/platform/automation_ingress*.go` 和 `internal/cli/webhook.go`；修改控制面入口、CLI 路由、自动化派发、数据库和 HTTP 接线。上一批次、P5 历史包及 Windows 桌面历史安装包均保留原件；历史桌面包不能视为本次入口功能交付。

## 实际构建与独立复核

Windows amd64 宿主、PowerShell 7.6.5、安装的 Go `go1.24.5`；实际构建时间为 2026-10-24 11:30:10 至 11:30:50（UTC 03:30:10 至 03:30:50），退出 0：

```powershell
# GOMAXPROCS=2 仅传给本次构建进程。
& ./scripts/build-runtime.ps1 -All
```

原始 50 次 Go 调用为：宿主工具链检查 1 次、离线 `go mod verify` 1 次、构建 24 次、读取构建信息 24 次；全部退出 0，stderr 为空。随后独立调用宿主 `go version -m` 24 次，重新读取交付文件并与构建原件逐项比较，全部退出 0。没有执行这些目标程序。

独立检查 24 个程序文件的实际大小、SHA-256、二进制头架构及 `GOOS`、`GOARCH`、`CGO_ENABLED=0`、`-trimpath=true`；目标清单与构建回执一致。`-buildvcs=false` 和只读模块模式由原始构建调用证明。详细结果见 [逐项验证](evidence/runtime-ingress-20261024T033010763Z-94b39f3b/verification.json)。

| 目标 | 实际文件类型 | 程序数 | 四程序总字节 | 第三方链接模块并集 |
|---|---|---:|---:|---:|
| Windows amd64 | PE32+，machine 0x8664 | 4 | 61,609,984 | 35 |
| Windows arm64 | PE32+，machine 0xaa64 | 4 | 58,003,456 | 35 |
| Linux amd64 | ELF64，machine 62 | 4 | 60,049,305 | 33 |
| Linux arm64 | ELF64，machine 183 | 4 | 57,336,764 | 33 |
| macOS amd64 | Mach-O64，CPU 0x01000007 | 4 | 60,099,408 | 34 |
| macOS arm64 | Mach-O64，CPU 0x0100000c | 4 | 57,681,880 | 34 |

跨目标第三方模块并集为 36，路径、版本、校验和、替换项及完整 `linked-modules.json` 字节均与上一自动化批次相同，见 [依赖比较](evidence/runtime-ingress-20261024T033010763Z-94b39f3b/linked-module-comparison.json)。该清单覆盖实际链接的第三方 Go 模块，不代替标准库、Node、Electron、外部工具或许可审查。

依赖网络、工具链自动下载、VCS 获取和 telemetry 均关闭。GOMAXPROCS、Go 缓存、模块缓存、临时目录及 telemetry 设置仅对本次子进程生效，全部缓存与临时目录位于仓库 `.cache`；用户级、机器级环境摘要前后一致，见 [环境复核](evidence/runtime-ingress-20261024T033010763Z-94b39f3b/environment-verification.json)。原构建脚本 SHA-256 为 `66256a8d1454045f6db0e68641e1f6a7b1d1ad23341d9485fe45e5e8221859d2`。

## 实际文件位置与 Windows 摘要

交付根目录为 `D:\codes\ai-dev-os\suma\dist\runtime`，各目标独立目录：

```text
dist/runtime/<target>/20261024T033010763Z-94b39f3b/
  suma-control[.exe]
  suma-runner[.exe]
  suma-deployer[.exe]
  suma[.exe]
  *.buildinfo.txt
  manifest.json
dist/runtime/builds/20261024T033010763Z-94b39f3b/
  manifest.json
  source-manifest.json
  linked-modules.json
```

Windows amd64 实际目录为 `D:\codes\ai-dev-os\suma\dist\runtime\windows-amd64\20261024T033010763Z-94b39f3b`；Windows arm64 对应将 `windows-amd64` 换为 `windows-arm64`。表中程序名直接追加到该绝对目录即为实际文件路径。24 个全目标文件的绝对路径、大小及 SHA-256 均在 [逐项验证](evidence/runtime-ingress-20261024T033010763Z-94b39f3b/verification.json) 的 `binaries` 中。

| Windows 目标 | 实际程序名 | 字节 | SHA-256 |
|---|---|---:|---|
| amd64 | suma-control.exe | 25,292,800 | `bfdf0eacb72baf1d7b4c0633ac6e7f066b222f8f7ad5e171291c95c4a70f95c6` |
| amd64 | suma-runner.exe | 9,798,144 | `9cfd758211fee9d03514d63044ee4d9e9faaf70758d76b1586a984425110b288` |
| amd64 | suma-deployer.exe | 16,125,952 | `f8f6a32145d7679bbd6d04f7496c806e7e5c4f3afc8d84ee9ce8ce5098e8a433` |
| amd64 | suma.exe | 10,393,088 | `3d989122ded5345c071e5ef57c1d3a89eda8d582f16e9f6b819fb13a164d82ca` |
| arm64 | suma-control.exe | 23,918,080 | `5041c274ae1b9c7232d7857243dd152cbb2e802892a564a050a21b230e729034` |
| arm64 | suma-runner.exe | 9,108,480 | `c15a0aa255258b2d141507f79ea6bc8025883137f421ba2966f201863e2cbb0b` |
| arm64 | suma-deployer.exe | 15,244,800 | `bf9ca2838b1138359dafe408e6a7999bca70542f4b44dc920bb60a2978827b1e` |
| arm64 | suma.exe | 9,732,096 | `d4f3f1ca57181d17a344ce8f33cd4687a7d2c9de67c9ce1d11b1df1d38ea1c36` |

控制面实际切换验收应绑定 `D:\codes\ai-dev-os\suma\dist\runtime\windows-amd64\20261024T033010763Z-94b39f3b\suma-control.exe` 及上表摘要，持久主密钥、数据库和事件防重放的运行证据单独记录。本页的构建/归档步骤没有启动或停止服务。

## 可追溯证据与历史保留

证据目录：[runtime-ingress-20261024T033010763Z-94b39f3b](evidence/runtime-ingress-20261024T033010763Z-94b39f3b/verification.json)。构建清单、源码清单和模块清单均从 `dist/runtime/builds` 原件逐字节复制，二进制只保留在忽略的 `dist` 下，没有复制到 Git 文档目录。

| 归档文件 | 文件 SHA-256 |
|---|---|
| [原始构建清单](evidence/runtime-ingress-20261024T033010763Z-94b39f3b/manifest.json) | `324eb94c7947e0128d3290bbac7debc0376477c25384d571550784474e42c172` |
| [源码清单](evidence/runtime-ingress-20261024T033010763Z-94b39f3b/source-manifest.json) | `f3c6e4bf8cd3c3725f4535a90634fee9801fc4f18c131b2440334e8dcc3b9f4d` |
| [链接模块清单](evidence/runtime-ingress-20261024T033010763Z-94b39f3b/linked-modules.json) | `fad3767e17e31cc27eb586d1f9ed293a57acd43cece37677147b7b0af29f554d` |
| [逐项验证](evidence/runtime-ingress-20261024T033010763Z-94b39f3b/verification.json) | `c388c44bc7eaf2184ad27f4ec9221f6a5c84b1d530fd9edda7b5135306b7b44c` |

同目录保存 [50 次原始 Go 输出](evidence/runtime-ingress-20261024T033010763Z-94b39f3b/go-tool-output.json)、`logs/` 中 50 对 stdout/stderr、[24 次独立读取回执](evidence/runtime-ingress-20261024T033010763Z-94b39f3b/independent-go-inspection.json)、`independent-buildinfo/` 中 24 对输出、构建进程回执、原构建脚本、调用包装和独立复核脚本。构建前 [855 份历史文件清单](evidence/runtime-ingress-20261024T033010763Z-94b39f3b/historical-files.json) 包括既有运行产物、构建收据及部署文档/证据；在本批次构建与交付收尾检查时，855 份文件的大小与摘要全部一致，11 个历史构建收据目录均保留原样。

随后本阶段部署工作按授权更新了现行指南 [server.md](server.md) 和 [containers.md](containers.md)。集成后重新核对上述 855 份清单，仅这两份指南的字节发生预期变化，其余 853 份保持原大小与摘要，11 个历史构建收据仍原样；原始运行包和归档证据没有变化。具体核对时刻、两份指南的前后摘要、原始证据复核与当前文档摘要见 [历史集成补充检查](../quality/evidence/ingress-historical-integration-check.json)。

构建交付当时的收尾复核见 [最终回执](evidence/runtime-ingress-20261024T033010763Z-94b39f3b/final-verification.json)，当时的全部证据摘要见 [最终摘要清单](evidence/runtime-ingress-20261024T033010763Z-94b39f3b/sha256-final.json)。两者保留原字节；其中本页摘要对应补充说明前的原文版本，本页当前版本另由上述集成补充检查绑定。原始 `.cache/runtime-build/20261024T033010763Z-94b39f3b` 工具输出及 `.cache/runtime-ingress-delivery/20261024T033003962Z-b809f415` 审计收据仍保留，没有清理历史或唯一文件。

## 验收边界

阶段 Go 质量证据独立归档在 [Go 验证回执](../quality/evidence/ingress-go-verification.json)：实际 385 个通过事件（含子测试/helper）、0 失败、0 跳过测试场景，测试及 vet 退出 0，测试期间源码未变。运行源码与该测试快照的逐文件绑定见 [测试绑定复核](evidence/runtime-ingress-20261024T033010763Z-94b39f3b/go-test-binding.json)。此前受限环境启动失败原证据保留，不能代替最终主机执行结果。

本页证明离线交叉编译、实际文件格式与字节摘要、源码和依赖绑定及历史保留。六目标原生运行、安装升级、二进制签名、持久密钥恢复、浏览器闭环、真实公网/厂商交付、生产 CI、数据库集群、生产恢复及规模性能分别验收。Windows 程序未签名；Linux/macOS 在目标环境使用前需按系统设置执行权限。本批次没有重新构建桌面安装包，不表示完整 P0–P7 已完成。
