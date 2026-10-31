# OpenCode SDK 桌面快捷连接

T-OPENCODE-QUICK-CONNECT-177；suma 1.0.0。状态：实施合同。沿用 `opencode-sdk` / `opencode-sdk-v1`，SDK／server 固定 1.19.4，原完整执行合同见 [opencode-sdk](opencode-sdk.md)。

## 统一入口

1. 原被动安装目录增加 `opencode-sdk`，成为八个协议接入项。仅固定 PATH／已知官方包解析，返回原生程序绝对路径、SHA、实际版本；无启动命令、无模型请求。已安装且精确支持时 `launch_supported=false,reason_code=configuration_required`；其他已安装版本为 `version_unsupported`，不可降级通用 CLI。
2. 复用 `discoverInstalledTools` 原单项请求以及 T176 批量请求。Go／桌面／Web 闭合目录一起更新；初始版本不回退旧七项。
3. 原 `manageLocalTool` 的 connect 允许 OpenCode 专用 `opencode_setup`：`{executable_sha256,provider_id,model,base_url,api_key_env_ref?}`。闭合对象，不接受路径、命令、Node、脚本、manifest、环境值、测试结果或任意参数。与 ACP／Codex selection 互斥。固定模型明确填写，按已有 SDK 合同校验，不能冒充实际模型目录发现。
4. Desktop 在任何新的 Worker 登记前重新读取该单项安装，核对精确原生 SHA 和版本，并解析宿主受信 SDK 资源及 Node。缺资源显示 `unavailable`，不发行新身份。普通 connect 不携带新 setup 时核对／恢复原配置；新配置仅在原节点已确认停止且无未结操作后允许。
5. 原加密 journal 保存完整私有配置并使用原 HMAC 身份、工作区、CAS／操作回执及 Worker 生命周期。所有资格检查、停止／未知事实和明确启用规则保持。

## 私有执行配置

`DesktopManagedRunnerInput.opencode_configuration` 为闭合私有对象：

```ts
{
 executable: string; executable_sha256: string; reviewed_version: '1.19.4';
 node_executable: string; node_sha256: string;
 runtime_manifest: string; runtime_manifest_sha256: string;
 provider_id: string; model: string; base_url: string; api_key_env_ref?: string;
 configuration_sha256: string;
}
```

路径只由宿主构造。`configuration_sha256` 复用 Go 的 `OpenCodeConfigurationDigest(OpenCodeSpec)`（policy_version=1、sdk_version=1.19.4、原固定 provider 结构和 JSON 编码），不得另造选模摘要。Go 检查原生与 Node 字节、manifest 字节、原 Git 基线和完整 SDK 资产后，固定构造原 Node＋bundle CommandSpec、NativeWorkspacePolicy 与 OpenCodeSpec，交原 Worker。此入口无需新增模型发现 CLI、数据库或 SDK／调度器。

连接回执沿用原 `connection`，OpenCode 可带 `model,tool_version,executable_sha256,configuration_sha256,provider_id,endpoint_origin`，connected 时这六项必需；这里的 executable 是原生 OpenCode。提供方与安全 origin 来自原持久配置，不从当前草稿推导，不返回完整地址或凭据。启动后的新库存另核 **Node SHA**、runtime tool_version=`suma-opencode-runtime 1.0.0` 以及准确 `opencode_execution`（固定 SDK/server/provider/model/origin/config摘要、observe_only/configuration_ack_only）。不能拿原生 server SHA 与库存 Node executable SHA 比较。

## 可信资源与安装

资源解析优先宿主控制面二进制旁的 `opencode-runtime/`，开发态可使用仓库现有 `packages/opencode-runtime/dist` 及明确探测的本机 Node；均不接受 renderer 路径。打包使用现有完整 runtime manifest、tool/plugin 真实依赖及许可证，附目标平台 Node；资源与目标必须一致，跨目标缺少对应 Node 时明确报错，不拿宿主二进制伪装另一平台。沿原打包计划和资源校验管理，不复制到无关 source-inspector 目录。

## 简洁体验与范围

原工具列表／快速连接提供原生安装检查及四项固定配置（提供方、模型、地址、可选凭据环境引用），再明确连接并验证，随后原新库存→暂停登记→明确启用。高级指引提供完整管理员配置例子。中文／英文完整，切换项目／身份拒绝晚到结果，错误说明下一步，不自动安装／登录／选择模型或启用。

资产验证与离线 probe 不证明厂商身份、模型成功、硬预算或计费。任务继续使用原 fixed_opencode 选项、隔离工作树、宿主工具、候选、同提交独立测试及人工验收。定向检查和可得实际原生观察分开披露；不重复历史全套或为本片跑完整设备／部署矩阵。原31项／P0–P7范围不变。
