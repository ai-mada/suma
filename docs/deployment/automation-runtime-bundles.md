# T-P4-002 自动化四程序运行包

日期：2026-10-24（Asia/Shanghai）。状态：六目标交叉编译、构建信息、交付文件摘要和源码绑定复核通过；目标系统运行、正式安装和签名分别验收。本页记录 Go 程序交付，阶段业务验证见 [统一验证报告](../quality/verification.md)。

当前批次：`20261024T023250966Z-97012787`。源码清单摘要 `source_sha256`：`cb40b728b0b540da3647df82aee986e2883e445738d0fed86a5f961e6f5d9f5a`。

## 源码范围与历史边界

四程序为 `suma-control`、`suma-runner`、`suma-deployer`、`suma`，入口和职责沿用 [P5 历史交付记录](release-runtime-bundles.md)。本批次在该 P5 源码基础上包含持久人工／五段 cron 自动化、固定流程版本的原子运行绑定、规则启停和授权运行事件 SSE 回放的 Go 实现；精确范围见 [自动化与事件合同](../../contracts/automation-events.md)。编译通过不单独证明这些业务行为已经验收。

源码清单共 93 个文件：`go.mod`、`go.sum`、上述四个入口和 `internal` 中的全部非测试 Go 文件。构建前、构建后以及独立复核时的清单一致，`source_unchanged=true`，当前文件与归档逐项匹配。未使用 `go:embed`；浏览器资源、Electron、Node 检索服务、测试文件、合同生成器、模型和外部工具安装均不在这份 Go 运行包中。

相对上一批次 `20261024T022519852Z-2a1cdc66`，本批次没有新增或删除运行源码，仅 `internal/platform/run_stream.go` 变化；[源码差异回执](evidence/runtime-automation-20261024T023250966Z-97012787/cut-difference.json) 保存前后 SHA-256。SSE 每次 Flush 成功后恢复至轮换截止时间加 2 秒的有界空闲截止时间，避免单次写入截止时间在 5 秒心跳间隔内过期。未发生实际写失败时，为返回后的终止 chunk 提供最多 2 秒、且不超过空闲截止时间的收尾宽限；每次 send 的 2 秒背压限制及流上下文截止时间约束保持不变，写失败的连接不会被重新放宽。

本批次保留上一批次的终态 metadata 修复：普通事件读取与授权 SSE 回放在反序列化后，将 nil `data` 映射为空对象 `{}`，保持公开对象合同；只修改返回的内存事件，不回写既有 `dag_events.data`。本页的编译和文件核对不代替至少 20 秒正常轮换、重启保持及浏览器业务验收。

| 历史批次 | 范围与证据 |
|---|---|
| `20261024T022519852Z-2a1cdc66` | [preidle-cut 历史文档](automation-runtime-preidle-20261024T022519852Z-2a1cdc66.md)，[原构建清单](evidence/runtime-automation-20261024T022519852Z-2a1cdc66/manifest.json)；终态 metadata 修复后、空闲 EOF 修复前，不得宣称该包空闲正常轮换通过 |
| `20261024T021451853Z-f8fe5e7d` | [prebrowser-cut 历史文档](automation-runtime-prebrowser-20261024T021451853Z-f8fe5e7d.md)，[原构建清单](evidence/runtime-automation-20261024T021451853Z-f8fe5e7d/manifest.json)；不得称为最终浏览器已验证版本 |
| `20261024T014255637Z-2381ecce` | [P5 最终历史记录](release-runtime-bundles.md)；后续自动化相对该批次新增 6 个运行文件、修改 4 个，见 [自动化初始源码差异](evidence/runtime-automation-20261024T021451853Z-f8fe5e7d/cut-difference.json) |

历史程序、收据及部署证据均原样保留；空闲 EOF 修复前运行包文档的原始字节另保存在当前证据目录，终态数据修复前文档的原始字节保留在上一批次证据目录。原 Windows 桌面安装包仍是 P5 之前的历史构建，未包含本轮 P4 自动化或 P5 发布功能。

## 实际构建

Windows amd64 宿主、PowerShell 7.6.5、已安装 Go `go1.24.5`，2026-10-24 10:32:50 至 10:34:44 执行：

```powershell
& ./scripts/build-runtime.ps1 -All
```

构建退出 0；先执行离线模块缓存验证，再编译六目标各四程序，并由宿主 Go 工具读取二进制构建信息。50 次 Go 子进程均退出 0，stderr 均为空。独立复核确认 24 个非空文件的实际大小与 SHA-256、归档构建信息及 `GOOS`、`GOARCH`、`CGO_ENABLED=0`、`-trimpath=true` 全部匹配。

