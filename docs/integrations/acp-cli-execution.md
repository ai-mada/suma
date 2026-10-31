# Qoder ACP 与后续本地 ACP 工具

T-ACP-RUNTIME-170；suma 1.0.0。公开执行合同与实现分层见[合同](../../contracts/acp-cli-execution.md)、[架构与SOP](../architecture/acp-cli-execution.md)。当前源码支持受审阅本机原生入口；真实厂商安装、账号及模型须单独验证。

Qoder官方以`qoder --acp`提供stdio，复用CLI登录或`QODER_PERSONAL_ACCESS_TOKEN`。速码本机配置的`credential_env_ref`引用管理员环境变量，值仅送入该进程的官方变量，不写入浏览器、Profile或日志。配置和确认模型均由当前SDK会话证明，不能由程序文件存在推定。

`qoder-acp`表示速码出站执行；`qoder-client`表示IDE／MCP客户端接入，两者分别管理。通用ACP引擎可以复用到后续厂商策略，但新启动器、版本、权限、模型、配置隔离和停止行为必须分别实现及验收。

首次连接建议从原工具管理入口生成配置：原生绝对路径、同程序`--version`完整规范化输出和明确模型ID均不填假默认值。Windows脚本包装器当前不会当作原生程序执行；安装器和包装器解析仍按平台能力矩阵交付。

该执行方式仅观察用量；需要强制Token或费用预算的项目应选择已有受控模型网关工具。客户端文件／terminal RPC拒绝不等于限制工具内建行为；启用受信本机CLI前应按组织策略审阅其本机配置和执行权限。

依据：[官方ACP文档](https://docs.qoder.com/cli/acp)、[CLI参数](https://docs.qoder.com/cli/cli-reference)、[模型配置](https://docs.qoder.com/cli/model)。成熟SDK版本沿[既有许可清单](../delivery/workbuddy-acp-dependencies.json)，本轮未新增第三方包或升级SDK。
