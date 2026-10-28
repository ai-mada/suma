# 1.0.0 当前交付状态

更新：2026-10-28。产品统一开发基线为 **1.0.0**；完整 P0–P7 与扩展需求 1–22 继续实施，生产集成尚未完成。[基线与范围](../product/v1-baseline.md)确定产品合同，[实施计划](implementation-plan.md)保留完整待办，[文档索引](../README.md)集中各领域设计、合同和证据。

当前能力按实现范围汇总。下表中的验证均受对应报告的源码、环境和实际场景限制；不会因本次版本统一继承为新代码全部通过。执行成功、独立测试、人工验收与生产发布是不同事实。

## 当前实现与证据

| 领域 | 当前实现 | 验证入口与明确边界 |
| --- | --- | --- |
| 身份、组织与治理 | 初始化、登录、会话、组织／项目隔离、真实账号、团队、邀请、角色、撤权及当前凭证授权 | [当前权限](../quality/current-authority-verification.md)、[平台验证](../quality/verification.md)；完整自定义角色／菜单、企业 SSO、OAuth／设备治理及跨数据库验收待办 |
| 项目计划与进度 | 迭代、里程碑、版本化排期、完整项目不可变基线、当前比较、自然周估算分摊与真实业务指标 | [产品与治理](../product/governance.md)、[验证](../quality/verification.md)；计划估算不等于实际工时，人员日历、连续历史、预测和 portfolio 待办 |
| 需求与设计 | 版本化产品／体验／技术设计、可审阅任务提案、明确发布及原子采用、固定执行与验收依据 | [设计采用](../quality/requirement-design-verification.md)、[依据展示](../quality/task-design-context-verification.md)；完整 AI 需求与设计编辑、可运行 UED 原型待办 |
| 知识、记忆、规范与技能 | 项目资产版本、审阅发布、固定任务上下文、组织共享目录、授权检索、跨项目独立草稿采用、模型辅助提案及发布溯源 | [项目资产](../quality/project-knowledge-verification.md)、[共享技能](../quality/skill-catalog-verification.md)、[模型提炼](../quality/knowledge-intelligence-verification.md)；授权向量索引、外部记忆、可执行插件与完整专家策略待办 |
| 智能规划与失败分诊 | 固定来源的单次模型规划、人工修订／采用、持久站内风险；真实失败证据形成不可变观察与人工修复需求 | [规划](../quality/intelligent-planning-verification.md)、[分诊](../quality/failure-diagnosis-verification.md)；本地自有协议证据，非厂商效果或长期自主恢复验收 |
| DAG 与共享执行池 | 条件／并行／汇合／map／子流程／人审／等待／补偿、增量前沿、attempt／fence／回执、双边授权及分层加权准入 | [增量调度](../quality/scheduling-verification.md)、[平台门禁](../quality/scheduling-platform-guards.md)、[共享池](../quality/execution-pool-verification.md)；同机多进程不等于物理分布式、完整公平／吞吐或长期容量验收 |
| 工具派发与应用工具 | 固定工具版本、真实库存、当前资格与路由推荐、明确采用后预览／启动；官方 MCP SDK 项目读取和受控草稿 | [工具执行](../quality/tool-execution-verification.md)、[工具推荐](../quality/task-dispatch-options-verification.md)、[项目 MCP](../quality/project-mcp-verification.md)、[只读诊断](../quality/developer-connection036-verification.md)；探测、复制配置和单次诊断不等于完整客户端接通 |
| 原生与外部 CLI 执行 | 原生受限工具、独立 Git 工作树、固定开发检查、机器封存及另一 Runner 同 SHA 正式测试；Harness／DSH、CodeBuddy 的限定协议与 Node 入口 | [原生编码](../quality/native-coding-verification.md)、[团队 CLI](../quality/team-cli-verification.md)；本地协议夹具与真实厂商任务分开，其他工具、会话恢复及 OS 隔离待办 |
| OpenCode 当前固定 SDK 执行 | 官方SDK／server1.19.4、固定普通／专家配置；当前轮MCP连接、真实六工具宿主、操作账本／封存、Go合成停止证明与observe-only门禁；简洁运行卡及测试下钻 | [设计与SOP](../architecture/opencode-execution.md)、[真实限定验证](../quality/opencode-actual-execution-verification.md)：官方Windows CLI／SDK配自有确定性模型协议、两Worker、候选交接与精确SHA独立测试；商业模型效果／账单、物理多机、Unix运行、会话恢复及生产待办 |
| Codex 独立模型执行 | 真实目录选择模型／推理强度、固定单任务计划、安装版 app-server 私有工作树编码、持久原进度／结果与部分用量、明确取消门禁 | [Codex 限定验证](../quality/codex-app-server-verification.md)；自有 Responses 端点实际编码观测 88 Token、费用未定价，配置 ACK 不证明提供方模型身份；observe-only 提醒不等于硬预算；专家逐项选择与修订已有[跨入口证据](../quality/model-selection-workflows-verification.md)；Codex活动工作区已接通[只读broker](../quality/codex-workspace-observation-verification.md)；协调保存、冲突修复及完整远程IDE待办 |
| 模型、预算与用量 | 原生模型固定选择、封闭执行参数、attempt 额度、冻结运营方费率、私有／Pool 原子预留与去重结算；专家逐任务原生／Codex选择与明确修订、原生冲突修复选择／修订 | [模型执行](../quality/model-execution-verification.md)、[团队模型](../quality/expert-team-model-execution-verification.md)、[跨入口配置与恢复](../quality/model-selection-workflows-verification.md)、[账本](../quality/usage-accounting-verification.md)；每工具当前匹配费率不等于完整报价目录，缺失／partial usage 不记零，厂商账单及全主体额度待办 |
| 专家团队与任务交接 | 版本化专家职责、固定工具／流程／知识、原子多任务派发、验收依赖推进、暂停／恢复／停止、未开始项修订及明确人员交接 | [团队派发](../quality/expert-team-dispatch-verification.md)、[依赖编排](../quality/expert-team-orchestration-verification.md)、[任务交接](../quality/task-handoff-verification.md)；运行完成不自动验收，完整自主专家协同与跨组织远端待办 |
| 候选、修复与仓库集成 | 机器封存 bundle、跨 Runner 导入、固定多源 Git 合成、精确结果独立测试、明确下游基线；冲突独立修复任务、受控重试／修订；固定提供方审批后 CAS 快进 | [候选交接](../quality/candidate-transfer-verification.md)、[多源合成](../quality/candidate-composition-verification.md)、[冲突修复](../quality/candidate-conflict-repair-verification.md)、[修订](../quality/candidate-repair-revisions-verification.md)、[仓库集成](../quality/repository-integration-verification.md)；本地裸仓库／同机 Runner 证据，完整合并策略、动态来源继承、物理远端与未知效果恢复待办 |
| 工作区与 IDE 基础 | 本地 Monaco 文本树、草稿／CAS 保存、只读 Git index／worktree 差异；原生／Codex活动工作区出站 list／read；原生一次保存许可、原回执及显式 inspect、手工差异合并 | [Git](../quality/workspace-git-verification.md)、[执行读取](../quality/workspace-observation-verification.md)、[执行保存](../quality/workspace-write-verification.md)、[传输能力](../quality/workspace-capability-verification.md)、[Codex读取](../quality/codex-workspace-observation-verification.md)；Codex只读，执行保存仅已有 UTF-8 文本且新旧均 ≤48KiB，创建／删除、大文件、Git 写、PTY／LSP／DAP、完整 IDE 待办 |
| 自动化与事件 | 持久人工／cron 触发、时区／错过／重叠策略、固定来源、签名 GitHub／Suma 事件、去重交付、当前授权 SSE 与断线续传 | [自动化](../quality/automation-verification.md)、[事件入口](../quality/ingress-verification.md)；本机真实 Git／协议事件不替代公网厂商、生产 CI 身份或消息通道验收 |
| 质量、制品与发布 | 独立测试证据、人工验收、失败去重回流；真实字节上传封存、包测试、固定范围、本机静态站点批准／发布／核对／独立回退 | [验收矩阵](../quality/acceptance.md)、[发布控制](../deployment/release-control.md)、[发布证据](../architecture/release-evidence.md)；生产 CI、签名、远端／容器部署、灰度与生产健康待办 |
| 国际化与交互 | 成对中文／英文目录、持久账户／组织偏好、结构化诊断、原文保留及代表性键盘／刷新旅程 | [语言偏好](../quality/locale-preferences-verification.md)、[自有前端](../quality/remaining-localization-verification.md)、[后端诊断](../quality/backend-diagnostics-verification.md)、[运行诊断](../quality/native-diagnostics-verification.md)、[显示资格](../quality/platform-display-verification.md)；目录键数不代表完整平台、库内控件、通知、安装器及所有语言表面完成 |
| 存储与客户端交付 | SQLite、本地文件／Lance 与 OceanBase／PostgreSQL 驱动路径；CLI／TUI、Web 构建、Windows 未签名桌面与六目标四程序交叉编译 | [数据设计](../data/transactional-store.md)、[构建](../deployment/build-reproducibility.md)、[运行包](../deployment/runtime-bundles.md)、[桌面](../deployment/desktop.md)；真实分布式数据库、S3／多模态全链路、安装／签名／目标系统运行、离线协作、移动 App 与 SaaS 商业化待办 |

