# T-P4-006 共享池与国际化运行程序

状态：交叉编译、摘要核对及 Windows 本机控制面激活已验证；跨平台安装、签名和生产发布未验收。日期：2026-10-24。关联：[阶段验收](../quality/execution-pool-verification.md)、[池 Runner 接入](../agents/execution-pool-runner.md)。

## 当前固定批次

批次 `20261024T101540951Z-e9d5b0f7` 使用 Go 1.24.5，绑定 175 个生产输入（四个命令目录与 internal 非测试 Go 文件，以及 go.mod/go.sum）；有序源码摘要为 `bdafacdab242d3a991581492f63ecd5478a1cc86492378881bb995d302f7f462`。编译前后源清单一致。此清单不包含网页、Electron、TypeScript 智能体、外部工具安装或数据库内容。

六个目标 `windows-amd64`、`windows-arm64`、`linux-amd64`、`linux-arm64`、`darwin-amd64`、`darwin-arm64` 各包含 `suma-control`、`suma-runner`、`suma-deployer`、`suma`，共 24 份程序；Windows 增加 `.exe`。逐文件长度与 SHA-256 独立重算均与清单相同。构建使用离线依赖缓存、`CGO_ENABLED=0`、`-trimpath`、`-mod=readonly`、`-buildvcs=false`，并读取宿主可识别的 Go 构建信息。

另一次独立只读检查实际 PE32+／ELF64／Mach-O64 头部、目标架构，以及重新读取的24份 Go 构建信息，均与清单一致；链接模块并集36项，逐目标／程序集合一致，不一致0，175源码前后摘要未变。独立收据摘要为 `2e6442ced04ab6eb855ea6a0f0c5c7bb80a30a9348a9d565abda6ef8c59d07e4`；该检查没有启动目标程序。

本地交付路径为仓库 `dist/runtime/<target>/20261024T101540951Z-e9d5b0f7/`，完整收据为 `dist/runtime/builds/20261024T101540951Z-e9d5b0f7/manifest.json`，独立核对原件为 `.cache/e2e/pool-stage/runtime-final-verification.json`。这些二进制、完整原件和缓存不进入 Git。复现入口是 `scripts/build-runtime.ps1 -All`；原始脚本随源码维护。

| Windows amd64 程序 | SHA-256 |
| --- | --- |
| suma-control.exe | `c82be9d57c15162dc19fb6eff0b5ce926a096135617152980e803f436a41b481` |
| suma-runner.exe | `5e920312e5a5edbf1efadf5c4e753ad0bb49cf1c5981fa89bcb9fa040195bf33` |
| suma-deployer.exe | `f8f6a32145d7679bbd6d04f7496c806e7e5c4f3afc8d84ee9ce8ce5098e8a433` |
| suma.exe | `3b45097f1497a43dd21c73f0af9a9e56f3606ad807594ef3f45be5073ed8b1ed` |

## 本机实际激活

隔离验收服务只绑定 `127.0.0.1:4324`，浏览器通过 `pool.localhost` 访问。核对旧进程 PID 与可执行文件完整路径后，仅停止本任务控制面，切换上述正式控制面程序；原数据目录与会话保持，`/healthz` 实际返回数据库已连接。没有停止其他阶段或用户服务。

当前共享 Runner 从此前正式批次启动；其运行文件与本批次 Runner 字节完全相同，因此保持原进程、机器身份、配置和逐槽状态。控制面短暂停机期间出现的 `session_unavailable` 按真实事实保留，不作为业务成功或测试通过。后续实际业务执行、事件和验收由阶段报告单列。

## 切面与未覆盖项

最终完整 Go 回归是 714 个通过事件、294 输入不变。之后仅三个 CLI 文件修复 `localized:false` 的存在性保真，再独立验证完整 CLI 33 个通过事件及静态检查；其余 291 输入保持原摘要。当前程序绑定修正后的源码，不把两个测试批次合并成一次全域计数。

历史迁移实验绑定旧批次 `20261024T094718972Z-d2914aa4`，与新批次的源码摘要分开记录。当前交叉编译不代表 macOS／Linux 实机运行、Windows 安装升级、Electron 新安装包、代码签名、厂商模型、OceanBase 集群、生产部署或规模 SLO 已验收。
