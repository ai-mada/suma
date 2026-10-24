# 工具执行工作台：组件、模型与类型验证

状态：T-P4-004 前端组件、模型、类型与格式检查通过；实际浏览器及端到端验收由主集成代理单独记录。日期：2026-10-24。负责人：UI 实现代理。合同：[工具执行与固定任务派发](../../contracts/tool-execution.md)。原始证据：[归档目录](evidence/tool-ui/README.md)、[逐字节映射](evidence/tool-ui/archive-map.json)。

本文只证明明确列出的前端范围。React 服务端渲染检查没有执行浏览器点击、布局、焦点、网络请求或外部工具进程；模型测试中的请求和存储使用受控替身。不能用本报告代替真实派发、模型认证、独立测试、人工验收、问题回流或生产验收。

## 最终前端检查

| 检查 | 实际结果 | 原始证据 |
|---|---|---|
| 完整 web 回归 | `pnpm --filter @suma/web test`，189 项通过、17 个测试文件，退出 0 | [最终原文](evidence/tool-ui/ui-web-lifecycle-link-final.txt) |
| web 类型 | `pnpm --filter @suma/web typecheck`，退出 0 | [最终类型输出](evidence/tool-ui/ui-typecheck-lifecycle-link-final.txt)、[执行收据](evidence/tool-ui/ui-lifecycle-link-verification.json) |
| 新文件格式 | 已有项目缓存内的 Prettier 3.6.2，`--check` 检查 13 个新增 UI 文件，退出 0；没有重写源码 | [版本](evidence/tool-ui/ui-final-format-version.txt)、[结果](evidence/tool-ui/ui-final-format-check.txt)、[格式收据](evidence/tool-ui/ui-final-format-verification.json) |
| 改动空白 | `git -c core.whitespace=blank-at-eol,blank-at-eof,space-before-tab,cr-at-eol diff --check -- apps/web`，退出 0；只覆盖已跟踪 UI 差异 | [原始空输出](evidence/tool-ui/ui-final-whitespace-check.txt)、[格式收据](evidence/tool-ui/ui-final-format-verification.json) |
| 当前源码对应 | 189 项切面的 18 份 UI 文件在归档前再次计算 SHA-256，全部匹配；旧切面单独保留 | [最终源码收据](evidence/tool-ui/ui-lifecycle-link-verification.json)、[归档复核](evidence/tool-ui/archive-verification.json) |

本阶段比此前 web164 基线增加 25 项：`tool-model.test.ts` 14 项、`ToolFacts-consumers.test.tsx` 6 项、`ToolBindingStatus-consumers.test.tsx` 4 项，以及既有 `record-lineage.test.tsx` 新增 1 项。最后一个文件的原有 3 项也继续通过。189 不是平台全部测试数量，也不是 189 条浏览器旅程。

早期类型检查发现两处 TypeScript 边界问题，修复后才继续交付；[原失败](evidence/tool-ui/ui-typecheck-first.txt)和[当时通过结果](evidence/tool-ui/ui-typecheck-green.txt)均保留，不将原失败输出删除或改写为通过。

## 已实现的界面与相应验证

### 执行工具与身份

[工具工作区](../../apps/web/src/ToolWorkspace.tsx)复用现有绑定执行身份，支持非秘密清单校验、暂停登记、显式启用／暂停／撤销。一次性 Runner 凭证仅保留在当次组件内存；回执恢复没有原文时明确提示不能再次读取，不把旧元数据当作已取得凭证。

[清单模型](../../apps/web/src/tool-model.ts)拒绝秘密字段、原始可执行路径、参数、环境值、不匹配协议／指纹和重复命令标识。原因上限是 1000 UTF-8 字节，不是 1000 个字符；不可用观察必须给出去除空白后非空的原因。可用观察要求完整配置指纹、程序指纹、版本及读写声明。模型测试验证这些规则；它们没有读取真实第三方登录文件，也不证明模型账号已认证。

