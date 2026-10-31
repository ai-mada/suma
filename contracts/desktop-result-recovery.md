# 原结果回执核对与到期恢复

T-DESKTOP-RESULT-RECOVERY-179；suma1.0.0。实现原结果已提交、ACK丢失且机器凭据已到期时的完整核对入口。原31项／P0–P7目标不变；未提交副作用的专用写入恢复继续交付，不能据本片宣称全部旧执行已恢复。

## 边界与流程

当前浏览器管理员与原HMAC／持有人／登记／工作区来源验证后，**只查询**`dag_operations`中准确的原结果回执。旧token不续期、不恢复Runner写权限；不调用Complete、Claim、心跳、SDK、Prompt或普通Worker.Run。本片无需新Actor、票据表或到期例外。

主进程：明确recover → 原进程已关闭 → 私有离线prepare并加密保存原计划／操作标识 → HMAC回执查询 → 保存服务端短期核对凭证 → 私有离线apply仅切换已有结果acked → 再运行178只读检查。没有原回执不新增结果；正常续期和明确连接继续独立。

## 共享计划与凭证

共享纯类型在`internal/desktoprecoverycontract`，Runtime与平台共同使用，前端不接触计划、原结果、凭证或key。

```ts
type Plan = {
 protocol:'suma-desktop-result-recovery-plan-v1'; project_id:string; worker_id:string;
 snapshot_sha256:string;
 items:Array<{journal_key:string;run_id:string;node_id:string;attempt:number;fence:string;
   operation_id:string;body_sha256:string;result_sha256:string}>;
};
```

每批最多32项；fence是规范正十进制字符串，避免JS精度丢失。`body_sha256`核对本机原Callback序列化依据；`result_sha256`必须按原Server解析为`dag.Result`再`json.Marshal`的fingerprint计算，不能用Runner wire或HTTP envelope替代。整份主日志规范化保留json.Number，只把计划目标acked归为false，其他字段全部参与snapshot摘要。

服务端`Proof`为闭合对象：`protocol='suma-desktop-result-recovery-proof-v1',scope,plan_sha256,issued_at,expires_at,items,signature`；scope为原project/user/tenant/worker/adapter/instance/binding/base/registration_operation_id/credential_sha256，items为`{journal_key,operation_id,status:'matched'|'missing'|'mismatch'|'source_changed'}`。期限60秒，HMAC-SHA256使用原workspace私钥及独立domain，签名对signature置空后的Go JSON编码。凭证仅证明原回执事实，不授予任何网络或执行权限。

私有`POST /desktop-runners/recovery/receipts`请求为`scope,recovery_operation_id,plan`（三字段闭合；scope字段名由共享类型定义），按原workspace key签名；HTTP200返回Proof。当前授权在查历史前完成。回执键为tenant/project/`result:<run_id>`/operation_id，另核历史attempt的Worker／attempt／fence，不要求旧lease仍活。matched不代表任务成功、当前head相同或副作用已排空；178门禁继续有效。

## 私有本机入口

`--desktop-runner-result-recovery-stdin`：`{action:'prepare'|'apply'|'status',state_directory,project_id,worker_id,plan?,proof?,proof_key?}`。路径／key仅主进程构造；输入最多256KiB。prepare不改原件；apply在原锁内重核完整snapshot、每项身份／正文摘要和凭证，clone后一次原子保存，只修改matched项的acked，不OpenJournal、不循环Ack(id)。status只读取原计划的恢复情况。

ACK写入后响应丢失可依据同计划识别已确认项；只允许计划中准确项acked从false到true，其他差异均阻止应用。过期凭证不能写入；当前会话授权下可对同计划重新查询原不可变回执。missing／mismatch保留本地原件，unknown结果即使运输ACK补齐仍保持unknown。其他sidecar不改；不把OpenClaim、workspace replay、云／发布恢复当作只读回写。

Runtime返回闭合`{protocol:'suma-desktop-result-recovery-v1',plan?,total,recovered,remaining,reasons}`，reasons固定为`no_saved_results,receipt_missing,receipt_mismatch,unsupported_pending,state_changed,source_changed,access_changed,invalid_state,active_worker,result_unknown,proof_expired`。

## 桌面IPC与简洁界面

原`manageLocalTool`新增`recover`与`recovery-status`，只接受原scope／locale。前者明确开始或继续原计划的核对，后者只读取原状态，不能创建新计划或改走connect。原renewal pending保留；续期结果未知必须先核对原续期，不能借恢复绕过可能已更换的凭据。

原connection可带安全`recovery`：`{operation_id,phase:'prepared'|'checking'|'unknown'|'complete'|'partial'|'blocked',total,recovered,remaining,reason_code?}`。均为非负安全整数，不返回计划／key／正文／路径。主生命周期仍表达凭据状态；恢复是独立事实。正在处理或结果未知阻塞父切换／登记，原状态核对按钮始终可达，不反锁自己。

单一恢复卡显示原结果数量、已核对／仍待核对与具体下一步；不自动续期、启用或连接。中文英文完整。仅定向验证当前授权、历史source、签名／期限、原snapshot、原子ACK与无执行边界；复用未变SDK，不跑无关全套。实际CLI与合成界面各自留证。
