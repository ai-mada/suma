# Claude SDK 自动发现与桌面快捷连接

T-CLAUDE-SDK-CONNECT-201；suma 1.0.0。既有[T192执行合同](claude-sdk.md)与[T177快接模式](opencode-quick-connect.md)继续作为唯一执行和生命周期来源。完整31项／P0–P7／58限定／34待交付／92基线继续。

## 接入流程与授权

本轮补齐已实现Claude SDK执行到本机快捷连接的缺口：被动发现原生CLI → 明确固定模型与凭据环境引用 → 宿主核对受信SDK资产 → 原托管节点连接及真实新库存 → 原暂停配置登记 → 独立明确启用。不新增HTTP业务接口、数据库、派发器或执行协议，不自动安装、登录、请求模型或启用配置。

本地扫描目录在原`opencode-sdk`之后追加第九项`claude-code-sdk`，原八项顺序保持；复用原单项和批量发现。product为原生程序实际产品名`Claude Code`，adapter区分SDK用途。该项识别原生Claude程序及实际版本／SHA，不以CLI存在推定SDK资源或模型认证就绪。原生CLI精确`2.1.314`时仍为`launch_supported=false, reason_code=configuration_required`；其他已安装版本为`version_unsupported`，不能回退claude-jsonl-v1。SDK固定`0.3.314`，宿主runtime为`suma-claude-runtime 1.0.0`。

原`manageLocalTool`的connect增加互斥闭合`claude_sdk_setup`：`{executable_sha256,model,credential_env_ref}`。SHA绑定用户刚看到的准确扫描；model／env-ref沿原Claude SDK约束。禁止secret值、路径、argv、Node／host路径、测试结果、任意环境及其他setup混用。普通status／stop／renew／recover等不携带setup；未知原操作重连不携带新配置，不用新参数覆盖旧Journal。

主进程在任何新Worker登记前重新核对原生CLI字节／版本、受信Node和完整SDK宿主资产。缺失、漂移或引用环境不可用返回明确不可用，不先创建身份。仅在原节点确认停止且无未结操作时允许替换配置。路径与执行形状只由宿主生成；凭据值只可由本机按明确引用私下向原Runner转发，不输出、记录、散列或持久保存值，不读取登录文件。

凭据引用必须是独立名称：`^[A-Z][A-Z0-9_]{0,127}$`；拒绝`SUMA_`、`NODE_`、`NPM_`、`CLAUDE_`、`LC_`、`LD_`、`DYLD_`前缀，以及`ANTHROPIC_AUTH_TOKEN,HOME,USERPROFILE,APPDATA,LOCALAPPDATA,PATH,SYSTEMROOT,WINDIR,COMSPEC,PROGRAMDATA,TEMP,TMP,TMPDIR,LANG,LC_ALL,PATHEXT,SYSTEMDRIVE,ALLUSERSPROFILE,PROGRAMFILES,PROGRAMW6432,COMMONPROGRAMFILES,COMMONPROGRAMW6432,HOMEDRIVE,HOMEPATH,PUBLIC,OS`。Go、SDK宿主、桌面与两种Web配置入口使用同一规则，避免把所选凭据放入版本探测保留的OS环境。原生版本探测复用既有有界进程生命周期，取消／停止失败保留未知及真实进程树证据，不无界等待或制造零进程已停止事实。

## 私有配置与完整摘要

`DesktopManagedRunnerInput.claude_sdk_configuration`为闭合私有对象：

```ts
{
 native_cli_executable: string; native_cli_sha256: string;
 reviewed_cli_version: '2.1.314';
 node_executable: string; node_sha256: string;
 runtime_manifest: string; runtime_manifest_sha256: string;
 model: string; credential_env_ref: string;
 configuration_sha256: string;
}
```

沿原`BuildClaudeSDKCommand`与`ClaudeSDKConfigurationDigest`，固定GoJSON字段顺序：`{policy_version:1,sdk_version:'0.3.314',cli:{native_cli_executable,model,credential_env_ref,reviewed_cli_version},node_sha256,native_cli_sha256,manifest_sha256}`。CJS编码包括Go的HTML／U+2028／U+2029转义，摘要绑定完整受信配置，不另造只含模型／环境引用的弱摘要。

