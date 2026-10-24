# T-P4-002 自动化运行包历史：prebrowser-cut

本页保存 `20261024T021451853Z-f8fe5e7d` 浏览器修复前的交叉编译记录。后续发现合法成功终态事件的 `data:null` 与公开对象合同冲突，导致客户端停在终态事件之前；该批次不能称为最终浏览器已验证版本。当前源码包见 [自动化运行包索引](automation-runtime-bundles.md)。下文的源码一致性与交付结论均限定在该历史批次归档时。

日期：2026-10-24（Asia/Shanghai）。状态：六目标交叉编译、构建信息、交付文件摘要和源码绑定复核通过；目标系统运行、正式安装和签名分别验收。本页记录 Go 程序交付，阶段业务验证见 [统一验证报告](../quality/verification.md)。

本页历史批次：`20261024T021451853Z-f8fe5e7d`。源码清单摘要 `source_sha256`：`8d7891e3bb3eb272588a8f297805338f9a48ec767880c638621feaefc7c9a43c`。

## 源码范围与历史边界

四程序为 `suma-control`、`suma-runner`、`suma-deployer`、`suma`，入口和职责沿用 [P5 历史交付记录](release-runtime-bundles.md)。本批次在该 P5 源码基础上包含持久人工／五段 cron 自动化、固定流程版本的原子运行绑定、规则启停和授权运行事件 SSE 回放的 Go 实现；精确范围见 [自动化与事件合同](../../contracts/automation-events.md)。编译通过不单独证明这些业务行为已经验收。

源码清单共 93 个文件：`go.mod`、`go.sum`、上述四个入口和 `internal` 中的全部非测试 Go 文件。构建前、构建后以及独立复核时的清单一致，`source_unchanged=true`，该次归档时文件与清单逐项匹配。未使用 `go:embed`；浏览器资源、Electron、Node 检索服务、测试文件、合同生成器、模型和外部工具安装均不在这份 Go 运行包中。

与 P5 最终历史批次 `20261024T014255637Z-2381ecce` 相比，新增 6 个运行源码文件，修改 4 个，无删除；[源码差异回执](evidence/runtime-automation-20261024T021451853Z-f8fe5e7d/cut-difference.json) 保存精确路径和前后 SHA-256。旧 P5 包和原始收据原样保留。原 Windows 桌面安装包仍是 P5 之前的历史构建，未包含本轮 P4 自动化或 P5 发布功能。

## 实际构建

Windows amd64 宿主、PowerShell 7.6.5、已安装 Go `go1.24.5`，2026-10-24 10:14:51 至 10:15:39 执行：

```powershell
& ./scripts/build-runtime.ps1 -All
```

构建退出 0；先执行离线模块缓存验证，再编译六目标各四程序，并由宿主 Go 工具读取二进制构建信息。50 次 Go 子进程均退出 0，stderr 均为空。独立复核确认 24 个非空文件的实际大小与 SHA-256、归档构建信息及 `GOOS`、`GOARCH`、`CGO_ENABLED=0`、`-trimpath=true` 全部匹配。

| 目标 | 程序数 | 四程序合计字节 | 第三方链接模块并集 |
|---|---:|---:|---:|
| Windows amd64 | 4 | 61,427,712 | 35 |
| Windows arm64 | 4 | 57,826,304 | 35 |
| Linux amd64 | 4 | 59,872,295 | 33 |
| Linux arm64 | 4 | 57,235,066 | 33 |
| macOS amd64 | 4 | 59,886,800 | 34 |
| macOS arm64 | 4 | 57,492,376 | 34 |

跨目标第三方链接模块并集仍为 36；逐项路径、版本、校验和、替换项与 P5 最终历史批次一致，`linked-modules.json` 文件 SHA-256 也完全相同。清单仅覆盖实际链接的 Go 模块，不代替标准库、Node、Electron、外部工具及法律许可审查。