## 1.0.0 初始化与验证边界

应用统一为单一 1.0.0 新库初始化，不保留历史库升级、旧字段别名或旧协议适配分支。现有 `.suma/`、原凭据、业务原件与专项验收档案保留；新代码不自动迁移旧库，不以本次版本统一删除或重写它们。产品版本、协议版本、业务修订和数据版本分别管理。

既有主预览曾以保留 188 表与原会话的前向交付取得证据，准确过程留在 [Codex 专项报告](../quality/codex-app-server-verification.md)。该事实不作为本次新库初始化变更的迁移承诺，也不表示当前代码已经重新部署。当前变更的检查结果以 [验证入口](../quality/verification.md)为准。

OceanBase／PostgreSQL 的驱动与 DDL 不等于实集群验收，本地检索不等于完整 OCR／转录，交叉编译不等于安装或运行，单元测试不等于目标执行槽位容量已通过。厂商模型、正式业务负责人签收与生产验收均需独立证据。

## 后续交付与使用

优先补齐智能选择与模型授权／报价、完整 IDE、知识／技能同步、真正多机与厂商任务闭环；身份、存储、协作、渠道、移动端、支付、安装和规模仍按完整台账实施，不以局部适配缩减范围。

浏览器默认 `127.0.0.1:4310`。执行需另建项目范围 Runner 凭证并配置许可命令；文件编辑需明确服务端允许根目录和项目工作区绑定。首次初始化与部署配置见 [服务器说明](../deployment/server.md)。

验证数据使用独立 `.cache/e2e/`，保留用户正式运行目录。正式产物进入 `dist/`，专业文档进入 `docs/`；秘密、数据库、原始日志、截图、依赖缓存和本地二进制不提交。历史阶段过程由 Git 和专项报告追溯，本页只维护当前能力。

## 初始化结构

当前源码以单一1.0.0新库结构建立，旧队列／资源镜像、旧内核分代与凭证／设计索引回填已删除；当前183表正常重启保持。详见[初始化设计](../architecture/initial-version.md)和[限定验证](../quality/initial-version-verification.md)。本次未改写既有用户数据库。
