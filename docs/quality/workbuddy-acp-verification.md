# WorkBuddy准确会话与ACP协议验证

阶段：T-WORKBUDDY-ACP-149；2026-10-30；速码1.0.0。只验证内部协议代码及固定SDK依赖，不开放云派发入口。完整平台及前后端任务流程继续实施。[合同](../../contracts/workbuddy-cloud-submission.md)、[架构](../architecture/workbuddy-cloud-submission.md)、[机器摘要](workbuddy-acp-summary.json)。

## 实现及实际验证

准确任务GET读取同ID的会话材料，秘密不进入Task投影、JSON或格式化诊断；仅接受官方网关。可用材料、有效期限、平台授权及连接健康各自表达。SSE和POST使用同一准确连接，POST接纳与RPC结果分别核对，不重连或重发未知操作。

固定社区SDK v0.13.5复用13类更新及权限类型；先核验JSON、session和discriminator再解析，未知扩展保留unsupported。单一回合、权限引用和取消均绑定本次连接；权限请求不自动批准。完整历史顺序消费，握手在接收SSE的同时发送；缺省loadSession能力不得尝试加载。

| 检查 | 实际范围及结果 |
| --- | --- |
| 准确任务／会话客户端 | 23个顶层、101个子场景通过，2.319s；其中8个顶层为149新增，其余15项为既有HTTP／OAuth回归 |
| 最终ACP包集成 | 33个顶层、62个子场景通过，0.624s；传输8、解码10、会话15；最后一次受影响源码冻结后运行 |
| 合计 | 56个不同顶层、163个子场景；149新增41个顶层。重复定向运行不增加数量 |
| 依赖 | 标准代理及校验和固定SDK；离线类型构建及go mod verify通过；新增一项直接依赖且SDK无额外require |
| 独立增量复核 | 修复准确RPC错误后槽位不释放、双向等待锁、旧取消影响新回合；最终历史及取消屏障增量未发现确定P1/P2 |

最终ACP命令：`.cache/toolchains/go1.26.10/go/bin/go.exe test ./internal/workbuddyacp -count=1 -timeout=90s -v`。原客户端结果来自同样参数的两包集成命令；其源码及测试摘要未变化，按项目规则复用，不重复跑OAuth。环境为Windows、Go1.26.10、CGO_ENABLED=0、GOTOOLCHAIN=local、GOWORK=off、GOENV=off、GOPROXY=off。GOMODCACHE为`.cache/go-mod-runtime`，本阶段GOCACHE／GOTMPDIR／TEMP／TMP均在`.cache/workbuddy-149`内，仅进程生效。未宣称race验证。

本机真实HTTP夹具覆盖：精确ticket／connection headers、无重定向及重试、SSE换行与断流、1MiB帧／32项背压、原连接绑定、所有固定SDK更新、跨session拒绝、未知扩展、权限变异与重放、回合关联、上下文取消与明确取消。历史夹具发送96条通知，并等待消费完成才返回load的HTTP确认；权限及未匹配回应仍留待处理。回调拒绝及取消观察到POST和SSE退出，无重复发送。

关键失败先复现再修复：`session-review-red.log`、`session-duplex-red.log`、`session-decision-red.log`、`session-cancel-fence-red.log`、`session-history-red.log`及`session-load-capability-red.log`。最后三个分别对应取消跨回合竞争、完整历史阻塞及缺省能力误调用。最终结果由`acp-stage-integration.log`证明；名字含final的较早日志不是当前最终结果。

## 证据及范围

本地日志位于`.cache/workbuddy-149/`，不入Git；摘要记录日志及当前源码SHA256。准确客户端复用`integration-tests.log`中workbuddy包部分；最终ACP用`acp-stage-integration.log`。源码清单单独固定，防止把并行工具接入／桌面改动纳入本阶段。

本阶段没有UI变化，不新增浏览器验收。前序147桌面授权与148／150团队界面证据保持各自范围。未调用真实供应方账户、模型或云任务，未验证多物理主机、其他数据库、安装包、生产部署或平台全套。

后续仍需完整交付：原DAG当前机器／owner委派代理、跨副本持久一次派发、可恢复进度与权限团队页面、初始云任务取消／停止证明、用量与预算规则、可信云产物及固定候选、独立测试和人工验收。当前SDK连接不能替代这些业务功能，不把未接入页面或planned目录标为可用。
