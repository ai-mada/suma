# 本机静态发布提供方

本页说明 P5 的本机 `suma.release.v1` 接入。功能以实际验证记录为准；配置存在不表示已部署或通过健康。合同见 [发布执行](../../contracts/release-execution.md) 和 [发布控制面](../../contracts/release-control.md)。本提供方不依赖 Docker，不运行上传文件或任意发布脚本，不连接外部目标或模型。

## 进程与管理员配置

控制面登记固定 provider ID、tenant/project、允许 site ID、独立 provider 控制凭证、管理 origin 和只读 reconcile 凭证。Runner 使用登记并绑定 worker 的独立凭证及固定命令 profile；旧共享项目 Runner token 不能替代可信发布身份。控制面生成管理执行 run/node、计划、代数和授权，普通用户流程不能自行提供这些权力。

提供方管理员 JSON 示例；凭证只填环境变量名，实际值通过当前进程秘密配置提供：

```json
{
  "provider_id": "local-static",
  "root": "D:\\project\\.cache\\release-provider",
  "management_address": "127.0.0.1:4350",
  "control_url": "http://127.0.0.1:4321",
  "control_token_env": "SUMA_DEPLOY_PROVIDER_TOKEN",
  "reconcile_token_env": "SUMA_DEPLOY_RECONCILE_TOKEN",
  "workers": {"release-worker": "SUMA_DEPLOY_WORKER_TOKEN"},
  "sites": {"preview-site": {"address": "127.0.0.1:4351"}}
}
```

```powershell
suma-deployer --config D:\project\.cache\provider.local.json
```

提供方根必须是管理员 `.cache` 中专属子目录，路径不得穿过链接。管理和站点只监听明确的回环 IP；站点测试可使用端口 0，真实配置应固定端口。每个 site 绑定一个环境；provider.db、生成文件和独占进程锁留在专属根内，启动不会复制、切换或覆盖用户源码 checkout。

Runner 的 `--external-config` 采用相同 provider ID；这是受信管理员文件，不是流程输入：

```json
{
  "local-static": {
    "url": "http://127.0.0.1:4350",
    "protocol": "suma.release.v1",
    "token_env": "SUMA_DEPLOY_WORKER_TOKEN",
    "poll_milliseconds": 500,
    "cancel_wait_milliseconds": 5000
  }
}
```

`SUMA_DEPLOY_WORKER_TOKEN` 是提供方识别固定 worker 的传输凭证，与 Runner 对控制面的 worker 凭证、provider 对控制面的凭证和只读 reconcile 凭证分开。它单独不能部署：Runner 从当前 managed assignment 的独立 HTTP 调用领取短期 opaque ticket，提供方向固定控制面在线 consume，并在激活前 begin-effect。ticket 只在内存传递，不进 Config、Assignment、journal、provider intent 或日志。

发布或回滚排队后，如页面显示上报缺少发布能力，请核对专用项目 Runner 的配置键是否与环境绑定的 provider ID 相同，再核对 `protocol`、固定 URL 和进程传输凭证是否已加载。未匹配前，运行保留在队列。Runner 下一次新领取上报匹配能力后即可继续参与调度。使用普通外部流程的同名插件无需填写发布协议，`workbuddy-cloud` 保留给 WorkBuddy 云任务。

## 构建与包测试 profile

命令许可表增加 `artifact_output` 或 `artifact_input`，二者互斥。以下仅演示字段结构；可执行文件和项目内构建/检查程序须由管理员实际准备、审查并登记同一 profile 摘要：

```json
{
  "build-site": {
    "executable": "go",
    "args": ["run", "./tools/build-static.go"],
    "read_only": false,
    "artifact_output": ".cache/release/site"
  },
  "test-package": {
    "executable": "go",
    "args": ["test", "-count=1", "./packagecheck"],
    "read_only": false,
    "test_evidence": true,
    "artifact_input": true
  }
}
```

固定源码交接示意为 `code → build → package-test`：build 的 `config.candidate_from` 指向 code，package-test 的 `config.artifact_from` 指向 build。构建和包测试都保持固定提交 C，工作树独立；包测试从服务端冻结绑定取得真实字节，校验后由进程局部 `SUMA_ARTIFACT_DIR` 指向其输入目录，管理员不能用同名 env 覆盖它。C 必须存在于本机仓库且是 adminBase 的后代，缺 object 失败，不 fetch 或回退 B。

