# T-P4-004 工具执行与任务派发四程序运行包

日期：2026-10-24（Asia/Shanghai）。交付批次：`20261024T045219750Z-715ba2c8`。六目标各四程序已实际离线交叉编译，共24个程序；随后逐文件独立读取构建信息，核对实际架构、大小及摘要。该结果证明构建与字节，不替代目标操作系统运行、安装、签名或生产验收。

## 冻结源码与范围

本切面基于 `0fe174f668f6e9d111404f72459f34bf3bccde8e` 加T-P4-004工作区，精确绑定 **116个输入文件**：114份非测试Go源码及go.mod、go.sum。源码清单摘要为 `498300c9adffa8327982faa25da7d745d1a9ecb08f62265691d5673deacb318e`，来自有序紧凑逐文件JSON；它与格式化source-manifest.json的文件摘要不同。构建前后及独立复核均一致，`source_unchanged=true`。

范围为四个入口 `cmd/suma-control`、`cmd/suma-runner`、`cmd/suma-deployer`、`cmd/suma` 及 `internal` 下非测试Go文件，没有go:embed资源。网页资源、Electron、TS原生智能体与知识服务、合同生成器、测试文件和外部CLI安装均不在运行包内；本批次不宣称这些TS资产已整体冻结或随包交付。

相对签名入口批次 `20261024T033010763Z-94b39f3b`，新增15个运行文件、修改16个、删除0个，见[逐项差异](evidence/runtime-tools-20261024T045219750Z-715ba2c8/cut-difference.json)。新增工具档案、实时库存与固定工具绑定、任务上下文预览和原子派发、typed进度/终态证据、当前执行身份及命令能力筛选，具体语义见[工具执行合同](../../contracts/tool-execution.md)。原程序包、构建收据和历史安装包保留；历史Electron安装包不能被描述为包含本阶段功能。

## 实际构建与独立复核

Windows amd64宿主、PowerShell 7、Go `go1.24.5`。实际UTC构建区间 `2026-10-24T04:52:19.7694102Z` 至 `2026-10-24T04:53:06.4852888Z`，原脚本退出0：

```powershell
# GOMAXPROCS=2仅由包装器传给此次子进程。
& ./scripts/build-runtime.ps1 -All
```

共50次原始Go调用：工具链检查1次、离线模块校验1次、构建24次、构建信息读取24次，全部退出0。独立验证又对24个实际交付文件执行宿主 `go version -m`，比较原始构建信息、实际文件大小和SHA-256，再读取PE/ELF/Mach-O头确认目标架构。`CGO_ENABLED=0`、`GOOS/GOARCH` 和trimpath由实际元数据核验；buildvcs=false、mod=readonly及各参数由原始命令回执绑定。

| 目标 | 实际格式 | 程序数 | 四程序总字节 | 链接第三方模块并集 |
|---|---|---:|---:|---:|
| windows-amd64 | PE32+ | 4 | 61,955,072 | 35 |
| windows-arm64 | PE32+ | 4 | 58,293,760 | 35 |
| linux-amd64 | ELF64 | 4 | 60,398,140 | 33 |
| linux-arm64 | ELF64 | 4 | 57,669,999 | 33 |
| darwin-amd64 | Mach-O64 | 4 | 60,418,000 | 34 |
| darwin-arm64 | Mach-O64 | 4 | 57,978,216 | 34 |

跨目标第三方模块并集为36个，标识、版本、校验和及replace信息与上一签名入口批次一致；完整linked-modules.json字节也一致，见[依赖比较](evidence/runtime-tools-20261024T045219750Z-715ba2c8/linked-module-comparison.json)。这不是标准库、Node、Electron、CLI插件或完整供应链的许可／安全证明。

GOMAXPROCS=2、构建/模块缓存、临时目录和telemetry设置仅对构建子进程生效；缓存位于仓库.cache，依赖网络、工具链自动下载和VCS获取关闭。构建包装与独立复核未修改用户级或机器级环境，摘要核对见[环境证据](evidence/runtime-tools-20261024T045219750Z-715ba2c8/environment-verification.json)。原构建脚本摘要为 `66256a8d1454045f6db0e68641e1f6a7b1d1ad23341d9485fe45e5e8221859d2`。

## 实际交付位置

```text
dist/runtime/<target>/20261024T045219750Z-715ba2c8/
  suma-control[.exe]
  suma-runner[.exe]
  suma-deployer[.exe]
  suma[.exe]
  *.buildinfo.txt
  manifest.json
dist/runtime/builds/20261024T045219750Z-715ba2c8/
  manifest.json
  source-manifest.json
  linked-modules.json
```

实际绝对根目录为 `D:\codes\ai-dev-os\suma\dist\runtime`。Windows amd64目录为 `D:\codes\ai-dev-os\suma\dist\runtime\windows-amd64\20261024T045219750Z-715ba2c8`，arm64相应替换目标名。24份程序的绝对路径、大小、摘要及格式均在[独立验证](evidence/runtime-tools-20261024T045219750Z-715ba2c8/verification.json)的binaries中。