| 目标 | 程序数 | 四程序合计字节 | 第三方链接模块并集 |
|---|---:|---:|---:|
| Windows amd64 | 4 | 61,414,912 | 35 |
| Windows arm64 | 4 | 57,830,400 | 35 |
| Linux amd64 | 4 | 59,851,296 | 33 |
| Linux arm64 | 4 | 57,231,915 | 33 |
| macOS amd64 | 4 | 59,895,088 | 34 |
| macOS arm64 | 4 | 57,492,472 | 34 |

跨目标第三方链接模块并集仍为 36；逐项路径、版本、校验和、替换项与上一自动化批次及 P5 最终历史批次一致，`linked-modules.json` 文件 SHA-256 也完全相同。清单仅覆盖实际链接的 Go 模块，不代替标准库、Node、Electron、外部工具及法律许可审查。

所有缓存、临时目录和 telemetry 目录均位于仓库 `.cache`；依赖网络、工具链自动下载和 telemetry 均关闭。环境设置仅传给构建子进程，独立核对用户级和机器级环境摘要保持一致，见 [环境复核](evidence/runtime-automation-20261024T023250966Z-97012787/environment-verification.json)。构建脚本 SHA-256：`66256a8d1454045f6db0e68641e1f6a7b1d1ad23341d9485fe45e5e8221859d2`。

## 文件位置与证据

```text
dist/runtime/<target>/20261024T023250966Z-97012787/
  suma-control[.exe]
  suma-runner[.exe]
  suma-deployer[.exe]
  suma[.exe]
  *.buildinfo.txt
  manifest.json
dist/runtime/builds/20261024T023250966Z-97012787/
  manifest.json
  source-manifest.json
  linked-modules.json
```

Windows amd64 控制面：`dist/runtime/windows-amd64/20261024T023250966Z-97012787/suma-control.exe`，25,113,088 字节，SHA-256 `5b18d6f0868bb3901ecd40c9ae4da444be92a7cc4e0fe34108accabe8bd91f8f`。实机验收应绑定这份交付字节；本构建记录没有启动目标程序。

| 归档证据 | 文件 SHA-256 |
|---|---|
| [原始构建清单](evidence/runtime-automation-20261024T023250966Z-97012787/manifest.json) | `aac21e65cd2fc814239107942b7f12f383287d004c97e5534fea3b3bc76ff8e3` |
| [源码清单](evidence/runtime-automation-20261024T023250966Z-97012787/source-manifest.json) | `58a2d6816c62097cb1519ba470dba16e13047d933c34c9f0b0c71e052ad93bb9` |
| [链接模块清单](evidence/runtime-automation-20261024T023250966Z-97012787/linked-modules.json) | `fad3767e17e31cc27eb586d1f9ed293a57acd43cece37677147b7b0af29f554d` |
| [逐项复核](evidence/runtime-automation-20261024T023250966Z-97012787/verification.json) | `88370b47d38b6fac0e99d513a0a4e571789dcc2547a6dedb2a995b97dd29ef26` |

`source_sha256` 是有序紧凑逐文件 JSON 清单的摘要，与上表带排版的 `source-manifest.json` 文件摘要不同。

同目录保存 [50 次真实 Go 调用输出](evidence/runtime-automation-20261024T023250966Z-97012787/go-tool-output.json)及 `logs/` 中的 50 对原始 stdout/stderr 文件、构建进程回执、[复核脚本](evidence/runtime-automation-20261024T023250966Z-97012787/verify-runtime.ps1)、[依赖比较](evidence/runtime-automation-20261024T023250966Z-97012787/linked-module-comparison.json)、[历史文件原摘要](evidence/runtime-automation-20261024T023250966Z-97012787/historical-files.json)、[交付收尾复核](evidence/runtime-automation-20261024T023250966Z-97012787/final-verification.json)和[全部证据摘要](evidence/runtime-automation-20261024T023250966Z-97012787/sha256.json)。归档清单与 `dist/runtime/builds` 原件逐字节相同，原 `.cache/runtime-build` 工具回执亦保留。

构建前记录的 668 份历史文件均已复核 SHA-256：388 份 `dist/runtime` 产物及收据、280 份部署历史证据；10 个既有构建收据目录均保留，包括 `021451` 和 `022519` 的全部原始文件。没有覆盖历史批次或删除唯一文件。

## 验收边界

本页仅证明实际交叉编译、宿主构建信息检查、文件摘要及源码／依赖绑定。六个目标的运行、安装升级、二进制签名、原生界面、远端部署、数据库集群、生产恢复和性能分别验收。Windows 程序未签名；Linux/macOS 复制后需按目标系统设置执行权限。Webhook/Git/CI/监控签名接入、厂商账号集成和完整分层公平调度仍按完整计划实施。
