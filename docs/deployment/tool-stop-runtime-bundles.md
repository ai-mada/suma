# T-P4-004 停止语义修复后的正式运行包

日期：2026-10-24。交付批次：`20261024T051829203Z-df0ca9b5`。本批次重新构建六目标各四程序，共24个程序，并逐文件独立核对构建信息、大小、SHA-256与PE/ELF/Mach-O架构。

## 冻结范围与实际变化

运行输入仍为116份：114份非测试Go源码及go.mod、go.sum；清单摘要为 `3cdf6ee27eee0ffc6ec731a62c435c474708eeae8db0b48fbb6023bb28fd8b76`。构建前后、独立验证及最终归档再次读取均一致。它是有序紧凑JSON清单的摘要，区别于格式化source-manifest.json的文件摘要。

相对[首轮工具执行包](tool-runtime-bundles.md)的 `20261024T045219750Z-715ba2c8`，新增0个运行输入、修改2个、删除0个：`internal/dag/scheduler.go`、`internal/runner/tool_stream.go`。见[逐项差异](evidence/runtime-tools-stop-20261024T051829203Z-df0ca9b5/cut-difference.json)。停止回执在非取消运行中统一记录失败；即使OS退出0、工具证据缺失或协议声明完成，也不能启动依赖成功的下游。取消请求后的真实停止仍记录取消，外部效果未知仍进入核对。Codex用量接受官方最小三项input/cached/output；可选reasoning/cache-write字段如存在仍严格校验，不宣称本机0.164实际省略了这些字段。合同见[工具执行规范](../../contracts/tool-execution.md)。

网页、TS原生智能体、知识服务、Electron及外部CLI安装不在Go运行包中，各自验证范围由对应证据说明；本批次不替换历史桌面安装包的验收结论。

## 构建与独立核对

宿主Windows amd64、PowerShell7、Go `go1.24.5`。实际UTC构建区间 `2026-10-24T05:18:29.2257188Z` 至 `2026-10-24T05:19:20.3250160Z`。调用原有 `scripts/build-runtime.ps1 -All`，子进程GOMAXPROCS=2，原脚本退出0。50次Go调用（工具链1、离线模块校验1、编译24、元数据读取24）全部成功；独立复核又读取24份实际文件元数据与二进制头。

| 目标 | 实际格式 | 程序数 | 总字节 | 第三方模块并集 |
|---|---|---:|---:|---:|
| windows-amd64 | PE32+ | 4 | 61,955,072 | 35 |
| windows-arm64 | PE32+ | 4 | 58,293,760 | 35 |
| linux-amd64 | ELF64 | 4 | 60,397,988 | 33 |
| linux-arm64 | ELF64 | 4 | 57,669,823 | 33 |
| darwin-amd64 | Mach-O64 | 4 | 60,418,000 | 34 |
| darwin-arm64 | Mach-O64 | 4 | 57,978,216 | 34 |

实际元数据核对GOOS/GOARCH、CGO_ENABLED=0与trimpath；原始调用回执绑定buildvcs=false、mod=readonly及其余参数。依赖网络、工具链自动下载、VCS与telemetry关闭。缓存和临时目录位于仓库.cache；用户/机器环境只记录摘要并保持未变，见[环境核对](evidence/runtime-tools-stop-20261024T051829203Z-df0ca9b5/environment-verification.json)。跨目标链接模块并集36项，与前一工具运行包的标识、版本、校验和及replace逐项比较见[依赖核对](evidence/runtime-tools-stop-20261024T051829203Z-df0ca9b5/linked-module-comparison.json)；这不构成完整供应链、许可或安全验收。

## 实际交付位置与Windows字节

运行文件位于 `D:\codes\ai-dev-os\suma\dist\runtime\<target>\20261024T051829203Z-df0ca9b5`，四程序为suma-control、suma-runner、suma-deployer、suma（Windows有.exe扩展名）。原始构建收据位于 `dist/runtime/builds/20261024T051829203Z-df0ca9b5`。全部24份绝对路径、大小、摘要及格式见[独立结果](evidence/runtime-tools-stop-20261024T051829203Z-df0ca9b5/verification.json)。

