# 执行诊断与命令入口国际化

状态：T-P4-010历史切面，本地验证完成；负责人：平台、DAG、Runtime。关联需求：FND-I18N-001。实现范围不等于全部平台语言表面完成。

后续T-P4-011已将46条原生目录统一生成，并通过版本化诊断合同贯通Runner／Journal／当前授权HTTP／DAG／界面；命令编辑与滚动事实身份也已修正。当前设计与SOP见[原生诊断链路](../agents/native-diagnostics.md)，证据见[后续阶段验收](../quality/native-diagnostics-verification.md)。本页保留T-P4-010当时的实现边界，不把后续结果倒填进历史切面。

## 错误身份与显示分层

`internal/localization.AnnotateError` 在可信生产者附加稳定键及参数快照。其 `Error()` 保存原始完整字符串，`Unwrap()` 保持原始 cause；`errors.Is/As` 和调度状态判断不因语言变化。DAG 的裸 sentinel 保留直接身份，不能为翻译改成全局包装对象。HTTP 只在原本公开的 invalid/conflict/stale/unsupported 分类中作显示投影；NotFound 仍使用现有安全公开文本，取消及内部错误仍遵守原有可见性。

`Definition.SourceTemplate` 是可选的完整规范原文模板。没有该字段时使用中文目录模板；DAG 的原始英文带 sentinel 前缀，独立于更易读的英文与中文显示。规范模板与两份译文共享 `named-dot-v1` 纯文本插值语法，禁止函数和执行逻辑。规范英文不注册为全局原文别名，避免将其他模块的 `invalid` 等通用字串误认成 DAG 错误。

`FormatError(locale, err)` 逐项核验：存在可信注解、外层完整错误仍等于捕获原文、cause 未变化、键存在、参数精确满足类型及字节约束、规范模板能逐字节还原原文。失败返回未映射，由原边界保留已有公开错误；不回显额外参数。带新上下文的包装、拼接的外部错误、未知键和超长参数不能丢失原始诊断。语言始终是显式调用参数。

151 个 DAG 目录键对应150处显式注解及6个裸 sentinel 显示键；多个相同语义工厂共用键。3处工具合同细节、8处SQL／迁移包装和51处裸返回保持原文或身份，不宣称都已翻译。动态 `%q` 使用 `strconv.Quote` 保持完整转义；可能为负的版本以十进制文本传递，不放入只接受非负安全整数的参数类型。

## 流程校验的两份数据

`POST /workflows/validate` 仍返回 HTTP200，不发布流程、不创建运行。`WorkflowValidation` 增加 `diagnostics: Rendered[]`；`errors: string[]` 保存原始编译原因，按相同位置对应。有效流程两数组均为空，不能以缺少错误文案推断成功。

网页保存完整结构化诊断，在渲染时使用已加载的公开双语目录解释。切换语言不重新请求、不重建节点或JSON草稿。未知键、非法参数、未本地化、位置数量不一致和旧响应回退对应原始错误。公告使用原面板的状态区域，不移动编辑焦点，不扩大发布或运行权限。

## 命令入口

控制面、Runner、Deployer 复用标准库 `flag` 与 `internal/commandlocale`，支持中英帮助、参数说明、自有启动诊断及日志标题。选择顺序为 `--language`、`SUMA_LANGUAGE`、进程 `LC_ALL/LC_MESSAGES/LANG`、默认简体中文。`--help`、`--version` 与非法显式语言在目录、数据库、配置文件、凭证、端口或执行工具访问前返回。程序标识、参数名、退出码、机器JSON及外部操作系统错误保持原有语义。解析失败不回显可能带凭证的完整参数值。

日志保留稳定事件标识，标题随显示语言变化；工具原始日志不改写。版本探测输出稳定程序名和构建版本，默认开发构建为 `dev`。

## 原生智能体

`packages/agent-runtime` 使用 Node `Intl.Locale` 和46项受控双语目录，错误事件新增可选 `display`，包含 `message_key`、受控 `message_params`、`locale`、`localized_message`。原始 `message`、缺失字段、`code`、请求标识、用量、模型HTTP分类及输出完整保留。显示选择不能进入模型system/user正文、工具参数、权限或幂等输入。

每次运行捕获独立语言；显式非法SDK语言也必须在模型、工具、目录文件及进程探测之前拒绝。只读CLI帮助、版本和非法参数不请求模型；JSONL执行模式只输出机器事件。适配器的原始 `reason` 与可用性事实保留，显示字段不表示已执行或已验证。

T-P4-010切面的Go Runner工具流解析不会转发这个新增原生`display`，当时仅证明原生SDK／CLI显示能力。后续T-P4-011以独立的`ToolDiagnostic`合同、净化及实际事件重放完成受控链路；它仍不将上游语言或译文作为正式证据，具体见本页开头的后续设计与验收链接。

## 验证与剩余工作

实现和证据见[专项验收](../quality/runtime-localization-verification.md)；公开合同见[语言合同](../../contracts/localization.md)。其他包／状态原因、库内NLS、安装器、通知、导出、渠道与完整双语文档继续逐项迁移。外部原件、用户内容和不可变执行证据保留原语言。
