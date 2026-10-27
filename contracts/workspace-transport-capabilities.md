# 执行器工作区传输能力合同

状态：T-P4-039／1.0.0，有界本地实现。负责人：Control、Runner、工作台。关联需求：远程工作区、开发工具接入、真实能力发现及需求9、12、21、22。权威类型：[ToolObservation](../internal/toolcontract/types.go)；实现：[当前能力门禁](../internal/platform/workspace_capability.go)。本合同补充[观察合同](workspace-observation.md)和[保存合同](workspace-write.md)。

## 声明与权限

原生MCP工具与Go Runner的出站传输是不同能力。批准档案中的 `workspace_tools_version` 只证明工具合同；当前库存新增可选 `workspace_observation_protocol: workspace-observation-v1`、`workspace_write_protocol: workspace-write-v1`。write要求read；未知版本、不可用观察、普通CLI、缺原生工作区、只读用途均不能声明。由实际Runner代码在固定原生配置成功探测后填入，没有管理开关。协议声明不进入批准Profile或其摘要，不自动扩大权限。

声明沿原有机器认证、Pool双边范围和60秒库存有效期持久化。服务端仍检查当前原调用身份、项目、批准档案、准确活动attempt／fence／worker、工作区策略以及任务依据。受信Runner的声明不是远程二进制证明、文件句柄存活、提供方身份或连接健康证明。

## 行为

| 场景 | 结果 |
| --- | --- |
| 只有原生工具声明，未声明read broker | 不列远程目标；新观察／新保存拒绝 |
| 当前只声明read | 可列读取目标；`can_write=false` |
| 当前声明read＋write，且人／任务有写资格 | `can_write=true`，仍需准确活动句柄执行 |
| 创建后移除write | 首次执行和有效租约正文重发均拒绝，不重新授权写入 |
| 原POST回执丢失后移除能力 | 原key／请求摘要的命令metadata仍可恢复 |
| 效果已发生后移除能力 | 准确原结果和ACK可排空，不丢事实；不能借此重新领取正文 |
| 显式核对 | 按038合同读取原效果或无效果屏障，不需要新写能力、不重新执行 |

不增加表、路由、自动升级批准档案或历史兼容分支。未声明是当前事实，不能推断实现能力。

## 工作台导入

工具详情的“最近非敏感观察”折叠区展示两项原始协议及“未声明”，同时说明时效与健康边界。复制Runner实际库存时，导入器接受现有公开模型部署、原生工作区及只读用途字段，采用生成类型；私有配置、密钥、带凭据URL和未知协议仍拒绝。导入只是填写待审批Profile，不能替代认证机器的当前库存上报。模型部署准确SHA及真实当前依据由服务端再次验证。

验收依据见[专项报告](../docs/quality/workspace-capability-verification.md)，运维和架构见[接入说明](../docs/integrations/workspace-transport-capabilities.md)。
