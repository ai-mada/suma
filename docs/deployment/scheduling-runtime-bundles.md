# T-P4-005 调度内核运行包与历史数据核对

日期：2026-10-24。批次：`20261024T064658380Z-b9221a5f`。本页记录六个目标各四个程序的独立文件验证，以及上一阶段历史数据库独立副本的只读迁移结果核对。**范围为交叉编译、文件字节、架构头和构建元数据；Windows 实际激活及浏览器旅程由主集成另行记录。**

## 固定输入与构建事实

本批次包含 133 份运行输入：131 份非测试 Go 源码及 `go.mod/go.sum`。范围为四个命令目录和 `internal`，未使用 `go:embed`；不包含网页、TS 原生智能体、Electron、外部 CLI 安装或数据库内容。

有序紧凑清单摘要为 `adb87de3ebbd6d716b25fd4232b8bd34b8eadbaa182b2a30f80d1c319665bc9b`。独立验证逐文件重算当前大小、SHA-256及完整文件集合，验证前后与构建清单一致；该摘要区别于格式化 JSON 文件本身的摘要。结果与复现范围见[阶段验证摘要](../quality/scheduling-verification.md)和[机器可读摘要](../quality/scheduling-summary.json)；原构建清单保留在本地 `dist/runtime/builds/20261024T064658380Z-b9221a5f/`。

原构建使用 Go `go1.24.5`，UTC 区间为 `2026-10-24T06:46:58.4173241Z` 至 `2026-10-24T06:47:55.8737258Z`，状态 `succeeded`，`source_unchanged=true`。50 次原 Go 调用均退出 0，包含工具链探测、离线模块检查、24 次编译及24次元数据读取；原始输出保留本地，不随源码提交。本页的独立核对再次用宿主 Go 读取全部24份实际文件的构建信息，不运行目标程序。

## 24 份程序独立验证

每份文件的长度、SHA-256、模块路径、工具链、GOOS／GOARCH、CGO、trimpath 与清单核对一致；按 PE32+、ELF64、Mach-O64 的实际头部读取机器架构。Go 构建信息仅忽略第一行文件路径差异，其余与原清单逐行相同。

| 目标 | 实际格式 | 程序数 | 总字节 |
|---|---|---:|---:|
| windows-amd64 | PE32+ | 4 | 62,380,032 |
| windows-arm64 | PE32+ | 4 | 58,685,440 |
| linux-amd64 | ELF64 | 4 | 60,809,722 |
| linux-arm64 | ELF64 | 4 | 58,029,453 |
| darwin-amd64 | Mach-O64 | 4 | 60,846,480 |
| darwin-arm64 | Mach-O64 | 4 | 58,339,832 |

实际元数据为 `CGO_ENABLED=0`、`-trimpath=true`、`-buildmode=exe`，amd64 为 `GOAMD64=v1`，arm64 为 `GOARM64=v8.0`。24 条原编译参数均包含 `-buildvcs=false`、`-mod=readonly`；原脚本摘要也与当前文件相同。独立重算跨程序模块并集为36项，模块标识、版本、校验和、替换以及目标／文件映射均一致，结果纳入[阶段摘要](../quality/scheduling-summary.json)。这不构成许可、漏洞或供应链审计。

程序位于 `D:\codes\ai-dev-os\suma\dist\runtime\<target>\20261024T064658380Z-b9221a5f`，分别为 `suma-control`、`suma-runner`、`suma-deployer`、`suma`，Windows 增加 `.exe`。原构建收据位于同仓库 `dist/runtime/builds/20261024T064658380Z-b9221a5f`。全部24份准确路径与摘要保留在本地 `docs/quality/evidence/scheduling-runtime/runtime-verification.json`；Git中的[阶段摘要](../quality/scheduling-summary.json)保留必要交付标识。

供后续 Windows amd64 激活核对的字节如下：

| 程序 | 字节 | SHA-256 |
|---|---:|---|
| suma-control.exe | 25,872,384 | `706ee2bdb0170dad2fba3c1ddb895126637336150e4c28e144017a5997e880ef` |
| suma-runner.exe | 9,988,608 | `9b6251f64f82a5700e99cb94223eec941076cc0f1a0b7d15d1150874de12d181` |
| suma-deployer.exe | 16,125,952 | `f8f6a32145d7679bbd6d04f7496c806e7e5c4f3afc8d84ee9ce8ce5098e8a433` |
| suma.exe | 10,393,088 | `3d989122ded5345c071e5ef57c1d3a89eda8d582f16e9f6b819fb13a164d82ca` |

交付核对代理没有启动、停止、安装或签名任何目标程序，不把宿主读元数据等同于目标系统实际可用。

## 上一阶段历史数据库保真

主集成先用 SQLite 在线只读备份取得独立 `history-before.db`，再复制为 `history-after.db`，以本阶段新服务／内核连续运行两次迁移。迁移方式和结果纳入[阶段验证摘要](../quality/scheduling-verification.md)。

独立核对只打开这两个任务副本，URI 使用 `mode=ro&immutable=1`、`query_only`，且先确认不存在非空 WAL，避免漏读已提交状态。没有打开活跃服务数据库，没有输出原始行，也没有归档数据库或上下文。对每个原表将全行规范化 JSON 排序后计算 SHA-256，采用与迁移前清单相同的二进制处理及编码方法。

结果为 **78 个原表的行数及全行逻辑摘要全部保持，差异 0，仅新增28表**。原有关键记录仍为4次运行、8个节点、5次尝试、46条事件、3项任务、2项需求和1项问题。不是只比较总行数；原表的全部字段内容都参与规范化摘要。

基线副本字节摘要仍为 `0f09fd3b935f084497842567ec372b7f701691b14fef0b76ce0bf216195c9849`，与最初快照清单一致；迁移后副本为 `969aed890b2bbad0be28e0c02d50646a4e423da53469b8f1b5ba5b44f8ac50cd`。两副本在独立只读验证前后字节未变。完整逐表清单保留在本地 `docs/quality/evidence/scheduling-runtime/history-migration-verification.json`。

该样本验证实际上一阶段数据保留和迁移可重入；活跃未知效果、旧预算与默认槽位等边界另见 [Core 回归与限制](../quality/scheduling-verification.md)。不据78表样本推导所有数据库、规模迁移、在线双写升级或无损回退已经验收。

## 本地原件与验收边界

原日志、元数据、字节收据与验证器保留在本地 `docs/quality/evidence/scheduling-runtime/`，不进入 Git；程序留在 `dist/runtime/`，数据库副本留在任务缓存。必要设计、复现方法和结果由[阶段验证摘要](../quality/scheduling-verification.md)保留，原件不删除或改写。

此前 T-P4-004 的交付与本地证据保留原样。新程序不复用旧运行结论。完整 Go／网页测试、Windows 激活、浏览器操作、安装升级、外部账户、OceanBase／PostgreSQL、分布式和生产规模分别验收；交叉编译不能证明完整平台P0–P7已经交付。
