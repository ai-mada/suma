# P5 四程序运行包历史构建记录

状态：实际交叉编译及交付文件校验通过，目标系统运行与正式安装分别待验收。日期：2026-10-24（Asia/Shanghai）。负责人：主集成代理／运行包构建代理。关联需求：P5 制品与发布闭环、P7 跨平台交付；接口见 [发布控制面](../../contracts/release-control.md) 与 [发布执行](../../contracts/release-execution.md)。实现为 `scripts/build-runtime.ps1`。

本页保留 P5 阶段的历史源码批次、摘要和原始证据。T-P4-002 自动化源码的当前四程序交付为 `20261024T023250966Z-97012787`，见 [自动化运行包索引](automation-runtime-bundles.md)；下文“最终”均限定为本页 P5 阶段，不表示已包含后续自动化源码。原 Windows 桌面安装包仍为 P5 之前的历史构建，未包含本轮 P4 自动化或 P5 发布功能。

## 程序与版本边界

| 程序 | 入口 | 职责 |
|---|---|---|
| `suma-control` | `cmd/suma-control` | 正式业务、制品证明、发布候选、审批与管理 DAG |
| `suma-runner` | `cmd/suma-runner` | 独立工作树、真实构建与包测试、受控任务执行 |
| `suma-deployer` | `cmd/suma-deployer` | 本机静态制品激活、内容健康检查、持久操作与核对 |
| `suma` | `cmd/suma` | 平台命令行入口 |

本页 P5 最终批次包含 P5 本机发布代码、环境读取事务授权、最终问题分类修正及提交前 `gofmt`：通用 `run_failed` 来源按执行问题记录，既有健康 Provider 观测保留原事实，Provider 专属问题投影不变。精确文件版本由源码清单定位。本机静态提供方的范围及配置见 [发布控制面部署](release-control.md) 和 [提供方接入](../integrations/release-provider.md)。这批 Go 程序不包含浏览器资源、Electron、Node 检索服务、模型或外部工具安装。

本页 P5 历史交付以 `20261024T014255637Z-2381ecce` 为准，目标平台主表及精确摘要见下文。与上一批次的运行时源码清单相比，唯一变化是 `internal/platform/release_deploy_plans.go` 的格式调整；[最终清单差异](evidence/runtime-release-20261024T014255637Z-2381ecce/cut-difference.json) 保存文件摘要。测试文件有独立验收记录，未计入这 87 个运行时源码文件。

## 历史批次（原样保留）

三份历史批次共 66 个程序与 9 份收据，合计 75 个文件均已在最终构建前后逐一核对 SHA-256，保持原样。此前桌面安装包也未被本次构建覆盖，不能宣称它们包含本轮 P5 功能。

| 历史批次 | 源码清单摘要 `source_sha256` | 边界与原始收据 |
|---|---|---|
| `20261024T013901937Z-e63107ea` | `b200a2fb11c33bc52621902443c1cb8602917b5f45a8f0235945618e00167d05` | 分类修正后、最终格式调整前；[构建清单](evidence/runtime-release-20261024T013901937Z-e63107ea/manifest.json) |
| `20261024T013101080Z-8d76ad7a` | `1880ef47cf9e28aab5e00686fdd063e482e8a26f0bd709ed6856342e19cace5e` | 分类修正前；[构建清单](evidence/runtime-release-20261024T013101080Z-8d76ad7a/manifest.json) |
| `20261023T163759813Z-4b7523bf` | `bcbd66b6732f635d8850ad9b3aa80d6d1ea3df40ed4f9a617f0313c7062e70a8` | P5 前的三程序／18 文件；[历史构建记录](runtime-bundles.md) |

两份历史 P5 批次之间，唯一运行时源码差异为 `internal/platform/release_failure_projection.go`；[分类修正差异](evidence/runtime-release-20261024T013901937Z-e63107ea/cut-difference.json) 保留修改前后摘要。历史包与原始证据不能替代最终交付包的源码绑定。

## 实际构建与文件校验

Windows amd64 宿主，PowerShell 7，已安装 Go `go1.24.5`；2026-10-24 09:42:55 至 09:43:58 执行：

```powershell
& ./scripts/build-runtime.ps1 -All
```

构建退出 0。先检验离线模块缓存，再依次实际编译六个目标的四个程序，宿主工具读取每个二进制的构建信息并校验 `GOOS`、`GOARCH`、`CGO_ENABLED=0`。50 次 Go 子进程均退出 0，原始 stderr 均为空；本轮没有启动目标程序。所有 24 个交付文件的大小和实际 SHA-256 已与构建清单再次逐一核对。