| Windows目标 | 程序名 | 字节 | SHA-256 |
|---|---|---:|---|
| windows-amd64 | suma-control.exe | 25,483,264 | `c06d3553c46278704dd976f5ab1dcef52d3b25a96f48d76e3bed4d08092b4eb8` |
| windows-amd64 | suma-runner.exe | 9,952,768 | `12c8681ec9d7e1ba325252e264e5d7aaa7d73d9c053cc3d6c3466292b44f56cb` |
| windows-amd64 | suma-deployer.exe | 16,125,952 | `f8f6a32145d7679bbd6d04f7496c806e7e5c4f3afc8d84ee9ce8ce5098e8a433` |
| windows-amd64 | suma.exe | 10,393,088 | `3d989122ded5345c071e5ef57c1d3a89eda8d582f16e9f6b819fb13a164d82ca` |
| windows-arm64 | suma-control.exe | 24,070,656 | `262e06cc580454cfb7136b3c8207e7f3d27981ea856c11cf0002ce0ffac57600` |
| windows-arm64 | suma-runner.exe | 9,246,208 | `9dc44bf36912de751e4e45729b0027d71464597fdcf1366e46e20fa2b072a98c` |
| windows-arm64 | suma-deployer.exe | 15,244,800 | `bf9ca2838b1138359dafe408e6a7999bca70542f4b44dc920bb60a2978827b1e` |
| windows-arm64 | suma.exe | 9,732,096 | `d4f3f1ca57181d17a344ce8f33cd4687a7d2c9de67c9ce1d11b1df1d38ea1c36` |

根代理后续实际切换应核对本表的control和Runner字节；此前localhost4322候选服务不是这批最终交付字节。构建及文档交付代理没有启动、停止或替换任何运行服务，本页不记录目标程序已经启动的结论。

## 原件、测试绑定与历史保护

| 归档文件 | 文件SHA-256 |
|---|---|
| [原始构建清单](evidence/runtime-tools-20261024T045219750Z-715ba2c8/manifest.json) | `02677f502364e61e2bae0078145d343686a9f4f80e5995f883ace2e025b9d3f1` |
| [运行源码清单](evidence/runtime-tools-20261024T045219750Z-715ba2c8/source-manifest.json) | `f20a4ac64cec9757bbe617e6eea0afa59351d6268fb7abd919f1a3a6d8af2c37` |
| [链接模块清单](evidence/runtime-tools-20261024T045219750Z-715ba2c8/linked-modules.json) | `fad3767e17e31cc27eb586d1f9ed293a57acd43cece37677147b7b0af29f554d` |
| [独立验证](evidence/runtime-tools-20261024T045219750Z-715ba2c8/verification.json) | `45c0a6c6589a5c46fba98bb9908bd59dceb39602273f4407d232b4627ff75a5b` |

[证据目录](evidence/runtime-tools-20261024T045219750Z-715ba2c8/verification.json)包含从dist原件逐字节复制的三份构建清单、50对原始stdout/stderr、24对独立metadata输出、原构建脚本、包装器及独立校验脚本。二进制只保留在忽略的dist中，不复制到文档目录。

最终Go全套已在根代理记录中实际通过：**479个通过测试事件、0失败、0测试跳过**，含父/子测试和helper；10个受测包、4个命令包仅编译，test及vet均退出0。运行包116份输入全部与该次187文件源码快照匹配，原始日志摘要重新核对为 `db5a89e2792d2d4dec579ac30e208aee27d6e520c4323c6ca4f1de05b8cdf828`。见[测试快照绑定](evidence/runtime-tools-20261024T045219750Z-715ba2c8/go-test-binding.json)和[原始测试回执副本](evidence/runtime-tools-20261024T045219750Z-715ba2c8/go-test-receipt.original.json)。本交付步骤没有重跑测试；原始JSONL由质量档案单独保存，避免把改写raw_file后的质量摘要与此原始副本混同。

构建前先记录1086份历史文件，其中1075份旧dist运行文件及部署evidence为不可变保护对象；12个既有构建收据目录保留。首次独立验证时1086份全部相同，见[原始历史清单](evidence/runtime-tools-20261024T045219750Z-715ba2c8/historical-files.json)与[分层保护检查](evidence/runtime-tools-20261024T045219750Z-715ba2c8/historical-preservation.json)。根代理集成期间对现行Markdown部署指南的更新与旧不可变产物分开记录，不重写初始快照；最后状态见[最终历史复核](evidence/runtime-tools-20261024T045219750Z-715ba2c8/historical-preservation-final.json)。

[最终交付复核](evidence/runtime-tools-20261024T045219750Z-715ba2c8/final-verification.json)绑定本页、所有运行源码、24份程序、原始收据及测试快照；[最终证据摘要清单](evidence/runtime-tools-20261024T045219750Z-715ba2c8/sha256-final.json)覆盖该证据目录除清单自身之外的原字节。

## 验收边界

本页仅证明当前Go运行时的离线构建、元数据、文件格式与摘要、源码/依赖/测试切面绑定及旧原件保留。Windows程序未签名；macOS/Linux和Windows arm64均未由交付代理运行，目标执行权限、原生安装升级、正式签名、外部工具账户与模型执行、真实集群、生产及规模验收由对应证据单独证明。浏览器、原生TS runtime及Electron仍需各自构建与验证，不以24个Go程序推断完整P0–P7交付完成。
