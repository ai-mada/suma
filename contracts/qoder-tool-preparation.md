# Qoder原生ACP配置准备

T-QODER-TOOL-PREPARATION-221，速码1.0.0。完整31项／P0–P7及58limited／34pending／92总账继续active。前序b76bcfb已验证并正常推送。官方参考：https://docs.qoder.com/cli/acp（2026-11-03读取），原标准启动qoder --acp与登录状态复用，未引入bypass/yolo参数。此参考不是供应商执行验收。

## 一个原生准备与执行合同

现Qoder已有acp-stdio-v1执行器、固定--acp、审核版本与Model、可选凭据环境变量名称，以及真实发现/库存；界面只提供手工JSON，无本机新文件准备或输出采用。原 --prepare-acp-stdin 名义通用但仅接Cursor，不能把Qoder套作Node包。

继续使用 --prepare-tools NEWFILE --prepare-acp-stdin，严格私有stdin五键：adapter_id、executable、reviewed_version、model、credential_env_ref，均显式提供且为字符串。现Cursor分支原五项非空、固定Node/包闭包、认证和manifest要求全部保持；新增adapter_id=qoder-acp分支，credential_env_ref允许空字符串，其余沿原ACP spec的长度/字符/路径/模型规则，不猜版本和模型，不接凭据值。未知/重复/null/多余键/尾随/超限拒绝。

Qoder复用原acpCommandForAdapter、qoderNativeTarget、validateACPExecutionSpec、原安装/文件pin复核和writePreparedToolCommands。只静态核对真实原生文件及父路径、摘要，固定args=[--acp]，不运行Qoder/version/initialize，不读取认证、登录或模型。使用local-qoder-acp命令标识和原ToolPreparation公开结果，O_EXCL全新文件、链接/覆盖拒绝、上下文取消与输入/二次pin失败不产生成功结果。真实版本/能力仍由后续原发现与库存/执行验证，reviewed_version仅为管理员声明。

## Web与三字段输出采用

新qoder-setup-guide-model.ts：QoderSetupInput={shell:''|'powershell'|'bash',executable:string,reviewedVersion:string,model:string,credentialEnvRef:string,commands:string}。导出qoderSetupCommands(value)→{request:上述私有五键,prepare:string}|undefined、validQoderSetupDraft(value)、QoderSetupDraft={shell:'powershell'|'bash',commands:string,localTool:'qoder-acp'}。沿runnerSetupLiteral/现有路径与凭据名称公共规则，Go仍最终校验文件，不能在浏览器伪称已存在/版本已观测。

QoderSetupGuide props disabled?、onUseConfiguration?(draft)。模型ID与可选凭据名称可见，机器文件/Shell/审核版本/新输出路径为清晰分组，避免过多折叠或步骤；保留原手工JSON为高级示例。明确复制原private-stdin命令，明确采用路径分别操作；二者不证明文件生成或工具可用。来源绑定caller/user/org和输入代次、pending/工具/账户ABA/卸载不采用旧clipboard回应，所有状态中英成对和44px/窄屏/键盘。

Root ToolSetupGuide/RemoteRunnerSetupGuide仅扩严格QoderSetupDraft+准确validator，qoder分支传disabled和onUseConfiguration。原nonce/caller/epoch/selection绑定，普通只采用shell/commands/localTool，共享只保本机输出引用，不自动填写Pool、登记、启用、登录或选择模型。

## 分工与必要验证

Runner代理 owns ACP解码/明确Qoder分支/静态准备helper及定向tests、CLI help/error最小双语说明；不改原Qoder执行器。Web代理 owns新纯模型、QoderSetupGuide+tests/局部CSS/本域CNEN；不改Root两指南。Root owns两指南严格draft接线+定向集成、合同/规范/质量/状态/Git。实际代理 owns隔离端到端夹具，待freeze后仅一次Web和必要Runner构建、代表CN/EN390普通/共享采用、原生产表单命令→CLI新写/拒覆盖；本机若无真实Qoder则使用明确原生文件fixture验证静态准备而不冒充厂商运行。

测试只覆盖闭形五键、两adapter不同规则、native文件/链接/二次pin/取消/拒覆盖、Shell/stdin安全、字段与来源ABA及原Cursor直接回归。无需重跑完整Go/SDK/vendor/模型/旧Pool30秒旅程。先释放可再生旧编译缓存至足够容量再编译，保留源码/DB/图片/证据，不接触C盘用户文件。正式Qoder初始化、账户/模型与代码交付/测试验收另有实际证据后才宣称可用；完整平台保持进行中。
