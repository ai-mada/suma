# 真实 CLI 接入与只读诊断

T-REAL-CLI-ONBOARDING-226，suma 1.0.0。本文说明已安装工具的发现、元数据探测和暂停登记；详细字段以[阶段合同](../../contracts/real-cli-onboarding.md)为准。[English](real-cli-onboarding.en.md)／[排障 SOP](../operations/real-cli-onboarding-sop.md)。

## 已知真实安装信息

本机已静态读取以下 Codex 文件，未复制程序、运行安装器或读取账号文件：

| 项目 | 观察 |
| --- | --- |
| 路径 | `C:\Users\Administrator\AppData\Local\OpenAI\Codex\bin\22cfb1d302613eae\codex.exe` |
| 文件大小 | 368970544 字节 |
| SHA256 | `3097c909fb6d5cd63510d44aceb1d42df9f6e85722a3e447e477def1ecf60e16` |
| 文件属性 | 普通文件，具有 Archive 属性；没有可读的 PE 文件版本资源 |
| 嵌入证书主体 | OpenAI OpCo, LLC；仅读取证书元数据，未验证证书链、吊销状态或签名时间戳 |

这些事实确认了准确的本机候选，不能代替实际 `--version`、帮助协议、账号或模型验证。不同进程的 PATH 可能不同；未在某个 PATH 中找到，不等于没有安装。Cursor 编辑器启动器也不能代替 Cursor 智能体／ACP 入口。仓库缓存中的自有小型测试程序不作为厂商安装证据。

## 从扫描到暂停登记

1. 在工具管理中选择准确工具和节点，沿原本机扫描或远程安装扫描读取当前来源。扫描不授权安装、配置写入或任务执行。
2. 版本、帮助和固定协议检查沿原进程控制器运行。无可信上下文时，系统为 HOME、账号配置目录、临时目录和工作目录创建私有空间；不会把配置表中的任意 HOME 当作可信私有目录。
3. 已有可信 helper 创建的 Codex、SDK 或 Cursor 目录通过内部上下文传递。Codex 为功能检查生成的配置保持原样，不被维护扫描改写到另一个 HOME。版本仅取有界 stdout；stderr 保持有界和净化，不能变成版本首行。
4. 需要自动准备时，沿[原受限准备流程](../architecture/managed-tool-preparation.md)明确配置与机器写入许可。原工厂、不可变 generation 和来源校验继续生效；`prepared` 仅表示已准备并等待库存确认。
5. 准确关联的原库存提交和 ACK 完成后，再从原 Quick 入口保存暂停配置。启用和派发仍是独立操作，必须满足其各自当前权限与来源要求。

扫描结果、文件生成、generation 接纳、库存确认、配置登记和启用是不同事实。新文件的摘要不能替代 Profile 摘要，也不能证明库存已确认。发生回执丢失时，恢复同一原请求及其操作标识，不重新准备或换一个请求碰碰运气。

## 分类诊断与原动作

可分类失败只公开 `diagnostic: {stage, code, next_action}` 三字段。阶段为 `resolve`、`source`、`version`、`help` 或 `protocol`；完整合法组合见[合同表](../../contracts/real-cli-onboarding.md)。浏览器使用固定中英文说明，不展示机器路径、argv、环境值或原始 stderr。

| 建议 | 使用方式 |
| --- | --- |
| `guide` | 打开该工具和准确目标节点的原接入指南，按当前原因人工处理 |
| `scan` | 当前权限、来源及维护状态允许时，明确发起本机重检或该远程节点的单工具重检 |
| `none` | 停止状态尚未确认；保留原记录，先走原核对流程，不再启动探测 |

权限诊断必须来自真实系统错误；不会靠错误文本猜测。解析阶段无法确认来源时显示 `scan_incomplete`，不冒充“未安装”。诊断缺失仅表示没有可分类的故障事实，不表示健康。旧请求未知、待恢复回执或当前忙碌状态优先于建议按钮。

## 边界与验证状态

当前完整接入证据限 Windows。Unix 原进程组为空并非 detached 后代停止证明：原 collector 无法形成完整停树证明，同一可信 scope 的连续帮助／特性探测会被阻断并保留目录。独立探测在缺少完整停树证明时也明确返回 stop_unverified，不把捕获文本当作成功。该已知退化及成熟 containment 的后续工作见[报告](../quality/real-cli-onboarding-verification.md)。

私有目录隔离避免元数据检查读取现有用户账号目录，不等于操作系统沙箱，也不证明工具全部内建行为受到限制。只有可靠的原等待／停止事实允许清理独立目录；未知停止保留目录和原维护记录。维护目录继续由原 Journal 管理，不按 PID 缺失或超时推断已经停止。

本阶段真实 Codex 已读取为 `codex-cli 0.166.0-alpha.5.1`，一次原 HTTP／SQLite／普通维护 Worker 实跑贯通扫描、准备、库存确认与暂停登记。18项定向检查通过，中英文原两组代表组件及一组历史保护补充验证通过；精确证据与边界见[报告](../quality/real-cli-onboarding-verification.md)。自有程序用于隔离、stdout 和错误投影回归，不能当作真实厂商协议验收。模型调用、账号登录、计费、独立代码测试、人工验收、多机及其他操作系统仍需各自证据；完整 31 项／P0–P7 目标继续。
