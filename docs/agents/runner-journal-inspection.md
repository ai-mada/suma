# 查看 Runner 本地执行日志

[English](../en/runner-journal-inspection.md) · [精确合同](../../contracts/runner-journal-inspection.md)

关联任务：T-DELIVERY-112；产品版本：1.0.0。此入口便于在保留原执行状态的前提下，查看本地有哪些结果、进度或制品仍待回写。

```text
suma-runner --inspect-state --state .cache/runner --language zh-CN
```

共享资源池可先按原启动目录查看所有本地槽位。环境令牌模式默认目录为 `.cache/runner`。配对身份模式默认使用 `<身份文件>.journal`，显式配置 `--state` 时沿用实际目录。

```text
suma-runner --inspect-pool-state --state .cache/runner --language zh-CN
```

先看有未知结果、待回执或领取记录未完成的槽位。总览只提供槽位、数量和本机检查点阶段。再按实际编号查看原尝试，例如：

```text
suma-runner --inspect-state --state .cache/runner/pool/slots/0 --language zh-CN
```

无需 Runner token、模型账号、控制面地址或工具配置。命令不会初始化目录、取得运行锁、修改结果、确认回执或重新启动任务；活动 Runner 可以继续持有自己的写锁。不要把诊断与执行参数混在同一次命令中。

先看 `status`。`available` 表示本地日志已读取；`missing` 表示指定日志不存在；`invalid`、`unreadable`、`over_limit` 分别表示内容无法解析、不能安全读取或超过 16 MiB。失败不会输出原始载荷，也不会把局部数据当成完整摘要。

`counts` 统计本地日志中的待确认结果、未知结果、待回写进度、未形成结果的收件项、待制品及待候选检查点。`items` 最多显示 100 项；`complete:false` 时需要注意条目列表不完整。摘要引用用于区分本地记录，不可用作操作凭据。

本命令只读 `journal.json`。它不检查领取检查点、工作区保存 sidecar、服务器当前回执、租约、真实拒绝原因或执行器健康。`acked` 只是本地记录；pending 也不等于已被服务器拒绝。输出包含安全标识和计数，排除了日志正文、命令、配置、路径、口令与执行载荷。

发现未知执行或回写积压时，保留原私有状态，按正常流程核对原操作。该入口不提供删除事件、手动 ACK、清空日志或自动重跑功能。当前实际验证范围以本阶段交付记录为准。