| Windows目标 | 程序 | 字节 | SHA-256 |
|---|---|---:|---|
| windows-amd64 | suma-control.exe | 25,483,264 | `d357669448f9ccee63752b7177dcea18b56eeb9efaff01052332da43439a4c0f` |
| windows-amd64 | suma-runner.exe | 9,952,768 | `644adbf34d68fce6e65779819118751a64c2eb2872110915d0acdd6e4399d8ab` |
| windows-amd64 | suma-deployer.exe | 16,125,952 | `f8f6a32145d7679bbd6d04f7496c806e7e5c4f3afc8d84ee9ce8ce5098e8a433` |
| windows-amd64 | suma.exe | 10,393,088 | `3d989122ded5345c071e5ef57c1d3a89eda8d582f16e9f6b819fb13a164d82ca` |
| windows-arm64 | suma-control.exe | 24,070,656 | `6d40bf56106c7cb341636e5d56223b50dc30dbe3a9b36125456107d2fd1ceaa9` |
| windows-arm64 | suma-runner.exe | 9,246,208 | `b2d39789b65ed03b42946f08b2a4cb9afa71b7b344449237c31c5a4991732fc0` |
| windows-arm64 | suma-deployer.exe | 15,244,800 | `bf9ca2838b1138359dafe408e6a7999bca70542f4b44dc920bb60a2978827b1e` |
| windows-arm64 | suma.exe | 9,732,096 | `d4f3f1ca57181d17a344ce8f33cd4687a7d2c9de67c9ce1d11b1df1d38ea1c36` |

本交付代理不启动或停止任何程序；根代理后续激活应核对本表字节并单独记录真实运行证据，不能沿用首轮c06d/12c程序的运行结论。

## 测试切面、原件与历史保留

根代理修复后重新执行完整Go测试和vet：**512个通过测试事件、0失败、0测试跳过**。共189份测试源码快照，运行包116个输入逐文件精确匹配；实际原始JSONL重新计算摘要为 `003b24c1e8a83799e89d7c1a4aee4d493e03d6fbaff1c7a052099611d0942f82`。见[完整测试绑定](evidence/runtime-tools-stop-20261024T051829203Z-df0ca9b5/go-test-binding.json)和[未改写的原始摘要副本](evidence/runtime-tools-stop-20261024T051829203Z-df0ca9b5/go-test-receipt.original.json)。原始测试日志由质量档案单独保存，本交付步骤不重跑测试。

停止修复先保存实际RED，再实际GREEN：20格DAG停止/未知矩阵、4种工具证据的真实HTTP边界、固定候选测试停止、原取消/正常验收/补偿用例，目标共38个通过事件。最终RED有8个真实缺陷叶用例失败，见[修复验证](evidence/runtime-tools-stop-20261024T051829203Z-df0ca9b5/stopped-zero-exit-verification.json)、[最终RED](evidence/runtime-tools-stop-20261024T051829203Z-df0ca9b5/stopped-zero-exit-red-attempt3.jsonl)及[GREEN](evidence/runtime-tools-stop-20261024T051829203Z-df0ca9b5/stopped-zero-exit-green.jsonl)。首个RED中的issue关联断言误查旧列，随后改为正式data.run_id，原失败尝试保留；成功测试夹具原误设Stopped的五处已改为正常完成事实，生产没有绕过规则。

三份构建清单从dist逐字节复制，50对原始stdout/stderr、24对独立metadata输出及原脚本/包装器/验证器均保存。二进制仅在忽略的dist中，未复制到文档目录。最初证据摘要见[初始清单](evidence/runtime-tools-stop-20261024T051829203Z-df0ca9b5/sha256.json)；[最终复核](evidence/runtime-tools-stop-20261024T051829203Z-df0ca9b5/final-verification.json)绑定本页、运行源码、程序字节、原件和新Go切面，[最终摘要清单](evidence/runtime-tools-stop-20261024T051829203Z-df0ca9b5/sha256-final.json)覆盖新证据目录除自身以外的全部原字节。

预构建历史快照1321份，其中1309份旧运行产物/部署证据严格保留；最终相同1321份，当前指南更新逐项另列于[历史复核](evidence/runtime-tools-stop-20261024T051829203Z-df0ca9b5/historical-preservation-final.json)。首轮工具文档、479测试原始日志和旧批次177文件仍保持原字节；旧结果是旧切面，不能描述为已经包含这次停止/用量修复。

## 验收范围

本页证明离线交叉编译、宿主读取的元数据/文件格式/大小/摘要、精确源码与新完整Go测试切面绑定及历史保留。目标系统执行、签名、安装升级、外部账号/模型、分布式集群、生产与规模指标由独立证据证明。macOS/Linux和Windows arm64没有由交付代理实际运行；TS/网页/桌面也需分别验证，不以24份程序推断完整平台P0–P7已交付。
