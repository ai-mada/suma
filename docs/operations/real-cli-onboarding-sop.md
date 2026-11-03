# 真实 CLI 接入排障 SOP

适用 suma 1.0.0、T-REAL-CLI-ONBOARDING-226。供操作人员和 AI 协作者使用；字段以[阶段合同](../../contracts/real-cli-onboarding.md)为准，接入步骤见[中文指南](../integrations/real-cli-onboarding.md)。本 SOP 不授权安装、登录、模型调用或自动启用。

## 先确认当前操作

1. 确认当前组织、项目、准确 Worker 和工具 adapter。共享 Pool 节点继续使用原 Pool 身份与当前授权，不能改用普通项目身份。
2. 先读取原维护请求及回执。未知、待确认或进行中的请求先核对；不要换操作标识、删 Journal、重写 commands 文件或新开 Runner 来绕过占用。
3. 区分已观察事实：静态文件存在、版本／帮助通过、generation 接纳、库存 ACK、暂停登记和启用分别记录。PATH 未命中、SDK 发布 manifest、编辑器入口和自有测试程序不能替代真实 CLI 证据。

## 按固定诊断处理

| 观察 | 操作 |
| --- | --- |
| `source_unavailable`／`permission_denied` | 核对准确来源及本机访问权限；不绕过 ACL，不展示原始路径／环境秘密。打开原指南处理后再明确扫描 |
| `source_changed` | 当前文件或安装闭包已变化；保留原记录，明确重检取得新来源，不沿用旧摘要 |
| `configuration_invalid`／`version_unsupported` | 按原工具指南核对公开配置和受支持版本；不安装或偷偷放宽协议 |
| `launch_failed`／`probe_failed`／`output_limit_exceeded` | 保留分类记录，查看受控本机诊断；向用户只显示固定说明，修正后由用户明确继续 |
| `help_contract_mismatch`／`protocol_mismatch` | 当前程序未证明原固定协议；保持不可用，不将文本输出当作可执行能力 |
| `probe_timeout` 且停止已确认 | 当前来源／权限仍有效时，按建议明确重检；不要自动循环 |
| `stop_unverified` | 停止新探测，保留私有目录和原停止记录，走原核对／隔离重配对流程；不能根据 PID 缺失推断安全 |
| `scan_incomplete` | 安装解析尚未确认；处理当前解析或访问问题，不标记为“未安装” |

诊断缺失时保持未分类，不从 stderr 文本、程序名或历史成功记录猜原因。`next_action` 是建议，不是授权；当前权限、来源、未知回执和忙碌门继续生效。

## 执行一次明确重检

本机按钮复用原本机扫描；远程按钮只创建准确节点、单一 adapter 的原安装扫描。不要把读取旧报告当作新探测，也不要在旧请求未知时创建新请求。可信 helper 负责私有 HOME／CODEX_HOME／工作目录，人工或 AI 不修改其内部配置来借用现有登录。

扫描成功后，如需准备，明确配置与机器写入许可，沿原工厂生成并接纳 generation。`prepared` 后继续等准确关联库存 ACK；ACK 丢失恢复同一原操作，不重复工厂或探测。只从当前确认的库存保存暂停配置，启用仍单独决定。

## 记录与收口

当前 Windows 完整接入已验证；Unix 同一可信目录的连续探测及原 collector 停树证明存在已知限制，可能阻断帮助／特性与正式接纳并保留目录。遇此情况按未知停止核对，不靠重检、删目录或将进程组为空当作整树已停来绕过。该平台需专门 containment 实现后再验收。

只记录固定诊断、准确来源摘要、操作引用、generation／库存 ACK 和停止事实。秘密值、账号文件、完整环境、原始 stderr 和本机路径不进入公开错误。保留真实失败及原恢复证据；没有执行的版本、模型或检查明确写“未验证”。

本阶段实际版本为 `codex-cli 0.166.0-alpha.5.1`，版本／帮助、原库存和暂停登记已通过；[报告](../quality/real-cli-onboarding-verification.md)记录可复核范围。结束前检查自身探测是否已可靠停止，未知状态保留原目录及核对入口；不清理用户文件，不把阶段交付写成全平台完成。