所有缓存、临时目录和 telemetry 目录均位于仓库 `.cache`；依赖网络、工具链自动下载和 telemetry 均关闭。环境设置仅传给构建子进程，独立核对用户级和机器级环境摘要保持一致，见 [环境复核](evidence/runtime-automation-20261024T021451853Z-f8fe5e7d/environment-verification.json)。构建脚本 SHA-256：`66256a8d1454045f6db0e68641e1f6a7b1d1ad23341d9485fe45e5e8221859d2`。

## 文件位置与证据

```text
dist/runtime/<target>/20261024T021451853Z-f8fe5e7d/
  suma-control[.exe]
  suma-runner[.exe]
  suma-deployer[.exe]
  suma[.exe]
  *.buildinfo.txt
  manifest.json
dist/runtime/builds/20261024T021451853Z-f8fe5e7d/
  manifest.json
  source-manifest.json
  linked-modules.json
```

Windows amd64 控制面：`dist/runtime/windows-amd64/20261024T021451853Z-f8fe5e7d/suma-control.exe`，25,125,888 字节，SHA-256 `5e3ad2bea83ac90d75e381da335e39a973cabb8ac61bcd7080bf47d2597ed732`。后续实机验收应使用当前索引的新批次；本构建记录没有启动目标程序。

| 归档证据 | 文件 SHA-256 |
|---|---|
| [原始构建清单](evidence/runtime-automation-20261024T021451853Z-f8fe5e7d/manifest.json) | `c9244b0665ea1bbc6c71a46ecff0ec03ffddd8c95f3677f9a889ce1132ebc666` |
| [源码清单](evidence/runtime-automation-20261024T021451853Z-f8fe5e7d/source-manifest.json) | `33982912e20f0ea44c6fce00323b7e574a6e2e8e70121bf89d3a0c44cf10310a` |
| [链接模块清单](evidence/runtime-automation-20261024T021451853Z-f8fe5e7d/linked-modules.json) | `fad3767e17e31cc27eb586d1f9ed293a57acd43cece37677147b7b0af29f554d` |
| [逐项复核](evidence/runtime-automation-20261024T021451853Z-f8fe5e7d/verification.json) | `06181ef74059cf520651df007bac41538df7d01caa78b61865d668dc2948d3b1` |

`source_sha256` 是有序紧凑逐文件 JSON 清单的摘要，与上表带排版的 `source-manifest.json` 文件摘要不同。

同目录保存 [50 次真实 Go 调用输出](evidence/runtime-automation-20261024T021451853Z-f8fe5e7d/go-tool-output.json)及 `logs/` 中的 50 对原始 stdout/stderr 文件、构建进程回执、[复核脚本](evidence/runtime-automation-20261024T021451853Z-f8fe5e7d/verify-runtime.ps1)、[依赖比较](evidence/runtime-automation-20261024T021451853Z-f8fe5e7d/linked-module-comparison.json)、[历史文件原摘要](evidence/runtime-automation-20261024T021451853Z-f8fe5e7d/historical-files.json)、[交付收尾复核](evidence/runtime-automation-20261024T021451853Z-f8fe5e7d/final-verification.json)和[全部证据摘要](evidence/runtime-automation-20261024T021451853Z-f8fe5e7d/sha256.json)。归档清单与 `dist/runtime/builds` 原件逐字节相同，原 `.cache/runtime-build` 工具回执亦保留。

构建前记录的 319 份历史文件均已复核 SHA-256：274 份 `dist/runtime` 产物及收据、45 份部署历史证据；8 个既有构建收据目录均保留。没有覆盖历史批次或删除唯一文件。

## 验收边界

本页仅证明实际交叉编译、宿主构建信息检查、文件摘要及源码／依赖绑定。六个目标的运行、安装升级、二进制签名、原生界面、远端部署、数据库集群、生产恢复和性能分别验收。Windows 程序未签名；Linux/macOS 复制后需按目标系统设置执行权限。Webhook/Git/CI/监控签名接入、厂商账号集成和完整分层公平调度仍按完整计划实施。
