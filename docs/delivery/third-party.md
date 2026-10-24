# 第三方来源与依赖清单

状态：本地依赖元数据快照；最终分发清单及许可审查未完成。日期：2026-10-24。Owner：集成、交付与发布负责人。

关联：[P0/P7 实施计划](implementation-plan.md)、[能力矩阵](../product/capability-matrix.md)、[验证报告](../quality/verification.md)。证据：[锁定参考](../integrations/references.json)、[依赖与许可证文件哈希](third-party-dependencies.json)、[Go 依赖声明](../../go.mod)、[JS 锁文件](../../pnpm-lock.yaml)。本页记录来源和观察结果，不提供法律结论或分发许可保证。

## 参考项目与实际依赖分开记录

下表的“参考”是产品、架构或协议研究。锁定 README/提交不代表完整代码审计、源代码移植、完整产品接入或许可证逐文件审查。原始获取方法、README blob 和时间保留在 references.json。

| 项目 | 锁定提交 / 基线 | 本项目用途与集成状态 |
| --- | --- | --- |
| Multica | `dbdbb299164347586e2650bb67684a490726a798`（fetch）；原工作区 HEAD `18fa9f728a9e25f2465df2789631edb2a75442ad` 保留 | 架构与交互参考；记录明确未复制源码，原工作区未覆盖 |
| Hermes Agent | `cb757fac59c92b49e9b8f2910e5cd7ab75efffdd` | README 与外部 CLI 能力参考；当前探测适配不等于引入 Hermes 源码或完成账户执行集成 |
| OpenCode | `02671e66454260d77fe04d63cf5eb94e76921dee` | README 与 CLI 协议参考；未复制源码；真实外部账户未验收 |
| Zed | `e96ffb0fd3cc43ec56dcff421146f5e823b13841` | GPL 相关编辑器参考；仅参考，未复制 Zed 源码。当前编辑器实际使用 Monaco。未对 Zed 整个仓库各子目录许可作统一结论 |
| Spec Kit | `27d14e20c187c1f2916540ea201ecbdc9f8ed588` | 规约流程参考；未将上游完整工具链集成为产品能力 |
| BubbleTea 参考仓库 | `f9df43c4f2c021d8740a2628d59cd810a862eb98` | README/交互参考提交；另有真实 Go 依赖 **v1.3.10**，以 go.mod 为准，二者不是同一个版本声明 |
| DeepSeek Harness | `687f282e8625c6b8d11ff58211a90524be2e4317` | README、外部 CLI 目录参考；未复制源码；未完成真实账户/工具会话验收 |
| OceanBase | `v4.4.2_CE_BP3` / `010580d` | 数据库兼容与原生检索目标基线；通过 mysql 驱动和 mysql2 适配，尚无真实集群验收 |

外部可执行文件、服务端产品、模型账户与其使用条款不因本项目提供适配代码而成为已经验收的随包依赖。未来引入上游代码时，应追加文件级来源、版本、修改与许可记录，不能沿用本表“仅参考”的结论。

## 快照范围与生成方法

本次离线扫描得到 **36 个六平台 Go 运行时链接模块**、**443 个已安装 JS 包版本**。机器清单逐项保存包名、实际版本、直接依赖归属、包 manifest 和可见许可证文件的 SHA-256；输入锁文件与各 workspace manifest 也有哈希。JS 同名不同版本分项；直接依赖按 workspace 实际解析版本匹配，不能把传递的旧版本误标为直接依赖。

Go 范围来自六平台 18 个实际二进制的 `go version -m` 去重信息；见[跨平台链接清单](../deployment/evidence/runtime-linked-modules.json)。Windows 链接 35 个模块，Linux/macOS 额外链接 `github.com/google/uuid v1.6.0`，合计 36；排除标准库、未链接模块和仅测试图中的额外模块。Go 标签来自模块根目录许可证文本特征，属于待人工复核的观察值。JS 范围来自本机 pnpm virtual store 的实际 package.json，包括构建工具、可选平台包；`license` 为包作者声明，不等同于逐文件结论。中央 frozen-lockfile 安装完成后重新采集；采集未新增或修改依赖。

这不是完整的安装包 SBOM。跨平台构建、原生二进制静态链接、Node/Electron 内嵌组件以及未来安装变化，需要从最终产物重新生成清单。绝对路径只记录本次采集位置，不能作为分发时仍存在的文件引用。

## Go 直接依赖

| 模块 | 实际版本 | 本地许可证观察 | 用途 |
| --- | --- | --- | --- |
| github.com/charmbracelet/bubbletea | v1.3.10 | MIT | TUI |
| github.com/charmbracelet/x/ansi | v0.10.1 | MIT | 终端文本处理 |
| github.com/go-sql-driver/mysql | v1.9.3 | MPL-2.0 | OceanBase/MySQL 驱动 |
| github.com/jackc/pgx/v5 | v5.7.4 | MIT | PostgreSQL 驱动 |
| github.com/mattn/go-isatty | v0.0.20 | MIT | 终端识别 |
| modernc.org/sqlite | v1.36.3 | BSD-3-Clause | 本机 SQLite 驱动 |

mysql 的许可证正文含其他许可兼容性说明，不能据此推断依赖采用额外 GNU 许可。modernc 模块的根许可证标签也不能替代其生成代码、上游 SQLite 与所有嵌入组件的文件级复核。

## JS 直接依赖（实际解析版本）