| 目标 | 编译／构建信息 | 四程序合计字节 | 实际链接模块并集 |
|---|---|---:|---:|
| Windows amd64 | 4/4 通过 | 60,782,080 | 35 |
| Windows arm64 | 4/4 通过 | 57,228,800 | 35 |
| Linux amd64 | 4/4 通过 | 59,229,047 | 33 |
| Linux arm64 | 4/4 通过 | 56,591,754 | 33 |
| macOS amd64 | 4/4 通过 | 59,274,784 | 34 |
| macOS arm64 | 4/4 通过 | 56,904,488 | 34 |

批次 ID 为 `20261024T014255637Z-2381ecce`；每个目标目录均使用此新 ID：

```text
dist/runtime/<target>/20261024T014255637Z-2381ecce/
  suma-control[.exe]
  suma-runner[.exe]
  suma-deployer[.exe]
  suma[.exe]
  *.buildinfo.txt
  manifest.json
dist/runtime/builds/20261024T014255637Z-2381ecce/
  manifest.json
  source-manifest.json
  linked-modules.json
```

构建和临时文件仅使用仓库 `.cache`，关闭模块下载、工具链自动下载和 telemetry；没有修改父进程、用户或机器环境变量。构建脚本 SHA-256 为 `66256a8d1454045f6db0e68641e1f6a7b1d1ad23341d9485fe45e5e8221859d2`。

## 源码与证据

源码清单包含 `go.mod`、`go.sum`、上述四个入口和 `internal` 下的全部非测试 Go 文件，共 87 个；包含 `cmd/suma-deployer/main.go`、`internal/deployer`、`internal/releasecontract` 及共享业务实现。程序目录从构建程序清单派生，避免新增程序后漏记入口。构建前后及归档时当前文件摘要一致。独立清单探针通过；在副本中移除提供方程序后，同一探针退出 1。

`source_sha256` 是逐文件清单的有序紧凑 JSON 摘要：`b8d6a3075cf29836c11ab8033bd5aab229eb6f9e5fb80c3402ede458e6a76365`。它与带排版的 `source-manifest.json` 文件 SHA 是不同值，不可混用。

| 归档证据 | 文件 SHA-256 |
|---|---|
| [构建清单](evidence/runtime-release-20261024T014255637Z-2381ecce/manifest.json) | `fa3cca1e386cb870142100780df157e2fde3a279b5f89b617e1ac6e4c9217464` |
| [源码清单](evidence/runtime-release-20261024T014255637Z-2381ecce/source-manifest.json) | `2f8f4ec287c7ae7968cc05ddfe5703c293608854af3c32e1b1a064bfa2905086` |
| [链接模块清单](evidence/runtime-release-20261024T014255637Z-2381ecce/linked-modules.json) | `fad3767e17e31cc27eb586d1f9ed293a57acd43cece37677147b7b0af29f554d` |
| [逐项复核](evidence/runtime-release-20261024T014255637Z-2381ecce/verification.json) | `98bad0e49b0d9a10d99455b8e1929dd9016faaf856a1415b400ce0b3e8a83133` |

同目录保留 [50 次真实工具输出](evidence/runtime-release-20261024T014255637Z-2381ecce/go-tool-output.json)、[清单探针](evidence/runtime-release-20261024T014255637Z-2381ecce/inventory-probe.ps1)、[缺少提供方的反向探针结果](evidence/runtime-release-20261024T014255637Z-2381ecce/regression-probe.json)、[历史文件原摘要](evidence/runtime-release-20261024T014255637Z-2381ecce/historical-files.json) 及 [全部证据摘要](evidence/runtime-release-20261024T014255637Z-2381ecce/sha256.json)。跨目标第三方链接模块并集仍为 36，清单仅覆盖实际链接 Go 模块，不代替标准库、Node、Electron、二进制工具及法律许可审查。

## 验收边界

这份记录证明本批次实际交叉编译、构建信息与文件摘要，不能代替六个平台的运行、安装升级、签名、原生界面、数据库集群、跨机器部署、生产恢复或性能验收。本轮控制面／提供方的真实本机行为另见 [发布控制面验证](../quality/release-control-verification.md)，不能把源码测试结果自动归入跨平台运行验收。Windows 程序未签名；Linux/macOS 复制后需按目标系统设置执行权限。完整平台仍按 P0–P7 计划持续实施。
