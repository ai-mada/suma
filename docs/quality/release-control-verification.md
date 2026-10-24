# 发布控制面验证

日期：2026-10-24。实际环境：Windows amd64、Go 1.24.5、真实 SQLite、Git、独立允许命令子进程及本机 HTTP。测试临时目录与 Go 缓存均在仓库 `.cache`；没有访问云目标、生产环境或付费服务。历史 runtime 交付包保持原样。

## 本次命令与结果

```text
go test ./internal/platform ./internal/releasecontract -run 'TestReleaseDeployment|TestDeploymentContract' -count=1 -timeout=180s -json
```

实际退出 0：platform 50.244 秒，releasecontract 0.306 秒。包含 8 个控制面行为场景、2 个共享协议场景、6 个负向子场景；额外一个自执行 fixture 入口不计作产品行为测试。未重复运行全 Go suite。精确用例、环境、原始日志 SHA 与九个本次实现/测试源码 SHA 在 [机器可读证据](evidence/release-control-review.json)。原始测试事件保留于 `.cache/release-control-final-tests.jsonl`。

## 实际覆盖

| 场景 | 实际结果 |
|---|---|
| 环境与当前权限 | 真实邀请的项目 admin（组织 member）可管理；reviewer、专属 Runner 不可管理；降为 viewer 后原成功回执也不能重放 |
| 环境身份/CAS | 同 provider/site 不能重复建环境，归档也不重置；真实两个 SQLite 连接竞争，恰一成功、一409 |
| 候选与唯一派发 | 原 B→真实开发 C→实际构建、上传/seal、下载后的包 HTTP 测试、正式 Review；两个连接同 key 派发仅一 run/op |
| 部署身份 | 普通构建 worker 自报部署能力不能 claim 管理 run；专属部署 worker 获顶层服务端绑定；复制普通图 Config 不授予 ticket |
| 临时票据 | 续期使未消费旧 secret 失效；错误 Provider/worker 被拒；consume 重放身份不变；历史 inspect 无 ticket；公开 DTO/普通回执无秘密 |
| 未知副作用 | 明确 unknown 不产生成功，保留环境屏障并创建稳定问题；重复及乱序旧观测保留历史，不覆盖当前；租约过期不清屏障 |
| 实际多版本部署/回退 | A 与不同内容 B 各有真实 Git/构建/封存/包测试/验收候选；实际 Provider 依次服务 A→B→A，三次新计划/批准/操作，generation 1/2/3，HTTP 内容随之变化 |
| 激活前在线门禁 | consume/begin-effect 重查当前租约、审批、候选和屏障；已撤批后不能把旧 effect grant 回执当新激活许可 |
| 结果丢失恢复 | 合法 effect 后先保存 Runner unknown，再撤批并改任务依据，随后租约过期；管理员读取实际 Provider 终态恢复 managed DAG，原 unknown 保存于独立审计，当前审批仍 revoked、候选仍失效 |
| stale/隔离/结构 | 审批后任务变更阻止派发，未留下 generation/run；跨项目 candidate/artifact/environment 被拒；未知字段、重复 JSON key、超深 JSON 拒绝 |
| 健康证明 | 错内容 hash、错 generation、缺 fencing ACK、无实际时间、伪改 digest 和 unknown 都不能形成成功 |

所有正向部署成功来自真实 Provider 的持久 active 指针和实际 HTTP 文件内容检查，未手填成功观测、证明或人工验收。负向 unknown fixture 只验证未知状态保持，不作为部署成功证据。

## 独立审查发现与修复

1. P1：真实 Provider 已健康，但丢失 Runner 最终结果且租约过期后，旧核对仅更新发布投影，管理 DAG 仍 `running/reconciling`。先以真实 A/B/A fixture 复现红测（15.75 秒），再接 root 的 `CompleteManagedObservedBound`：当前管理员与实际 Provider 绑定验证同事务，原 attempt 结果另存审计，绿测覆盖后续撤批和任务变化。新授权门禁与已发生事实分别处理。
2. P2：真实有效候选请求附加未定义 `passed:true` 被静默接受。独立 overlay 探针复现（6.34 秒）；root 为 Candidate 创建增加严格字段校验，永久回归现通过。没有把此问题描述为伪造构建或绕过 proof，实际 proof 原先仍被校验。
3. 控制面自身复核中，修正较旧观测乱序到达时的处理：保留不可变历史，不改变最新版本、active 或屏障；已执行计划详情不再错误显示环境仍 reconciling。

同时只读审查 root 的上传/封存、证明派生、候选任务/需求依据、managed run、CompleteBound 和 ClaimEligible 接入。当前证据未发现其他已验证 P1；未以本机 SQLite 测试推断跨数据库或生产并发结果。

## 未验收范围

浏览器验收由 root 另行负责，本子任务未运行浏览器旅程。PostgreSQL/OceanBase、生产云发布、签名、公证、跨机器分发、实际生产流量及目标系统安装未验收。Provider 不可达时保持屏障，不提供人手强制成功或强制解除；首轮无自动回退。先前六平台 runtime 包是本次 P5 Go 增量之前的历史源码快照，不能作为这些新增接口已打包的证据。