[写入恢复](../../apps/web/src/tool-write.ts)在发送前保存非秘密的原路径、请求、版本及操作标识；网络未知后重新实例化恢复对象仍重放原请求。后续撤权拒绝不会证明原未知操作未生效，恢复记录继续保留。测试使用内存 `Storage` 替身及受控 `fetch`，不能据此宣称浏览器会话存储、密码管理器或真实丢 ACK 已实测。

### 固定任务执行计划

[派发对话框](../../apps/web/src/TaskDispatch.tsx)先准备计划，再显示完整固定任务／需求依据、工具和独立测试范围；需要明确审阅后启动。客户端与服务端合同一致地预检可写工具、所选 agent 的固定候选独立测试，以及已核实的 `suma/suma-jsonl-v1`、`codex/codex-jsonl-v1` 任务协议。通用命令及其他工具仍可用于明确配置的独立工作流，不能冒充已核实任务目标和智能体终态。

现场反馈发现一个实际形态遗漏：`ready` 计划也有预留的非空 `run_id`。原界面按编号是否存在判断已启动，因而隐藏了显式启动入口。新增语义与消费者回归先复现失败，再修复为：

| 持久事实 | 界面表达 |
|---|---|
| `ready` 且有预留编号 | 仅显示预留编号和“尚未创建正式运行”；保留审阅及显式启动，不提供真实运行入口 |
| `started` 且有正式编号 | 提供本计划运行入口，执行结果与验收继续分开 |
| `started` 但编号未读取 | 要求重新读取并核对原操作，不提供虚构运行入口，也不允许创建第二次执行 |

这些规则由 [RED 原文](evidence/tool-ui/ui-reserved-plan-red.txt)和[通过原文](evidence/tool-ui/ui-reserved-plan-and-link-green.txt)证明到组件／模型层。修复后的实际按钮点击和服务端推进需要主集成代理的浏览器旅程证据，本文不代言该结果。

### 当前资格与历史执行

[运行绑定状态](../../apps/web/src/ToolBindingStatus.tsx)直接读取固定 NodeRun 的 `tool_profile_id`、指纹和命令，不依赖可编辑草稿；尚无进度或结果时也可显示固定依据。它每 10 秒读取一个精确 profile，并隔离切换后的迟到响应。当前新派发资格、版本漂移、失联和权限读取失败不会改变本次运行的历史状态。消费者检查覆盖无事件、旧上报、指纹不匹配及读取失败。

[执行事实组件](../../apps/web/src/ToolFacts.tsx)将平台状态、外部报告终态、会话、消息和显式用量字段分开。缺失 Token 字段不补零、不估算费用。只保留当前 attempt／fence／worker 的结构化进度；等待输入不提供不受支持的答复或自动恢复。旧等待消息不能让已结束运行继续声称占用槽位。

工具详情 URL 保留原运行和节点的有界返回上下文；[任务来源组件](../../apps/web/src/RecordLineage.tsx)新增当前项目与精确需求 ID 的详情链接，拒绝跨项目或任意 URL，不依赖已加载的 100 条数据，也不猜需求名称。模型与消费者验证 URL 编码和解析；实际浏览器前进／返回仍由主集成单独验收。

## RED → GREEN 原始记录

