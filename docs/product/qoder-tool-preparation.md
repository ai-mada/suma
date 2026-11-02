# Qoder 配置快捷准备与 AI SOP

速码1.0.0 · T-QODER-TOOL-PREPARATION-221。负责人：主集成；关联[合同](../../contracts/qoder-tool-preparation.md)、[交互设计](../design/qoder-tool-preparation.md)和[限定验证](../quality/qoder-tool-preparation-verification.md)。完整31项／P0–P7继续。

## 用户流程

原Qoder ACP执行器已经存在。本阶段补齐本机配置准备和准确输出采用，避免用户手工拼接JSON。进入工具指南并选择Qoder，填写明确模型ID和可选凭据环境变量名称，再填写目标机器的原生可执行文件、已审核版本、PowerShell或Bash及全新JSON输出路径。版本、模型与登录状态均不会被界面猜测。

1. 复制预览命令，在目标机器明确执行。原Runner从私有标准输入读取五字段，只静态检查原生文件、目录和摘要，并独占新建配置。
2. 明确采用当前输出路径：普通指南只填写Shell、commands与准确工具；共享指南只展示本机引用，再继续原Pool接入。复制与采用分别操作，不依赖剪贴板成功。
3. 核对本机文件并沿原身份、真实库存、matches、暂停登记及明确启用流程继续。此处不自动创建连接、配置Pool、读取凭据或调用模型。

父目录必须存在；已有文件、链接或来源变化不能覆盖处理。若写入后来源核对失败，新文件保留供核对，先检查原文件再决定处理，不反复覆盖或声称成功。Windows要求经过审核的原生.exe，不能把npm的.cmd启动脚本当原生程序。

## 能力与维护边界

复用原ACP模式，不增加新flag、执行器、HTTP服务或调度权威。私有五键为adapter_id、executable、reviewed_version、model、credential_env_ref；Qoder可显式空凭据名称，Cursor继续原必填凭据及固定Node包闭包规则。固定Qoder参数为--acp，不启用bypass或yolo。官方接口参考[Qoder ACP](https://docs.qoder.com/cli/acp)，版本和真实能力仍由原发现／执行链独立核验。

AI操作先确认准确项目、节点、协议与原待核对写入，再从严格表单生成准备命令。不得推断私有路径或模型、接受凭据值、把prepared解释为厂商认证。字段、账号、工具和锁变化使旧回应失效；各专用表单与命令组合器使用独立组件标识，来源切换不得保留旧项目表单。

本阶段没有证明真实Qoder初始化、授权、模型运行或任务交付成功；这些继续按独立证据验收。
