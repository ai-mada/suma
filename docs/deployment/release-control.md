# 本机发布控制面

本轮实现将真实封存制品、构建/包测试证明、人工验收、发布计划、管理员批准、受控执行和提供方 HTTP 健康记录连接起来。API 精确字段见 [release-control 合同](../../contracts/release-control.md)，执行与信任边界见 [release-execution 合同](../../contracts/release-execution.md)。

## 配置提供方

控制面启动读取管理员进程环境变量 `SUMA_RELEASE_PROVIDERS_JSON`。配置只引用秘密变量名；不要把原始 token 写入此 JSON、工作流、计划或普通业务记录：

```json
[
  {
    "id": "local-static-1",
    "tenant_id": "替换为组织ID",
    "project_id": "替换为项目ID",
    "site_ids": ["preview-site"],
    "token_env": "SUMA_LOCAL_PROVIDER_CONTROL_TOKEN",
    "origin": "http://127.0.0.1:4350",
    "reconcile_token_env": "SUMA_LOCAL_PROVIDER_RECONCILE_TOKEN"
  }
]
```

ID 应替换为实际值。原始秘密至少 32 字节，Provider 向控制面使用的 token 与只读核对 token 必须不同，跨受众不能复用秘密。首轮实际核对只支持管理员固定的 HTTP 回环 IP origin（如 `127.0.0.1` 或 `[::1]`）；请求不能设置 origin、端口、目录、健康 URL 或命令。缺少配置时选项为空，不生成演示提供方或把 configured 显示成健康。

Provider 的本地根目录、站点与监听地址、控制面连接和 worker 传输凭证另由管理员管理；启动入口是 `suma-deployer --config <管理员配置文件>`。它提供静态文件，不运行上传内容或任意远程 shell。本页不提供生产云环境或厂商 CI 配置。

## 实际操作顺序

1. 管理员登记受信构建、包测试和部署 worker。普通共享项目 Runner 可继续旧任务，不能领取或授权管理部署。
2. Runner 从固定源码实际构建、上传字节并封存；包测试下载同一制品，在固定源码工作树执行实际测试。控制面在最终结果事务中派生证明，seal 本身不等于测试通过。
3. 对实际交付任务完成正式人工验收，再创建引用相同构建、包测试证明及任务版本的发布候选。浏览器只读制品/证明，不能手填 `passed`、构建结果或摘要制造候选。
4. 管理员创建环境，选择服务器已登记的提供方与站点，配置固定站点内健康路径。一个提供方站点不能重建另一环境以重置 generation；归档保留历史身份。
5. 为候选和环境预览新计划。创建时确认 `preview_sha256`；批准时确认不可变 `revision/sha256` 与可变 `version`。计划不原地改内容，修改需要新计划和审批。
6. 当前 browser 项目 admin 或组织 owner/admin 批准并执行。服务端同事务重核权限、候选依据、审批和环境，预约下一 generation、持有屏障并创建唯一管理 DAG。普通 reviewer、API/Agent 或 Runner token 不能批准发布。
7. 专属部署 worker 领取非秘密绑定，再在内存中领 120 秒 ticket。Provider 使用独立身份在线 consume，下载并校验真实制品，在激活前重查 begin-effect，实际切换后进行 HTTP 内容与代数检查。
8. 查看实际 active 版本、健康和问题。健康失败与未知分别保留；失败/未知会创建稳定问题回流，不被普通发布登记状态覆盖。

## 版本、重试与恢复

- `generation` 是最高预约代数，失败或未激活的预约也不回退。`active_generation` 是提供方实际观测的活跃代数；两者可以不同。
- 稳定幂等 key 和完整请求必须保留。超时后以原请求、原 key 恢复操作身份，取得 operation ID 后查询/核对原操作；不能换 key 掩盖未知副作用。
- 领取 ticket 不保存秘密回执。丢响应可重新领取，旧未消费 ticket 作废；已消费授权可由同一 Provider 按完整绑定只读 inspect。ticket 到期只限制首次 consume，已受理操作的 begin-effect 仍逐次重查当前权限、租约、审批、候选和屏障。
- effect grant 后，租约过期、停止请求和 Runner 退出都不清除屏障。未知状态无法手工填成成功，Provider 不可达时核对返回不可用并继续占用。
- 管理员核对从固定 Provider origin 读取结构化观测。过期或 reconciling 的管理 DAG 可在同一证据事务恢复已知终态；原 Runner unknown/失败结果保留在 `dag_managed_resolutions` 与事件中，不虚构停止 ACK。
- 后来撤批、撤权或任务变化会阻止新的授权。已经合法发放 effect grant 后实际发生的激活/健康仍如实记录，当前批准和候选失效分别显示，不能伪装已重新批准。审批历史单独不可变保存。
- 迟到的旧观测只进入历史，不能覆盖较新的 active、健康或屏障。同版本不同内容拒绝；终态不可随意改写。

## 回退

回退选择同环境历史真实健康操作，读取其准确候选、制品与原证明，创建新的 `action=rollback` 计划。候选当前验收依据仍需有效，重新批准、重新派发并消耗新 generation；Provider 校验原封存字节，再进行实际内容健康检查。它不会把旧操作改成当前成功，也不自动运行补偿命令。

## 验证边界

真实 SQLite/Git/子进程/字节上传/HTTP 的控制面与本机静态 Provider 联调记录见 [发布控制面验证](../quality/release-control-verification.md)。该证据不表示 PostgreSQL/OceanBase、云部署、真实生产流量、签名、跨机器制品分发或生产恢复已验收。未提供 Provider 不可达时的人工强制解除屏障或自动回退。

此前 `dist/runtime/builds/20261023T163759813Z-4b7523bf` 的六平台包保持原样，其源码哈希对应 P5 发布控制面加入前的版本；本轮 Go 源码新增能力不能据此声称已包含在那些历史包中。
