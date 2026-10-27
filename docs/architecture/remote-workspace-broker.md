# 远程工作区出站读取 Broker 后续设计

状态：T-P4-036 后续设计，尚未实现。当前 `developer check` 仅验证 CLI→控制面项目 MCP 的真实只读调用，不代表远程 Runner 文件、Git 或 IDE 已接通。

现有 Runner 通过出站 `claim`／`heartbeat`／`progress`／`results` 与控制面协作；原生工作区 MCP 仅监听 `127.0.0.1`，绑定单次 assignment 的 run／node／attempt／fence／worker，在执行关闭后撤销。不能把该临时 bearer 或端口返回浏览器，也不能将控制面的本地文件绑定当成 Runner 工作树授权。

下一阶段先提供列表与有界文本读取；继续复用 Runner 的 `os.Root`、路径／敏感文件检查、内容摘要及输出限制。Git 观察需要独立移植已有安全 Git 合同；PTY、写入、LSP、DAP 不随只读接入自动开放。

控制面持久化观察请求，固定原项目、当前调用者、run／node／attempt／fence／worker、工作区策略摘要、操作及严格参数、到期时间与一次性 request ID。调用者必须具有当前项目读取权；目标必须为当前仍运行的准确 attempt，不能只凭历史运行 ID 取得文件权限。Runner 仅用既有出站 HTTPS 凭证拉取分配给自己的观察任务，执行前复核当前 assignment、租约和工作区句柄；不增加入站公网监听。

请求状态为 `requested → leased → completed`，或明确进入 `expired`／`cancelled`／`failed`。读取租约独立于 DAG 执行租约；观察失败不能完成节点、触发重试或改写候选。Runner 提交结果必须携带原 request ID、准确 scope 和租约 fence；控制面只接受当前租约的一次有界回执。失联保留未完成事实，迟到／重复／旧 fence 回执不能覆盖已关闭请求。读取可在到期前重新派发，但必须增加观察租约 fence，禁止把不同时间的结果拼成一个快照。

浏览器读取完成结果时再次核验原调用者当前项目权限、相同 assignment 与请求到期状态；撤权、目标 attempt 变化、取消或过期均停止后续披露。结果只代表带 `observed_at` 的当前观察，不能变成正式代码候选、测试或验收证据。保留内容摘要、短期结果和审计定位，不记录工作区 bearer／宿主绝对路径。跨组织 Pool 的授权须同时核验现有 pool grant，不扩大 Runner 凭证为通用项目读取凭证。

验收必须覆盖真实分离进程的出站领取／文件读取／结果回传，以及撤权、路径越界、超时、旧 fence 和执行关闭后的拒绝。物理多机／TLS 部署另列实际验收，不能用同机协议夹具替代。
