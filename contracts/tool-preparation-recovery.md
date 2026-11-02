# 工具准备诊断与准确交接合同

T-TOOL-PREPARATION-203；suma 1.0.0；负责人：主集成代理。完整31项／P0–P7及58 limited／34 pending／92基线保持active。复用[工具管理](tool-management.md)、[桌面SDK接入](claude-sdk-quick-connect.md)、既有原节点和远程准备指南。

## 本阶段必须闭合的断点

1. 已选工具的“准备当前节点”必须交接同一适配协议及准确节点，不能回到默认Codex／全部工具准备。
2. 本地Claude SDK和OpenCode SDK准备失败必须保留有限、非秘密的准确原因，进入对应工具的原指南或配置；不把所有`unavailable`猜成缺资源。

## 桌面私有连接事实

沿既有`manageLocalTool`连接响应新增可选`connection.preparation_reason`，仅允许`phase=unavailable`且`adapter_id`为`claude-code-sdk`或`opencode-sdk`。允许值固定为：

- `native_version_unsupported`：当前原生版本观察明确不受支持。
- `installation_changed`：选定原生安装的实际文件身份不能成立。
- `sdk_assets_missing`：受信SDK资源边界内的必要文件缺失。
- `sdk_assets_invalid`：SDK清单、完整性或受信资源布局不成立。
- `node_runtime_unavailable`：受信Node缺失、目标不符或原版本检查未通过。
- `credential_reference_unavailable`：明确环境引用在当前进程不可用。

在准确检查边界抛出内部typed原因，禁止解析异常消息来推断原因。公开响应不含文件路径、密钥值、环境值、原始异常或stderr。未分类的错误保留原通用失败，不创造猜测原因。

此字段是本次准备观察，不是新的配置或健康权威，不写成Journal执行状态。原未知登记、续权、停止及回执恢复优先处理，不因诊断启动第二操作、丢失原意图或重probe。`status`／恢复不为诊断增加新探测；明确连接仍按原机制复核实际资产和凭据，登记、启用、模型认证和质量分别表达。

Web私有closed reader拒绝未知原因、错误类型、错误adapter或phase。已有诊断映射原因，仅在调用者拥有现成callback时展示guide／configure／detect；原操作pending优先reconcile；凭据引用原因的配置动作定位到明确环境名称字段，不能冒称模型账号认证失败。不透传HTML或执行任意命令。当前scope／调用者及迟到结果隔离继续适用。

## 同一工具、同一节点的准备交接

原Quick→Workspace→ToolSetupGuide→RemoteRunnerSetupGuide携带准确适配选择和目标worker；复用原配对资格、当前调用者、项目、nonce与请求所有权。

- 显式选择来自当前正式适配描述符；未知或不匹配协议不产生默认工具配置。
- 原SDK／ACP专门配置入口继续沿既有生成器；Go不支持通用准备的适配器不能生成“全部工具”的通用prepare冒充完成。
- 已有远程命令仍是目标机器的公开草稿，只复制，不执行、安装、登记、启用或修改身份。
- 选择请求仅在当前目标和原nonce下明确采用一次；普通渲染、迟到请求、身份／项目／节点变化不能覆盖人工草稿，A→B→A不能复活原旧请求。
- 新目标选择不能被同一worker上旧工具的配置请求覆盖。壳类型、路径、代码基线等仍由用户和现有指南提供，不借浏览器OS推断远程OS。
- 保留原工作区、配对收件箱、原操作恢复和明确启用入口，不新建连接中心或派发器；补齐下述原生Codex配置准备模式。

## Codex App Server实际配置生成

源码核对确认该协议缺少远程配置生成器，本阶段一并补齐。

- 原Runner增加`--prepare-tools <new.json> --prepare-codex-stdin`，先排除显式false、混用、无效目的地等模式，再读取私有stdin。
- 输入必须正好七项：`adapter_id`固定`codex-app-server`；`executable`、`provider_id`、`base_url`、`api_key_env_ref`、`windows_sandbox`为字符串；`enable_user_input`为明确布尔值。引用和sandbox按原平台规则可为空，Windows目标仍必须明确sandbox；不从浏览器OS推断目标。
- 复用`codexDiscoveryCommand`、`validateCodexAppServerSpec`及原生文件检查，生成唯一`local-codex-app-server`命令；固定参数`app-server --listen stdio://`，不接收任意argv或密钥值。
- 原rooted／O_EXCL写入器只创建一个新commands文件，不覆盖已有文件，不启动厂商CLI、模型、Worker、身份或平台连接。准备事实不证明真实Codex版本或模型可用。
- Web专门表单沿原壳引用生成private stdin及准确命令，明确采用后把同一commands路径、shell及协议接回原Remote指南。

## 实施分工与证据

宿主代理拥有两个SDK资产读取、原managed连接及相关桌面定向检查。控制代理拥有私有Web响应解析、诊断模型／组件及其双语和定向检查。交互代理拥有准确准备目标交接、原Guide／远程命令草稿和相关组件检查。主集成代理拥有合同、Codex配置准备Go模式及定向检查、通用类型／国际化、代表界面、文档、白名单与阶段Git交付。

仅执行改动直接相关的分支与隔离资源检查。SDK模型查询、原完整登记／测试链、未变化Go、全套设备和生产验收不重复；需要时复用原证据并注明范围。真实资源检测、合成组件交互、模型认证、执行与正式验收各自记录。完整平台目标继续。
