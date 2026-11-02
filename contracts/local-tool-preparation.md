# 通用本机工具配置准备

T-LOCAL-TOOL-PREPARATION-220，速码1.0.0。完整31项／P0–P7和58limited／34pending／92总账保持active。上一阶段b8cce16已验证并正常推送。

## 真实缺口

Go PrepareToolCommands与现有CLI --prepare-tools NEWFILE --local-tool ID已能固定PATH发现、被动版本探测并独占新建配置。普通Runner指南已有此命令；T219共享模式隐藏普通身份指南后，也隐藏了四项通用CLI的自动准备入口，只剩手工JSON。专用SDK／ACP／AppServer准备器继续原显式输入，不猜模型、凭据或包目录。

## 模型与共享规则

新增local-tool-preparation-model.ts：LocalToolPreparationDraft={shell:'powershell'|'bash',commands:string,localTool:四项非special目录ID}；闭形3键。导出 localToolPreparationTarget(value)、validLocalToolPreparationDraft(value)、localToolPreparationCommand(value):{prepare:string}|undefined。Target从原preparationForLocalTool目录过滤special=false，只含codex-jsonl/codebuddy-cli/claude-code-cli/dsh-multica。绝对.json新输出路径沿原公共路径校验，Shell字面量沿runnerSetupLiteral，不复制第二正则或自造flag。

remoteRunnerSetupCommands在四个明确localTool时复用同一prepare builder；原localTool为空的有界全部工具模式保持，special仍走原专用入口。不得修改report/start/discoveryStart的实际身份或权限语义。

## 简洁准备卡与交接

LocalToolPreparationSetup props为localTool、disabled?、onUseConfiguration?(draft)。固定所选工具，只有目标Shell和新输出JSON路径两个输入。明确复制既有CLI命令、明确采用输出路径分别操作；复制/采用不声称文件已存在或已生成。提示实际目标机执行会进行有界被动版本探测并新写本机文件，拒覆盖／链接由原Go writer决定；不安装、登录、调用模型或登记启用。

组件绑定当前caller/user/org/localTool与单调输入revision；账号或工具A→B→A、pending、输入变化均不能采用旧clipboard完成。原命令预览与路径采用按当前draft重新计算。中英文、44px、窄屏与键盘沿原设计系统，不新增步骤页面。

ToolSetupGuide将该卡用于四项generic CLI，原大段手工JSON保留在按需展开的高级示例中；专用五协议、内部规划/测试等模板保持原义。ConfigurationDraft和RemoteRunnerSetupGuide严格Request读取器扩入新3键draft，复用原owner/epoch/navigationNonce/selectionNonce交接。新文件引用必须匹配当前选定协议；没有明确nonce来源时生成新当前选择，再采用，不放宽旧请求重放。

普通指南只填入原shell/commands/localTool字段，共享指南只显示可复制的本机输出引用，不写Pool表单、不携带到URL/API、不授予身份或执行。后续原库存/matches→paused登记→明确enable与质量链不变。

## 并行分工

Runner代理仅拥有新纯模型/测试及remote-runner-setup-model.ts共享prepare接线；不改Go。Web代理仅拥有新LocalToolPreparationSetup.tsx/测试/局部CSS/本域双语。Root拥有ToolSetupGuide.tsx、RemoteRunnerSetupGuide.tsx的严格draft接线和直接交互检查、文档与Git。实际验证代理拥有隔离integration夹具，待三源freeze后一次代表界面构建及必要原CLI实际运行；优先复用source-matching现有binary，否则只编译runner一次，不跑整包Go。

必要验证只覆盖Shell引用和闭形4目录、两字段/拷贝与旧ACK失效、ordinary/shared准确采用和A→B→A、原special与普通prepare直接回归。一次实际原CLI被动探测/新写/拒覆盖分别记录，不把版本探测或文件准备称为模型开发或远程修复。全局类型/i18n一次，修正仅续受影响；代表CN与EN390，避免重复T219身份/权限/30秒旅程。