Go复核三组路径与字节、完整manifest、原Git基线及准确配置摘要，再构造原Node＋SDK bundle CommandSpec、工作区隔离和ClaudeSDKSpec。原HMAC身份、加密Journal、固定工作区、CAS／原操作回执、续权、停止及未知结果恢复保持。完整宿主资产闭包内不能混入Node、桌面打包收据或运行时文件。

## 连接证据与登记

SDK连接沿原connection，新增该分支必需`claude_sdk_execution`，严格复用现有`ClaudeSDKExecutionCapability`闭形。connected必需`model,credential_env_ref,tool_version,executable_sha256,runtime_executable_sha256,configuration_sha256,claude_sdk_execution`。credential_env_ref只返回原持久配置中的环境变量名称，绝不返回其值；当前本机项目／身份授权先于读取。tool_version／executable_sha256指原生CLI；runtime_executable_sha256指Node，不混淆二者。能力中的SDK／CLI版本、模型、完整配置摘要、model_source=administrator_sdk_option、budget_mode=observe_only、identity_evidence=sdk_init_model_ack_only及auth_strategy=api_key_env_ref均须准确。

主进程是本机私有配置权威；渲染器不掌握Node和完整宿主路径，不复算或冒造完整摘要。渲染器只接受当前scope／当前请求代次的原可信IPC回执，核对用户选择的原生CLI SHA／版本、模型和环境变量名称，再保存该准确完整配置证明。草稿或scope变化立即使旧证明失效；单调代次隔离A→B→A后的旧回调，不能靠字符串再次相等复活。

只有同模型／同原生SHA的status不能证明环境引用已采用；必须同时核对可信主进程返回的准确credential_env_ref。完整connected回执匹配当前公开输入时，可恢复原连接证明，包括新页面读取、首次ACK丢失及starting转connected；starting缺少完整字段时仅显示进度，不提前声明登记就绪。状态没有准确环境名称或与新草稿不同则不得登记。由原配置回显变量名称解决来源歧义，不新增弱setup摘要、第二Journal或仅依赖renderer记住首次ACK的并行关联。未知原恢复不携带新setup、不改变原配置；实际更改仍先按原生命周期明确停止，再新明确连接。

原Worker启动后新鲜完整库存还须准确匹配该worker、Node SHA、正式Profile.tool_version=`2.1.314`及完整claude_sdk_execution能力／配置摘要，才能沿原Create／matches形成paused档案；不能将旧库存、CLI扫描或连接图标当作已登记／已启用。T192原`claudeSDKVersion`与`SupportsClaudeSDKExecutionProfile`的正式工具版本均指原生CLI，SDK宿主自身的`suma-claude-runtime 1.0.0`输出不替代这个字段，不引入双版本兼容。现有配置的调整继续沿T200正式交接，原明确启用与同提交独立质量验收保持。

## 资源与跨平台

成熟资源准备／打包流程增加受信`claude-runtime`资源，结构为Node及原许可证、打包收据与单独`host/`资产目录；完整复制当前已锁SDK bundle、manifest和其列明许可证。原生Claude CLI由用户已有安装提供，不打包或自动下载。开发态只使用仓库受核资产与同目标Node；打包态只使用控制面二进制旁目标资源，不接受渲染器指定来源。目标格式和交叉打包验证不能冒称该OS实际运行或安装。

## 交互与并发分工

原Quick／DesktopToolConnector增加SDK分支，常用表单只保留固定模型、环境变量名称及一个连接动作；高级手工指南按需展开。原生已安装、资源可用、节点连接、新库存、暂停登记和模型认证分别表达。中文／英文、窄屏、键盘、失败／缺资源／漂移／未知与重试保留原公开草稿和准确目标。

Root：共享合同、私有Go类型、必要Schema／文档、集成实际链与Git；Control：Go扫描、私有配置校验与构造及必要CLI识别；Runner：桌面CJS／Journal／资产验证／打包及其必要检查；Workbench：Web原入口、strict bridge、SDK薄连接组件及双语。分支不交叉写同一文件。仅验证新增和直接受影响边界，已有SDK执行、模型与完整质量链不重复；真实厂商账号、其他OS、多机、数据库、规模及生产继续分别验收。

Multica参考基线为本地`5a2cc891ac338eed1919aa11a1eef5957cd82cec`，本轮只读且工作区干净。借鉴daemon发现／有界探测／注册收敛的架构模式，复用suma已有成熟实现；该参考LICENSE含Hosted／embedded commercial条件，本轮不复制受其附加条件覆盖的源码。
