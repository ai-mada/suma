# Codex原生项目技能合同

T-CODEX-SKILLS-173；suma 1.0.0；2026-10-31；负责人：主集成／知识领域／Runner／工作台。按原P2、P3、P4及技能、记忆和多团队开发要求实施；完整31／P0–P7范围保持。当前状态：限定实现与证据已交付，详见[验证记录](../docs/quality/codex-native-skills-verification.md)。

## 正式来源与唯一派发

复用单任务和专家团队现有的明确知识选择。只有当前授权的已发布`kind=skill`修订形成原生技能包；memory/spec继续使用原固定文本上下文，不擅自翻译、升级、安装脚本或插件。Codex app-server适配首先接入，其他工具保持各自已有文本方式，不冒称原生装载。

新`codexcontract.ExecutionSkillDocument`固定`id/revision/sha256/title/name/description/content/content_sha256/document_sha256`；原发布修订SHA不改称文件SHA。稳定名称为`suma-`加知识ID摘要前24个小写十六进制字符，不使用用户提供的路径。description取原summary，空时取title。`RenderExecutionSkill`生成UTF-8文档：YAML前言name、description使用JSON双引号标量，其后原content。正文和文件字节分别摘要。

`ExecutionSkillBundle {protocol,documents,bundle_sha256}`协议为`suma-codex-skills-v1`；最多16项，原选择顺序、ID和名称不重复，拒绝未知字段、无效UTF-8、空内容、非法修订／摘要、超限和摘要不符。bundle摘要按Go规范JSON（自身SHA置空）计算。由原固定TaskContext派生，预览和Start复核当前发布与来源；已开始运行保持原固定修订，不在领取时升级到最新版本。

`TaskDispatchPlan.execution_skills`与受保护开发节点`config.execution_skills`均为同一服务端派生包；构建先清除调用者自带字段。启动沿原计划验证；普通／Pool领取只从原持久计划解析到`Assignment.execution_skills`，与该节点固定配置精确相符，继续原机器／Profile／scope／lease／fence门禁。没有原计划的任意workflow字段不得成为文件授权。原任务、知识、流程总输入上限继续32000 UTF-8字节，不能借文件绕过；Codex模型文本使用原技能身份摘要和原生引用，避免重复整份正文。其他上下文及完整计划快照保留。

## 物化与官方协议

Runner只在本attempt已有隔离`codex-home/skills/<name>/SKILL.md`创建文件，不写工作树、用户HOME、仓库AGENTS或全局技能目录。逐层拒绝符号链接／非目录，拒绝非普通文件与错误已有字节，不覆盖不匹配内容；同attempt准确字节可复核复用。当前包和文件启动前与正常收尾均重核摘要；不把文件权限位称为OS不可篡改。未知执行仍沿原Journal核对，不重发Prompt。

保持原initialize、model/list和thread配置ACK门禁。选有技能时新增`skills/list`，指定当前工作区、forceReload及本次隔离skills作为perCwdExtraUserRoots；核对准确cwd、每项name/path、enabled及无读取错误后，`turn/start`在原文本外增加明确`{type:skill,name,path}`，文本包含对应`$name`。不支持、遗漏、重复、路径变化或读取错误应在发送Prompt前失败，不静默退回文字后宣称原生成功。无技能保持原协议流程。第三方自带／仓库发现项不是平台批准的技能，不自动选入或提升其权限。

## 凭证与界面

`CodexExecutionEvidence.skills`采用`ExecutionSkillEvidence {bundle_sha256,documents,materialized,catalog_acknowledged,invocation_requested,turn_acknowledged,files_verified,failure_code?}`。documents使用不含正文和本机路径的`ExecutionSkillReceipt {id,revision,sha256,name,document_sha256}`。五类事实独立，目录成功或回合ACK均不证明模型实际读取、执行或质量改善；本阶段没有伪造read/used标志。证据沿原ToolEvidence／结果／outbox及attempt历史持久化，平台核对固定包与凭证，不接受其它任务的技能装载回执。

界面复用当前技能选择、计划详情和运行证据区，不增加配置向导。预览说明本次原生方式、准确发布版本及数量；运行中简洁显示物化／目录／调用／回合／文件复核，展开后查看精确ID与摘要。无技能、其他工具文字方式、缺失证据和失败分别表达；中英文、键盘和390px均可操作。不能仅凭当前Profile或包存在标为已装载。

## 必要验证与边界

定向验证渲染／摘要、路径与链接拒绝、篡改、原计划和Config／Assignment对应、知识漂移的Start拒绝、既有固定运行、普通／Pool授权、目录与路径ACK／拒绝、原单次Prompt及中英文证据展示。未变的独立测试／人工验收／预算链复用证据；只做本次代表性交互，不重复旧全量套件。实际已安装Codex的无模型目录检查与受控协议测试分别记录，不据此宣称真实模型使用。

官方依据：[app-server技能接口](https://learn.chatgpt.com/docs/app-server)、[本地技能结构](https://learn.chatgpt.com/docs/build-skills)。HelixHarness固定`44ffc536f73d8af525b4795ad10384c516898576`及Multica固定`e8ae72c9dfc1b4474b61a9087f51b33cbee6a048`的流程思想继续复用，无新增上游代码复制。其他工具原生格式、带支持文件的技能包、签名安装、完整Codex审批／持续会话、跨设备同步、物理多机和生产验收继续实施。