| 包 | 版本 | package.json 许可声明 | 用途 / 范围 |
| --- | --- | --- | --- |
| @lancedb/lancedb | 0.23.0 | Apache-2.0 | 知识服务原生数据库绑定 |
| mysql2 | 3.24.6 | MIT | 知识服务 OceanBase 适配 |
| react / react-dom | 19.0.0 | MIT | Web 运行时 |
| @monaco-editor/react | 4.7.0 | MIT | 编辑器 React 接口 |
| monaco-editor | 0.52.2 | MIT | 实际编辑器与 workers |
| @radix-ui/react-dialog | 1.1.6 | MIT | 对话框基础组件 |
| @xyflow/react | 12.4.4 | MIT | DAG 画布 |
| elkjs | 0.10.0 | EPL-2.0 | DAG 布局 |
| lucide-react | 0.468.0 | ISC | 图标 |
| electron | 35.7.5 | MIT（包声明） | 桌面运行时；另有 Chromium 等组件 notices |
| electron-builder | 26.15.3 | MIT | 桌面打包工具 |
| typescript | 5.7.3 | Apache-2.0 | 构建与类型检查 |
| vite | 6.1.0 | MIT | Web 构建 |
| @vitejs/plugin-react | 4.3.4 | MIT | Web 构建插件 |
| vitest | 3.0.5 | MIT | Web 测试 |
| @types/node | 22.20.5 | MIT | 类型声明 |
| @types/react | 19.0.3 | MIT | 类型声明 |
| @types/react-dom | 19.0.2 | MIT | 类型声明 |

声明用途不表示该包一定进入每个产物。全部 443 个包版本及其文件哈希见机器清单，不能只使用本表替代传递依赖清单。

## Go 传递模块

以下为同次链接图的其余模块；标签仍为文本观察。

| 模块 | 版本 | 本地许可证观察 |
| --- | --- | --- |
| filippo.io/edwards25519 | v1.1.0 | BSD-3-Clause |
| github.com/aymanbagabas/go-osc52/v2 | v2.0.1 | MIT |
| github.com/charmbracelet/colorprofile | v0.2.3-0.20250311203215-f60798e515dc | MIT |
| github.com/charmbracelet/lipgloss | v1.1.0 | MIT |
| github.com/charmbracelet/x/cellbuf | v0.0.13-0.20250311204145-2c3ea96c31dd | MIT |
| github.com/charmbracelet/x/term | v0.2.1 | MIT |
| github.com/dustin/go-humanize | v1.0.1 | MIT |
| github.com/erikgeiser/coninput | v0.0.0-20211004153227-1c3628e74d0f | MIT |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause |
| github.com/jackc/pgpassfile | v1.0.0 | MIT |
| github.com/jackc/pgservicefile | v0.0.0-20240606120523-5a60cdf6a761 | MIT |
| github.com/jackc/puddle/v2 | v2.2.2 | MIT |
| github.com/lucasb-eyer/go-colorful | v1.2.0 | MIT |
| github.com/mattn/go-localereader | v0.0.1 | 模块根目录未找到许可证文件；待核查 |
| github.com/mattn/go-runewidth | v0.0.16 | MIT |
| github.com/muesli/ansi | v0.0.0-20230316100256-276c6243b2f6 | MIT |
| github.com/muesli/cancelreader | v0.2.2 | MIT |
| github.com/muesli/termenv | v0.16.0 | MIT |
| github.com/ncruces/go-strftime | v0.1.9 | MIT |
| github.com/remyoudompheng/bigfft | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause |
| github.com/rivo/uniseg | v0.4.7 | MIT |
| github.com/xo/terminfo | v0.0.0-20220910002029-abceb7e1c41e | MIT |
| golang.org/x/crypto | v0.31.0 | BSD-3-Clause |
| golang.org/x/exp | v0.0.0-20230315142452-642cacee5cc0 | BSD-3-Clause |
| golang.org/x/sync | v0.10.0 | BSD-3-Clause |
| golang.org/x/sys | v0.36.0 | BSD-3-Clause |
| golang.org/x/text | v0.21.0 | BSD-3-Clause |
| modernc.org/libc | v1.61.13 | BSD-3-Clause |
| modernc.org/mathutil | v1.7.1 | BSD-3-Clause |
| modernc.org/memory | v1.8.2 | BSD-3-Clause |

## 正式分发前待办

- 复核 `go-localereader` 文件级授权来源；根目录文件缺失不表示没有许可，也不能自行套用相邻模块许可。
- 针对最终安装包核查 Electron 的 LICENSE、LICENSES.chromium.html、Chromium/FFmpeg、LanceDB 原生 Rust/C++ 组件及工具链附带内容。清单已记录当前可见 notices 哈希，但未完成嵌入组件逐项展开。
- 复核 MPL/EPL 等依赖的具体文件、修改和分发方式，形成适用的 notices、源码提供等交付安排；本页不判定这些义务是否已经满足。
- 按最终平台和产物重新采集，保存锁文件、产物摘要和 notices；Go/Node 标准运行时及跨平台可选依赖也需纳入对应产物审查。
- 任何上游源码复制、补丁或新增外部二进制必须更新来源记录、完整性摘要和许可资料，再由发布负责人确认交付包。

本次证据只支持“依赖和参考来源可追踪到当前快照”，不支持“所有第三方许可已经审核完毕”或“可以直接商业分发”的结论。