| 规则 | 首次失败 | 对应通过 |
|---|---|---|
| 初始清单、恢复、固定依据、执行事实 | [12 项失败](evidence/tool-ui/ui-red.txt) | [12 项通过](evidence/tool-ui/ui-model-green.txt) |
| 终态不能继续声称等待占槽 | [回归失败](evidence/tool-ui/ui-waiting-terminal-red.txt) | [13 项子集通过](evidence/tool-ui/ui-current-green.txt) |
| 任务开发须可写与独立候选测试 | [回归失败](evidence/tool-ui/ui-task-prerequisite-red.txt) | [14 项子集通过](evidence/tool-ui/ui-prerequisite-green.txt) |
| 无事件时显示固定绑定与当前资格、返回路由 | [5 项失败](evidence/tool-ui/ui-binding-status-red.txt) | [14 项子集通过](evidence/tool-ui/ui-binding-status-green.txt) |
| 原因 1000 UTF-8 字节及不可用原因非空 | [2 项失败](evidence/tool-ui/ui-inventory-reason-boundary-red.txt) | [12 项模型通过](evidence/tool-ui/ui-inventory-reason-boundary-green.txt) |
| 任务结构化协议与通用流程边界 | [回归失败](evidence/tool-ui/ui-task-protocol-boundary-red.txt) | [13 项模型通过](evidence/tool-ui/ui-task-protocol-boundary-green.txt) |
| 未加载需求的精确项目路由 | [回归失败](evidence/tool-ui/ui-requirement-link-red.txt) | [4 项来源测试通过](evidence/tool-ui/ui-requirement-link-green.txt) |
| `ready + reserved run_id` 与真实 `started` | [2 项失败](evidence/tool-ui/ui-reserved-plan-red.txt) | [24 项联合子集通过](evidence/tool-ui/ui-reserved-plan-and-link-green.txt) |

各行通过数是当时所执行子集的测试数量，不可相加为独立场景或最终总数。

## 源码切面与归档语义

四份源码收据均按原始字节保存，内部旧 `.cache` 路径通过 `archive-map.json` 映射到公开归档。不能将旧收据重新解释为当前全部文件的摘要。

| 切面 | 测试与范围 | 原件 |
|---|---|---|
| 初轮工具 UI | web183，16 份源码；后续修复前的历史切面 | [源码收据](evidence/tool-ui/ui-source-verification.json)、[web](evidence/tool-ui/ui-web-final.txt)、[类型](evidence/tool-ui/ui-typecheck-final.txt) |
| 原因边界同步 | web185，16 份源码；历史切面 | [源码收据](evidence/tool-ui/ui-reason-boundary-verification.json)、[web](evidence/tool-ui/ui-web-reason-boundary-final.txt)、[类型](evidence/tool-ui/ui-typecheck-reason-boundary-final.txt) |
| 任务协议同步 | web186，16 份源码；历史切面 | [源码收据](evidence/tool-ui/ui-task-protocol-verification.json)、[web](evidence/tool-ui/ui-web-task-protocol-final.txt)、[类型](evidence/tool-ui/ui-typecheck-task-protocol-final.txt) |
| 现场生命周期与需求链接修复 | web189，18 份源码；归档时与当前源码全部匹配 | [源码收据](evidence/tool-ui/ui-lifecycle-link-verification.json)、[web](evidence/tool-ui/ui-web-lifecycle-link-final.txt)、[类型](evidence/tool-ui/ui-typecheck-lifecycle-link-final.txt) |

旧收据的 `final_review_notes` 中保留了先前 178 项的过程文字；没有修改该历史字段。各切面的通过数以对应 `web_tests.passed` 和表中原始测试日志为准，不能把过程备注当作最终结果。三个旧切面各有 6 个文件与当前不同，属于后续明确修复；最终 189 切面的 18 个文件没有差异。

归档仅包含允许清单中的 UI 测试、类型、格式输出及源码摘要；没有复制实际 Runner 凭证、签名密钥、数据库、控制服务日志、浏览器会话或实际执行目录。原件逐字节摘要、常见秘密模式检查与文件清单见归档映射。测试中的秘密字符串是显式单元测试夹具，不是实际登录凭证。Git 属性对证据目录关闭换行转换。

## 尚未由本报告验证

实际浏览器旅程、390／768／1366／1920 视觉、深浅主题、Escape 与回焦点、未知提交的真实恢复、跨项目切换、需求返回，以及真实工具进程、候选、独立测试、人工验收和问题回流均需主集成代理单独保存证据。本文没有启动模型、执行 UI 浏览器旅程、重建生产网页、生成安装包或执行 commit，也不证明厂商认证、分布式负载或完整 P0–P7 平台已完成。
