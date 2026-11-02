# OpenCode SDK 原生新文件准备合同

状态：本阶段限定验证；版本1.0.0；阶段T-OPENCODE-PREPARATION-207；负责人：主集成、Runner、Web与共享指南代理。关联原工具准备、T203准确目标、T204库存反馈与T206简洁首屏。

## 目标与边界

管理员在原工具管理选择OpenCode SDK，填写固定供应方、模型、服务地址及凭据环境变量名称，在按需机器设置中填写Node、原生OpenCode、已审核运行包manifest和新的commands路径。界面生成准确准备命令；在目标机器明确执行后，只创建一个新的配置文件，再接回原检查／上报、登记paused和独立启用流程。

不新增API、存储、调度器或依赖；不安装包、登录、读取密钥值、运行CLI版本探测／SDK／模型、启动Worker或连接控制面。本步骤静态检查本地文件及原审核资产闭包，不把固定审核版本当作实际原生版本观察。实际版本、配置ACK、模型可用及执行资格仍由原库存与正式派发验证。

## 唯一私有输入与文件输出

`suma-runner --prepare-tools <new-absolute.json> --prepare-opencode-stdin`从受限私有stdin读取正好九个字符串字段：

| 字段 | 规则 |
| --- | --- |
| `adapter_id` | 固定`opencode-sdk` |
| `node_executable` | 目标机器的原生Node绝对路径 |
| `runtime_manifest` | 已审核包的`suma-opencode.manifest.json`绝对路径 |
| `native_cli_executable` | 目标机器原生OpenCode绝对路径，拒绝脚本包装器 |
| `provider_id`、`model`、`base_url` | 沿原OpenCodeSpec的固定模型、地址与供应方校验 |
| `api_key_env_ref` | 必须提供字符串，可为空；只允许原许可变量名称，不接受值 |
| `reviewed_version` | 固定当前`1.19.4`审核SDK基线 |

未知字段、缺项、重复键、非法Unicode、超限、显式false和混用身份／执行／发现／其他准备模式均拒绝。目标目录必须已存在并符合原rooted写入合同；原包目录、已有文件及链接等按原安全规则拒绝。取消、输入或资源校验失败不能留下新配置。

复用`managedOpenCodeCommand`固定构造；bundle由manifest父目录的固定名称推导，不接收任意argv或bundle入口。复用原Node／原生文件静态检查、`nativeManifestDigest`完整资产闭包、`validateOpenCodeSpec`与`OpenCodeConfigurationDigest`，写入前重新核对依据。输出仅一个`opencode-coder`命令，`native_workspace.checks=[]`，不能引用新文件中不存在的检查命令。经原O_EXCL／0600／同步写入器落盘，不覆盖用户文件。通用配置可由管理员随后按原完整允许列表合同明确编辑，不在快捷准备阶段虚构独立测试命令。

## 界面与准确交接

中文与英文同键；固定模型／供应方／地址／变量名称为常用信息，目标机器与文件设置按需展开，不从浏览器操作系统推测远程操作系统。PowerShell与Bash沿原安全字面量引用，PowerShell的UTF-8编码覆盖仅在生成命令局部恢复，不改变系统配置。

明确“使用此配置继续”只传`OpenCodeSetupDraft {shell,commands,localTool:'opencode-sdk'}`。原ToolSetupGuide与RemoteRunnerSetupGuide闭合解析保留准确项目／节点、调用者、代次、选择nonce及配置nonce；旧请求、换项目和ABA不能覆盖新目标。继续复用原ToolReportFeedback的准确工具库存检查、限时等待和返回Quick，不自动登记或启用。

## 实施分工与最小验收

- Runner代理：新`internal/runner/opencode_preparation.go`及测试，CLI模式与专门helper／测试；不改Web。
- Web代理：原OpenCode配置模型／组件、双语域及必要组件样式和定向测试；不改共享Guide。
- 共享代理：原ToolSetupGuide／RemoteRunnerSetupGuide严格union、回调、原反馈及定向交接测试。
- 主集成：合同、产品／架构／AI SOP、跨语言真实文件准备与代表界面、一次类型／语言、白名单提交与dev推送。

验收证明闭合输入、安全新文件、原资产校验、shell引用、准确交接及未知／过期保护不回退。跨语言实际验证用Web生成的同一JSON，经真实Go CLI创建文件，确认固定入口与空检查、不覆盖已有文件、没有原生／模型／控制面运行。生产组件浏览器使用明确标注的本地隔离数据，不能冒称实际供应方连接。未变化旧链与完整设备矩阵不重复执行。完整31项／P0–P7、58限定／34待办／92总项保持进行中。
