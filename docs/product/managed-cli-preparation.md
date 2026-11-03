# 已安装CLI一键接续配置

速码1.0.0 · T-MANAGED-CLI-PREPARATION-225 · 负责人：平台能力组。关联[合同](../../contracts/managed-cli-preparation.md)、[架构与AI SOP](../architecture/managed-cli-preparation.md)、[交互](../design/managed-cli-preparation.md)及[验证范围](../quality/managed-cli-preparation-verification.md)。

将原本机成熟CLI准备能力接入原远程扫描行。用户选择准确机器上的工具，按实际能力填写必要信息，明确确认后，由节点生成全新配置、确认库存，再回原快速接入保存暂停。已有配置不覆盖，启用与任务派发各自确认。

| 工具 | 用户输入 | 配置事实 |
|---|---|---|
| Qoder／Cursor ACP | 模型、凭据变量名 | 原固定ACP配置；账号值不上传 |
| Claude CLI | 明确固定模型 | 原CLI参数与完整Claude能力；沿用本机CLI身份 |
| Codex JSONL／CodeBuddy／dsh | 本次写入确认 | 原扫描命令，不覆盖本机模型或账号 |

只有节点明确允许已安装检测及远程准备，且准确扫描具备可核对来源，才可继续。机器路径、命令参数和安装依赖从原私有扫描回执取得，用户无需手工编辑。安装文件或依赖变化要求重新核对；无完整来源时保留原手工接入，不猜路径或静默忽略输入。

验收必须同时满足：六类配置闭形校验；原子库存与准确Profile绑定；Claude固定模型与能力一致；其他三种非空覆盖被拒绝；Node host与受限安装树漂移被拒绝；旧回执及未知效果不重复执行；中英文对应模式和窄屏可操作。成功准备、当前可用、启用、实际任务、独立测试和人工验收仍分别取证。

两SDK及Codex App Server的远程自动准备仍需要各自可信运行资源／provider合同，原手工入口保留。真实账号、厂商模型、多机和完整31项／P0–P7继续，不将自有测试程序视为厂商验收。
