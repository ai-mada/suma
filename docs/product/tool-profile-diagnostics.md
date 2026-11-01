# 工具配置自动体检

T-TOOL-DOCTOR-181；suma 1.0.0。依据[领域合同](../../contracts/tool-profile-diagnostics.md)，配套[架构与 AI SOP](../architecture/tool-profile-diagnostics.md)、[交互设计](../design/tool-profile-diagnostics.md)与[英文说明](../en/tool-profile-diagnostics.md)。当前限定实现与验证见[阶段报告](../quality/tool-profile-diagnostics-verification.md)，完整生产验收仍待完成；完整 31 项／P0–P7 目标保持 active。

用户选定正式工具档案后，原详情自动读取当前等待原因，提供已有管理入口。体检说明当前事实，不打健康分，也不替用户执行下一步。

## 四项独立事实

| 固定顺序 | 含义 |
| --- | --- |
| `authorization` 配置授权 | 原档案状态及当前启用人权限是否满足条件 |
| `worker` 节点身份 | 原私有／Pool 节点身份、凭据与资格是否有效；有效不代表机器在线 |
| `inventory` 库存时效 | 原节点是否上报准确且新鲜的库存 |
| `profile` 完整配置 | 新鲜库存中的原 command、完整 Profile 与原 Observation 是否匹配 |

每项只取 `pass`、`blocked`、`not_checked`。库存未上报、过期或来自未来时，库存阻断，完整配置为 `not_checked`；旧 Observation 不能产生通过结论。未启用档案仍可读取节点和库存事实。

原工具 `Available`／`Reason` 及 Observation 继续是原可用性判断；体检复用同一判断，不增加 ready 或健康权威。默认展示固定顺序中第一个阻断原因。

## 原入口中的明确下一步

| 建议 | 已有入口 |
| --- | --- |
| `review_profile` | 暂停或启用人权限失效时，打开原版本 CAS 启用确认；当前非撤销已启用档案也须明确重新审阅 |
| `open_worker` | 节点资格、凭据、池或库存问题，定位当前准确节点的原管理入口 |
| `register_profile` | 撤销或完整配置改变时，进入原快捷配置流程 |
| `refresh` | 未知或需要当前事实时，只读重查 |
| `none` | 已知前提通过，不提供额外动作 |

只有父级已提供、绑定当前准确来源的入口才显示按钮。缺少准确入口时保留原因与重读；不会自动新建节点、探测本机、启用、续期或修复。

## 时效、身份与原操作

`checked_at` 来自同一数据库时刻；`expires_at` 最多延后 60 秒，且不超过新鲜库存原期限。这是展示期限，不是授权租约。界面从请求起点用单调计时扣除网络时长，过期后隐藏可操作建议并允许重读。

原 pending 写入始终优先。新报告不能清除未知写入、替换原 body／稳定 key，或绕开原确认、当前权限、CAS 与操作回执。

API caller、账号、组织、项目、工具 ID 或版本变化后丢弃迟到响应。报告与当前详情版本不同就提示刷新原详情；旧报告不能解锁原动作。

## 只读 CLI 与范围

`suma --project PROJECT tools doctor --id PROFILE_ID` 复用原登录会话、当前项目视图和 client，读取同一诊断 GET，严格核对 scope、完整 Tool、四项及期限。命令只输出事实，不运行建议，不调用模型／CLI／SDK，也不产生写入。

本片沿原工具、Worker、库存和操作链，不新增调度或 repair。节点身份、库存、模型登录、预算、实际执行、测试和人工验收分别记录；体检报告不代表真实厂商运行或完整平台验收。
