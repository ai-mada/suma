# 工具未知检查的受控核对恢复

T195，初始化版本1.0.0；完整平台31／P0–P7／58限定／34待交付／92目标继续。

## 当前管理者一次条件授权

`POST /tool-inspections/{id}/recoveries` 接收 `project_id,expected_version,inspection:{inspection_id,claim_id,fence},idempotency_key`。原当前 BrowserProjectApprove 人员明确批准“核对并释放本次维护占用”；300秒、固定原引用及不可变来源、每原请求最多一个未结束核对。当前新管理者可处理原发起人已撤权的未知记录。该授权只关闭原维护占用，不恢复旧任务、原配置或旧执行权，不安装、启用、停止或重新探测。

`GET /tool-inspections/{id}/recoveries?project_id` 返回最近10项；`GET /tool-inspection-recoveries/{id}?project_id` 恢复原记录。创建重放固定原创建回执；读取恢复当前头，不换新操作标识。

## 准确机器读取、存证与持久接纳

普通前缀 `/runner/tool-inspections/recovery`，Pool为 `/pool-runner/tool-inspections/recovery`：

- POST 原路径：`project_id,worker_id,inspection`；Pool以 `tenant_id,project_id,runner_id,inspection` 替代身份。返回 `{recovery:ToolInspectionRecovery|null}`。
- POST `/proofs`：身份字段加 `recovery_id,expected_version,idempotency_key,proof`，返回核对记录 `proof_stored`。
- POST `/acknowledgements`：身份字段加 `recovery_id,expected_version,idempotency_key,proof_sha256`。节点必须先持久接纳准确proof回执，再发送本操作；原核对与原Inspection `closed_unknown` 同事务保存。

`ToolInspectionStopProof` 协议 `suma-tool-probe-stop-v1`，含准确 `inspection`、`source_sha256`、原durable collector `receipt_sha256`、`probe_returned`、`all_native_trees_stopped`、`no_pending_inventory`、owned启动的工具进程树数与已确认排空的树数（相等，0–2048；不是所有descendant进程总数）及 `observed_at`。两个计数字段都须显式提供，0树不能通过缺字段Go默认值冒称；proof:null、false事实、漏spawn或不等计数均拒。来源摘要由共享 `InspectionSourceSHA256` 对原不可变public source字段以Go标准JSON序列化并SHA256；状态、版本、当前维护instance不参与。公开proof摘要由共享 `StopProofDigest` 对全部proof字段序列化取SHA256。摘要绑定原件，不替代实际停止事实。

节点只读原durable证明，不重新探测、不kill；先恢复原待发送库存。只有原probe已返回、全部spawn意图均有覆盖、每native tree真实停止、无待发库存时方能提交。硬崩无原停止回执、Unix全树未知、漏spawn或身份无法核实必须保持未知并引导显式隔离旧身份／重新配对，禁止清空Journal或以不存在PID、超时、布尔默认值伪造停止。

核对使用当前准确节点身份／Pool route与grant及核对授权者资格；原RequestedBy撤权不阻止新授权者关闭占用。原Profile版本变化可核对旧来源而不赋予其新执行权；原节点身份撤销或Pool不再授予当前scope时拒绝并指向隔离重配对。任何旧晚到库存不得覆盖关闭后新库存。关闭tick不调用probe；后续显式重检必须另建新请求。历史原未知result code／Started事实及证明全部保留，不冒充not_started、stop或completed。

## 真实维护能力

原认证库存新增非空可选 `maintenance:{protocol:'suma-tool-maintenance-v1',instance_id}`。instance是持久Journal逻辑owner，跨同Journal重启稳定，不是PID或每次进程随机值。仅连续Worker／Pool inventory owner声明；一次report-tools、注册身份、在线或工具adapter不冒称支持。独立能力水位与当前机器身份绑定，新库存提交时写入；旧周期ACK不能续水位。原ToolProfile读取附 `maintenance` 状态 supported／expired／unsupported／unknown，必填服务器 `checked_at`；unknown只含状态及checked_at，其余状态含真实protocol／instance／observed_at／expires_at。supported为v1且expires_at>checked_at，expired为v1且expires_at≤checked_at，unsupported为合法非v1声明；不回退猜测。客户端以请求开始的monotonic计时及expires_at−checked_at扣除传输时间，不以本机墙钟或expires_at−observed_at重置期限。状态声明不等于工具、模型、native停止证明或健康。新重检要求fresh supported；核对恢复允许当前准确身份的last-known v1 owner即使expired，防止未知检查阻止新报告后又不能取得恢复许可的自锁，不为旧ACK续水位。

读取已领取／未知检查时，原source变化只增加 `source_changed:true` 非授权提示，保留claimed／unknown和原result code；未领取pending可显示superseded。只有明确写命令及真实proof／接纳回执可以落closed_unknown释放占用，界面不得把superseded投影当已停止。

当前能力是固定目的接口协商，不是新任务dispatcher或第二配置权威。新表沿原1.0.0组件基线，SQLite／OceanBase／PostgreSQL未知已有布局在DDL前拒绝，不迁移回填、不重置用户数据。Root统一共享Schema、真实Windows进程／Worker／Journal旅程、界面及Git；各owner仅新增与直接受影响检查。
