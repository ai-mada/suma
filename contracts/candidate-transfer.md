# 跨执行节点的固定候选代码传递

状态：T-P4-018 已实现合同，限定本地验证，2026-10-26。负责人：平台集成。范围包含普通项目Runner和共享资源池独立机器身份，不改变业务验收语义。

使用Git原生bundle传递管理员base到固定head的对象，单包≤64MiB。生产者只在command/agent已停止、workspace证明clean/completed、成功且head!=base后生成。base==head、只读／测试节点不强制上传。候选源和消费节点仍遵循既有candidate_from、不可变修订及lineage校验。

## 共享接口

权威类型在internal/candidatecontract：AssignmentRef保留普通attempt/fence/worker，以及pool的tenant/project/pool/runner/slot/grant/version/credential/version/dispatch/state/lease完整回调身份；Binding保留project、source、base/head、workspace evidenceSHA、bundleSHA、bytes和sealed状态。

普通命名空间 `/api/v1/runner/candidate-transfers` 与机器命名空间 `/api/v1/pool-runner/candidate-transfers`：

1. POST集合：StartRequest → Binding(state=uploading或已sealed)，201。
2. PUT `/{id}/bundle?project_id=...`：精确ContentLength、X-Suma-Operation-ID、X-Suma-Assignment(base64url JSON AssignmentRef)，200 acknowledged。
3. POST `/{id}/seal`：CommandRequest → Binding(sealed)，200。
4. POST `/resolve`：CommandRequest，以当前consumer读取DAG已固定候选 → Binding，200。
5. GET `/{id}/bundle?project_id=...`：相同AssignmentRef头及operation头，只有当前匹配consumer能读，application/octet-stream。

JSON命令只接受声明字段，普通和pool的字段不得混用。pool的JSON允许Client.poolPayload既有tenant/project/runner_id包装；二进制不走该JSON包装，直接携带完整AssignmentRef。pool请求在human Authenticate之前按独立machine authority处理，不将machine变为Principal。

## 权威、幂等和存储

原身份、当前权限／机器凭据、项目路由、当前lease/fence和用途检查必须先于任何成功回执重放。source只允许当前command/agent，不允许test或release冒充代码生产。普通Runner不能剥掉pool字段读取pool run；pool消费需原dispatch及当前双边批准仍允许新数据读取。撤回授权可继续停止／回报事实的既有规则，不自动授予新的候选代码访问。

Start固定唯一tenant/project/run/node/attempt/fence及manifest；同操作同正文恢复，漂移冲突。 在当前权威和原操作核对后，Start恢复可投影同一transfer已单调推进的sealed状态，全部不可变依据及owner必须一致；原uploading操作回执不被改写。PUT先授权，在事务外有界写入安全临时文件、SHA／长度核对并同步，末次授权仍有效才发布；读亦在锁外有界读取，回传前再次核验当前授权和binding。所有原件以内容摘要定位，路径只来自管理员DataDir和服务端ID，禁止用户路径／URL和链接逃逸。

seal早于source结果提交，不等于任务成功。resolve与GET必须同时证明源节点当前succeeded、原attempt/fence、当前workspace base/head/evidenceSHA与封存一致，及consumer的dag_candidate_revisions完全对应。源重试、撤权、旧fence或来源失配不得通过历史receipt绕过。

## Runner持久恢复

source生成完成后，先将原成功Result及固定bundle路径／摘要和上传操作写入Journal CandidatePending，再上传／seal。断网或ACK丢失只重传同包，不再次运行代码；重启保留该pending，持当前lease心跳恢复。租约已丢失或取消时停止新增传输并保留核对事实，不将未知外部执行声明为新成功。结果ACK仍走原持久结果流程。

consumer仅在固定head对象缺失时resolve／下载；已有对象继续本地验证，不做无意义传输。下载核对manifest、字节、SHA与固定来源；以Git bundle verify和严格header检查确认单HEAD与base prerequisite，再unbundle到管理员仓库对象库；不fetch远程、改refs、checkout用户分支、运行hooks或自动merge。随后重新验证head对象及管理员base lineage，创建独立测试工作树。

source workspace证据追加 `candidate_bundle: Binding`；consumer实际导入后追加 `candidate_import: ImportReceipt`。这些传递元数据不改变原workspace evidence摘要合同，也不证明测试、评审或发布成功。界面在既有证据中紧凑显示，缺导入receipt不得推断已经传输，不提供人类下载入口。

验收至少包括两份只共享base的独立Git仓库：A真实开发C、上传，B导入并测试恰好C；中断后恢复不重执行；hash／base／head／scope／fence／撤权与重放边界；project与pool两种身份。只运行本轮相关检查，原017预览不改动。原始包、DB、凭据、程序、截图和日志均留忽略目录。
