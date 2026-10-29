# WorkBuddy云任务协议与持久提交验证

阶段：T-WORKBUDDY-143；产品1.0.0；2026-10-30。范围为尚未开放的内部HTTP客户端、Journal检查点和单次提交恢复组合，不等于完整可派发的云执行适配器。独立代码基于已提交的86fb65e；本工作区139–142及桌面改动分别保留。

## 实现与边界

依据[WorkBuddy官方Open API](https://open.workbuddy.cn/docs/openapi)，`internal/workbuddy`实现准确POST创建、GET原任务和分页读取，使用请求字节摘要、固定origin、明确响应大小与类型校验。返回投影只保留任务事实，丢弃ACP凭据和连接／沙箱地址；未知提供方status不映射为速码完成。

`workbuddy_journal.go`复用原Journal，绑定完整Assignment、请求摘要、凭据引用版本和端点。首次持久prepared才授予一次提交资格；重复及重启后的prepared保持待核对，task_known仅查询原ID。写入失败回滚内存，磁盘与内存出现分歧则停止并要求重开Journal。原终端、模型和制品通道不会与云提交混用。

组合helper先读原记录，后决定是否准备新提交，保留已知任务在原Runner未知结果后的只读核对。提交响应丢失、保存返回ID失败、请求／凭据漂移均不能触发第二次POST。查询失败仍返回原任务ID。未修改Execute、能力发现或工具目录，没有新增公共API／界面及派发资格。

领域规则见[合同](../../contracts/workbuddy-cloud-submission.md)，实施与后续接线见[架构](../architecture/workbuddy-cloud-submission.md)。本阶段没有复制Multica或厂商实现，没有新依赖。

## 实际检查

去重27个顶层Go用例通过，子场景不重复累加：

- `go test ./internal/workbuddy -count=1 -v`：8项、0.219秒。覆盖实际本机HTTP方法、准确路径与状态码、鉴权、请求摘要、身份、分页一致与余量、超时、丢失响应、畸形／重复／超限JSON、重定向及秘密边界。日志 `.cache/workbuddy-143/protocol-tests.log`。
- `go test ./internal/runner -run "^(TestWorkBuddy|TestModelGatewayJournal|TestTerminal.*Journal|TestJournal)" -count=1 -json`：14项、0.539秒。包括9项新Journal、4项组合HTTP／持久恢复及1项既有模型网关恢复。覆盖20并发单次准备、完整身份／Pool漂移、重启、已知ID不变、存盘失败、磁盘／内存分歧、损坏拒绝、响应丢失和原ID查询。日志 `.cache/workbuddy-143/journal-tests.jsonl`。
- 5项受影响的既有终端恢复回归通过，7.621秒：原停止证据、异attempt拒绝、未知结果、过期结果处置及捕获来源恢复。精确命令与用例清单位于 `.cache/workbuddy-143/journal-summary.json`，原始日志 `terminal-regression.jsonl`。
- 本阶段文件空白检查通过。没有更改UI，不重复前端构建或浏览器检查。139–142的界面待验状态不因这些测试改变。

环境：Windows、仓库内Go1.26.10，GOTOOLCHAIN=local、GOWORK=off、GOENV=off、CGO_ENABLED=0、GOPROXY=off；模块缓存 `.cache/go-mod-runtime`，临时目录 `.cache/workbuddy-143/tmp`，复用进程局部的既有Go构建缓存，不改全局环境。不提交缓存、凭据、数据库或原始日志。以上是本机协议与恢复证据，没有真实厂商任务、账户、物理多机或生产验收。

## 后续完整接入

继续实现正式OAuth授权及授权实例、当前资格与秘密托管、ACP进度和权限请求、用量与预算能力、准确云任务核对、产物导入与固定Git候选、独立测试及人工验收。创建幂等、取消与停止证明按实际提供方合同逐项验证；当前不臆造这些接口。完整平台和原实施计划继续在途。