输出只有静态 manifest 和普通文件，最多 128 个、单文件 16 MiB、合计 64 MiB。禁止链接、特殊文件、路径穿越、Windows ADS/设备名及大小写冲突。构建真实进程成功且源码干净后，Runner 保存实际输出快照，再上传和 seal；封存只证明字节存在。控制面从当前受信成功尝试、workspace evidence、实际 seal 和 profile 派生构建证明，从固定包输入的真实测试派生包测试证明。手工登记、上传文件、source_ref 或填写 passed 都不能生成这些证明。

上传前会保存 manifest、内容摘要和成功进程结果。封存成功而响应丢失时，Runner 重开 journal 后用原操作恢复上传和最终结果，不重新构建；没有执行完成 checkpoint 的崩溃仍保持未知。恢复期间继续心跳；失效归属不能通过新建 ID 恢复权限。

## 真实激活、健康和回滚

提供方从固定制品服务读取授权计划的 manifest 和字节，重新验证全部摘要，物化到专属生成目录。在本地 SQLite 事务中，generation CAS、active revision 和操作回执共同更新；HTTP 从同一个 active 快照选择完整文件。站点只返回 manifest 中的文件，读取时再次校验实际字节。

健康检查真实 GET 固定站点内容并核对状态、内容 SHA、`X-Suma-Generation`、`X-Suma-Artifact-SHA256` 和保留标记 `/.suma-release`。200 响应或进程存活单独不能通过健康。已切换而健康失败保留 applied/failed 的真实组合，不能显示未部署。

回滚仍走新计划、审批、operation 和更大的 generation，绑定真实历史健康制品；不是执行上传内容中的 undo。提供方不会自动回滚。`GET /v2/operations/{operation_id}` 支持原 worker 与独立只读 reconcile 身份；取消 ACK 不表示停止，旧代数不能覆盖新 active。未知结果在控制面保留环境屏障，不能以租约到期或手工填写成功跳过。

消费授权的响应丢失或提供方重启时，提供方用固定控制面 `/provider/deployment-authorizations/inspect` 找回同一绑定的历史消费回执，再按当前授权规则恢复。若确认尚未消费，Runner 可为原操作取得新的内存 ticket；不能换 operation 掩盖旧结果。启动时无法连接控制面、且本地尚无消费回执的操作保留 reconciling，需恢复连接后重新核对或重启；当前没有自动越过未知操作的流程。

此本机实现采用可信管理员进程和普通文件系统隔离，不是操作系统沙箱或供应链签名。跨机器发布、对象存储、生产审批、证书签名、容器、真实外部健康及断电存储恢复需要分别实施和验收，不能从本机 fixture 通过推断。

## 本轮执行验证（2026-10-24）

Windows amd64、Go 1.24.5、CGO 关闭；执行 `go test -json ./internal/runner ./internal/deployer ./cmd/suma-runner ./cmd/suma-deployer -count=1` 及相同四包的 `go vet`，均退出 0。原始记录有 65 条测试通过事件，包含父测试、子测试和子进程 helper 入口，0 失败或跳过；两个命令包仅编译，没有独立测试用例。测试及构建缓存、临时文件均在仓库 `.cache`，依赖代理和校验服务关闭。

覆盖真实隔离 Git C 构建及包测试、实际制品字节上传下载、seal ACK 丢失后的 journal 重放、包被改动/坏摘要/缺对象拒绝、同操作重复请求、真实静态 HTTP A/B/A 及新代数回滚、错误内容 200 的健康失败、消费 ACK 丢失和 active 事务提交后的真实子进程退出/重启。提供方测试使用本机控制协议 fixture；完整平台权限、正式业务验收和真实浏览器发布由独立集成证据记录，不包含在这 65 条中。

- [原始测试记录](../quality/evidence/release-execution-tests.jsonl)，[SHA-256](../quality/evidence/release-execution-tests.jsonl.sha256)
- [测试前后源码摘要清单](../quality/evidence/release-execution-source-sha256.json)，[SHA-256](../quality/evidence/release-execution-source-sha256.json.sha256)
- [命令、环境、二进制摘要与验证边界](../quality/evidence/release-execution-verification.json)，[SHA-256](../quality/evidence/release-execution-verification.json.sha256)

原始记录 SHA-256：`d997b012188fbbf5987c368e2107e3b253f51c90b33149b4c93049c3a1739dff`。41 个相关 Go 文件在测试前后及最终备用二进制构建时摘要一致。备用程序位于 `.cache/bin/suma-runner-release-final.exe` 和 `.cache/bin/suma-deployer-release-final.exe`，本轮未覆盖正在运行的 release 程序或此前跨平台/桌面产物。
