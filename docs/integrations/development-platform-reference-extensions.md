# 开发平台新增参考与实现差距

状态：参考基线已核对，尚未完成第三方接入或生产验收。负责人：主集成代理。关联需求：扩展第21项（智能动态选择工具、专家、技能及模型参数／速率／成本）与第22项（完整专业IDE）；速码版本保持 **1.0.0**，完整P0–P7范围继续实施。

## 固定参考基线

2026-10-27通过公开远端引用查询固定下列 `main` 提交，再按完整提交读取README、版本和许可文件；没有克隆仓库、运行参考源码或调用厂商模型。

| 项目 | 本次固定提交与源码版本 | 读取边界 |
| --- | --- | --- |
| OpenWorkBuddy | [`2e76920dabc3ac057283148ccaa5f8dbb0489530`](https://github.com/CatCatUncle/openworkbuddy/tree/2e76920dabc3ac057283148ccaa5f8dbb0489530)；[package.json](https://github.com/CatCatUncle/openworkbuddy/blob/2e76920dabc3ac057283148ccaa5f8dbb0489530/package.json)为`0.17.5` | 本次官方发布页显示[v0.16.6](https://github.com/CatCatUncle/openworkbuddy/releases/tag/v0.16.6)，与主分支源码版本分开记录；没有下载或验收安装资产。 |
| Octop | [`890111b7879665840414ca28ab168b05b0db0921`](https://github.com/TencentCloud/Octop/tree/890111b7879665840414ca28ab168b05b0db0921)；[pyproject.toml](https://github.com/TencentCloud/Octop/blob/890111b7879665840414ca28ab168b05b0db0921/pyproject.toml)为`1.0.3b1` | 同名[发布版本](https://github.com/TencentCloud/Octop/releases/tag/v1.0.3b1)；Beta团队能力仍按其官方标记记录。 |

原始8份文件的缓存证据位于`.cache/e2e/candidate-repair-revisions-stage-033/references/official-source-manifest.local.json`，采集时间`2026-10-26T23:55:39.169019Z`，清单SHA256为`e16d453317c9191328e30ae70f5b58846dd10207f2a9a1a70f090267c7130846`。GitHub连接器传输不可用后，使用公开引用查询与官方原始文件HTTPS读取；原件和缓存不进入Git。

## 可参考能力

OpenWorkBuddy的固定[README](https://github.com/CatCatUncle/openworkbuddy/blob/2e76920dabc3ac057283148ccaa5f8dbb0489530/README.en.md)声明：本地办公文件交付与预览、桌面和CLI共享配置／技能／记忆／会话、调用本机Codex及Claude Code、配置模型、多用户自托管与配额。重点借鉴紧凑进度、按需查看用量和可打开文件的交付判据。其[许可](https://github.com/CatCatUncle/openworkbuddy/blob/2e76920dabc3ac057283148ccaa5f8dbb0489530/LICENSE)为PolyForm Noncommercial 1.0.0；本轮仅研究交互与合同，没有复制源码或将其打包进速码。

Octop的固定[README](https://github.com/TencentCloud/Octop/blob/890111b7879665840414ca28ab168b05b0db0921/README.md)声明：多用户专家、AgentTeams Beta、OAuth／MCP连接器、记忆与知识库、消息渠道以及双向ACP；入站面向IDE，出站包括OpenCode、CodeBuddy、Claude Code及Codex。它以单进程处理多入口，控制面数据库与专家文件后端分别配置；不能把单进程自托管等同于速码所需的分布式调度。其[许可](https://github.com/TencentCloud/Octop/blob/890111b7879665840414ca28ab168b05b0db0921/LICENSE)为MIT。

上述是官方资料核对结果，不是速码已经具备这些能力的证据，也不是对参考项目运行质量的独立测试结论。

## 对应实现、缺口与下一步

| 能力 | 速码当前对应实现 | 尚缺能力与下一步 |
| --- | --- | --- |
| CLI／Agent统一运行 | [工具档案](../../internal/platform/tool_profiles.go)、[Runner执行](../../internal/runner/execute.go)已有固定版本、机器库存和执行回执；既有范围见[团队CLI接入](team-cli-runtime.md)。 | 为新增平台逐项验证官方CLI／ACP合同，覆盖等待输入、取消、恢复和断线核对。不能把可配置命令当作已完成第三方适配。 |
| 动态选择专家与工具 | [智能规划](../../internal/platform/intelligent_planning.go)、[团队推进器](../../internal/platform/expert_team_orchestration_tick.go)承接受控计划和依赖推进。 | 第21项仍需统一候选能力目录，按当前权限、任务类型、能力、可用容量及策略生成可解释选择；固定所选版本与依据后再执行，变化时形成修订。 |
| 模型参数、速率与成本 | [计量模型](../../internal/platform/usage_accounting_model.go)与账本支持部分模型价格合同、预算预留和实际用量来源。 | 补齐模型能力／上下文／参数边界、速率、质量及成本联合策略。原生写入当前未定价、未报告用量仍保留未知；不得宣称已有最优模型自动路由或完整厂商账单。 |
| 统一技能、知识和记忆 | [技能目录](../../internal/platform/skill_catalog.go)、[技能工具](../../internal/platform/application_skill_tools.go)、[项目知识](../../internal/platform/project_knowledge_service.go)提供授权、固定版本和受控使用基础。 | 完善向量检索、记忆同步、版本化萃取与评估；根据任务选择技能并校验当前授权，不能直接动态注入未审查插件或把目录等同于完整知识引擎。 |
| 完整专业IDE | [IDE界面](../../apps/web/src/IDE.tsx)使用本地Monaco及语言Worker；[工作区接口](../../internal/platform/workspace.go)提供文件读取、保存版本冲突与恢复基础。 | 第22项仍需完整终端PTY、LSP／DAP、调试、Git差异审查、预览和协作编辑，并统一工作树及运行上下文；文件编辑器与调用外部IDE都不代表完整IDE交付。 |
| 多入口团队与隔离 | [团队当前授权](../../internal/platform/expert_team_orchestration_authority.go)复用项目、实际凭据和运行权限边界。 | 对接桌面、CLI、IDE及消息渠道时复用同一身份、组织／项目隔离和事件合同，逐项验证恢复、配额与操作门禁；第三方平台配置不能授予额外权限。 |

开发顺序保持纵向闭环：先补可解释的能力选择与固定执行合同，再扩展真实厂商适配、完整IDE和多入口协作；分别记录源码完成、协议验证、真实运行及生产验收。参考研究不替代本轮多分支合成、独立测试、基线交接和异常恢复交付。

*English scope: The two official repositories are pinned references. Suma integration, intelligent capability selection and the complete IDE remain implementation work; documentation review does not establish feature or production acceptance.*
