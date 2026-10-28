# 1.0.0 当前 OpenCode SDK 路径与冗余清理验证

日期：2026-10-28；阶段：T-OPENCODE-104；负责人：主集成＋执行／SDK／控制面三个并行代理。基线：`b2462b47b2126a37a3c49f14bd4e8004de8bdedb`。[精简摘要](opencode-execution-summary.json)、[当前合同](../../contracts/opencode-sdk.md)、[架构与 AI SOP](../architecture/opencode-execution.md)。

## 当前交付

产品保持唯一 **1.0.0**。移除未完整实现的旧 OpenCode JSONL 执行 pair／parser、专用永久未验收分支和重复前端派发／证据 DTO，收敛为固定官方 SDK／plugin／server **1.19.4** 的单一当前路径。生成合同、示例、双语目录和当前参考矩阵同步更新；去掉矩阵入口的重复历史阶段流水。产品历史版本兼容、迁移、双写及第二个派发中心均未增加。

SDK 使用隔离配置和真实锁定插件依赖资产，经可信六工具调用已有 MCP／CAS／操作账本。普通／专家计划固定管理员配置，当前准入和私有／Pool 原子额度均拒绝 enforce；用量保持 partial／unreported 与未定价。整轮关联终态、原 session 取消、Go 进程清理和原回执继续独立校验，候选封存与精确 SHA 独立测试复用现有路径。

## 实际检查与范围

| 检查 | 实际结果与边界 |
| --- | --- |
| 共享合同与控制面 | 固定配置在 HTTP options、普通／专家预览、节点、开始、当前读取、领取与未定价账本中保持一致；组织／项目 enforce、漂移、重试、未绑定配置及伪造停止回执被拒绝。私有／Pool 回调和通用 DAG 分别验证原停止事实 |
| SDK 包 | 17 个定向用例通过；实际官方 SDK 调用本机自有 HTTP 协议端点，覆盖关联整轮终态、单次 prompt／不重放、丢失创建回执、取消、稳定工具身份、串行调用、失败闭合、配置隔离、真实依赖资产及双语帮助。没有启动实际 OpenCode serve 或提供方模型 |
| Runner 与原生宿主 | 自有 Node 协议子进程经过真实 stdin／MCP／文件账本／Git 封存；同操作重放保持一次效果，独立测试绑定精确候选 SHA；原始结果跨 ACK 丢失保留。不是官方 OpenCode CLI 编码证据 |
| 进程与异常 | OpenCode 子进程继承 Go 范围；已创建 session 的取消需要实际 abort ACK 和服务停止，再由 Go Stop／Finish 确认。含 2 秒延迟取消；宿主清理失败清除公开停止事实并保持 unknown。仅本机 Windows 检查，Unix／macOS运行未验 |
| Web | 7 文件共 99 个不同受影响用例通过；错误成功响应保留原操作、固定配置不变成可选择模型、CNEN展示无需新模型请求。前端类型检查通过；4625 对键，生产中文硬编码 0 |
| 浏览器 | 独立 SQLite 初始化、真实 Control HTTP 和自有库存夹具中，经产品入口选择流程与 OpenCode 工具、创建固定预览、展开配置、刷新恢复同一计划、切换中英文；未点击启动。关闭时运行／用量 attempt／厂商调用均为 0 |
| 构建 | 官方 SDK bundle／真实资产 manifest、最终 Web 构建与 Windows amd64 五程序构建通过；五个实际 `--version` 均为 1.0.0。Go 受影响包 vet、当前合同生成及版本检查通过 |

环境：Windows、Go 1.26.10（仓库离线工具链）、Node 22.16.0、TypeScript 5.7.3、pnpm 11.25.0。Go 检查串行分配时段；TS／UI／文档并行。未运行无关全平台测试或模型效果重复测试。

## 失败修正与证据

原始失败保留，不记成通过：TC 初始 RED；Runner 初始不支持协议；并行源码未齐时的一次编译缺口；两个测试比较 map／struct JSON 字段顺序；一处新测试未用变量；typed 配置与 durable JSON 身份差异；前端生成类型揭示 nullable／不完整测试夹具；中文文案断言顺序；删除旧显示项后的四处旧数量断言。只对受影响函数／文件回放。包含先前已通过的子集时不重复累加为新全套。

进程复查发现 SDK 的 Unix detached 子进程会脱离外层范围，已删除；当前服务保持 Go 范围。取消宽限仅 OpenCode 改为有界 10 秒；其他工具保持原值。补充的停止检查通过；Unix／macOS的实际停止保证仍待对应系统验收。

Go 原始回执在 `.cache/mcp019-deps/20261028T*.{command.json,stdout.txt,stderr.txt}`，精确命令／退出码／摘要见 JSON；SDK、GUI和 Web 原件位于 `.cache/e2e/opencode-104/`。GUI 预览编号 `c5890df6ee21a96379820550d1b91ba2`，刷新前后固定 `owned/model-a` 和原配置摘要不变。最后两条通用工具帮助文案更新后执行目录校验和最终构建，没有重跑模型或完整 GUI。

最终 Windows manifest：`dist/runtime/builds/20261028T002604908Z-382e6cea/manifest.json`，460 个生产 Go 输入，SHA256 `b3358cd72c818d66637c525b23ea5216411faa450c98c2b70bae462ecc91e31b`，构建前后源码不变。SDK 入口 SHA256 `ba780fea768677ac0c9eb10739c8127fd3f87d62d15e3134a2cc47b659cefda6`；manifest `0b3417bbca73fafab3a2c85617417cfadbca169e4d6f7af5cb618a8b0956252a`，3621 项真实资产／57,548,131 字节。最终 Web 41.42 秒，保留现有 Monaco／Flow 大块警告，不据此推断性能指标达标。

## 未完成与交付边界

本次没有取得“实际 OpenCode executable／serve／提供方模型＋本机 Go 宿主”的同一次完整厂商编码证据，也不证明厂商身份、完整账单、物理多机、Unix／macOS运行、安装签名或生产验收。SDK 服务配置、安装版探测与真实厂商端到端仍需独立验收。完整交互会话恢复、工具原生修复／规划／提炼／分诊与全平台 P0–P7、需求1–22继续实施。

用户原数据库与活动预览未改写；本次无新业务表。Git只收必要源码、配置例、依赖锁、合同、设计和此精简证据；缓存、依赖分发树、数据库、凭据、截图、原始日志及二进制保持忽略。产品统一 1.0.0不意味着重写 Git 历史或删除原业务证据。
