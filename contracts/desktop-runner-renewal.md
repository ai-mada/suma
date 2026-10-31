# 本机工具凭据续期

T-DESKTOP-CREDENTIAL-178；suma1.0.0。原执行、配置、质量门禁和完整31项／P0–P7目标不变。本片解决原Worker已排空、凭据到期后无法沿原配置重新连接的问题，不把凭据恢复当作任务重跑。

## 入口与事实

- `manageLocalTool` 新增 `action:'renew'`，只接受原身份／项目／工具／locale，不接受新配置、secret或路径。原 `connect/status/stop` 保持独立。
- 原 `connection` 新增可选 `credential_expires_at:string`、`credential_version:number`，来源为Worker登记／续期回执及当前读取，UI明确是凭据登记期限；原 `expires_at` 仍是库存观察期限，不得混用。
- 新阶段 `renewal_unknown` 表示原续期结果未定；该状态只核对原续期，不能改走connect或生成另一个secret。新登记结果未知必须先恢复原登记。
- `connection.renewal_reason` 为可选安全原因：`execution_pending`、`local_state_pending`、`credential_changed`、`access_changed`、`context_changed`、`result_unknown`。用于区分已知拒绝与未知传输，不返回私有路径；重启后未知原因只可降为`result_unknown`，不能据此扩大恢复权限。
- 只能在原进程已关闭、原来源未变且本机与服务端均无待核对执行时续期；停止未确认、解密失败、来源变化、已撤销凭据不得静默恢复。未知操作、日志、fence和制品证据不得清除。

## 原子续期合同

`POST /desktop-runners/renew` 使用当前浏览器管理员和原HMAC签名。闭合请求：

```ts
{
 project_id:string; user_id:string; tenant_id:string; worker_id:string;
 adapter_id:string; instance_id:string; expected_binding_sha256:string;
 base_commit:string; registration_operation_id:string;
 expected_worker_version:number; previous_credential_sha256:string;
 credential_sha256:string; idempotency_key:string;
}
```

`registration_operation_id` 指原 `desktop-runner-<32hex>`；新操作为 `desktop-runner-renew-<32hex>`。主进程先生成新token，把原正文、key和新token原子加密保存为pending，再发送摘要。服务端不生成或返回secret。

响应复用 `DesktopRunnerRegistration` 的安全结构，operation_id为本次续期；Worker保留id/worker_id/profiles/deployment/created_at，更新version及24小时期限。服务端先核当前资格、原登记来源及持有人，再恢复原body/key回执；新操作才做旧digest、版本、未撤销状态、工作区和排空检查。原子撤销旧authority/slot、发行新摘要并更新原Worker行；旧token立即失效。不得借通用发行接口在重复恢复时再次生成secret。

原登记请求和配置保留。ACK后主进程原子采用新token／Worker回执并保存准确续期来源，原登记正文仍能核对；普通重启不改变新secret。成功回到prepared，用户明确连接后才由原Worker重放合法已知回执、发布新库存并进入connected；不自动启用工具或派发任务。

## 本机状态检查

新增私有离线入口 `--desktop-runner-renewal-check-stdin`，输入 `{state_directory:string}`，闭合输出：

```ts
{protocol:'suma-desktop-runner-renewal-check-v1',safe:boolean,reasons:string[]}
```

目录仅由主进程构造。复用原Runner日志和锁，非破坏性检查实际会影响旧执行／回执的主日志、claim、inventory、云／制品／权限等sidecar；仅主journal的pending=0不足以宣称安全。活动锁、已知未结状态、损坏、未知或不可核实来源均返回safe=false，保留原件。没有创建任何执行的空目录可安全检查；不打开数据库、不删除／重写日志、不发送Prompt。主进程据此阻止换凭据后丢失原回放依据。

服务端另外核旧Worker running／reconciling尝试和仍有效的执行／副作用资源等正式未结事实；租约过期不是停止证明。拒绝返回明确错误，原pending正文可在原核对完成后继续恢复，不自动换key绕开门禁。本片不声称解决“旧授权已过期且尚有旧token绑定未结副作用”的专用恢复票据流程；该全平台能力继续交付。

## 简洁交互与验证

连接卡单独显示凭据登记期限和原续期结果，停止后可明确续期，结果未定显示“核对原续期”；失权／撤销与到期不混淆。Qoder／Codex等叶子向父传递busy／unknown，停止后同配置重连重新读取库存；不丢草稿、不借选择器绕开原操作。中文和英文成对。

只验证新的权限、原body/key、旧token隔离、CAS、排空和故障恢复，复用未变SDK与已通过检查；一次统一类型／语言检查和必要代表界面。实际SDK／厂商／多机／数据库／安装与原完整平台范围分别记录，不用限定检查宣称全目标完成。
