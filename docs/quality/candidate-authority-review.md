# 候选交接与验收权限独立复核

日期：2026-10-23。范围：DAG 候选编译、派发、尝试回执、条件与补偿路径，以及业务验收中的工作树证据和当前项目权限。本轮独立复核不操作浏览器、不调用模型或外部服务。复核代理仅新增 `internal/platform/transaction_authority_test.go` 和本文；业务修复由主集成代理完成。

## 已复现并复核修复

| 问题 | 原行为 | 修复后的独立结果 |
|---|---|---|
| 验收权限在事务外被撤销 | 真实受邀 reviewer 的权限读出后，owner 经正式授权接口将其降为 viewer；原调用仍把任务改为 accepted | 返回 403，任务保持 in_progress；成功验收回执也不得向已撤权调用者重放 |
| Stopped 绕过成功候选校验 | 非 cancelling 运行提交 `Stopped=true` 和错误测试提交，可进入 succeeded | 错误候选返回 invalid，运行保持 running，不消耗回执 |
| 提前满足的 any_success 汇合 | 候选源仍 running 时，已 ready 的测试被 Claim 直接判 failed | 测试保持 ready 等待候选源，未提前派发或失败 |
| 固定候选的来源字段不一致 | 正确代码提交配上错误 source node/attempt/fence/digest，仍被 Complete 接收 | 来源与持久化冻结修订不符时返回 invalid |

候选内容测试使用独立 SQLite 中的合成 Runner 结果，只用于证明服务器状态机和验证分支；它不证明实际代码命令、Git 合并、生产环境测试或人工签收已经完成。验收权限探针使用正常初始化、真实邀请账号和正式项目授权变更，未通过 SQL 伪造账号或角色。用于权限回归的单测试节点保留已声明的非代码测试语义，不能作为代码或合并证据。

永久回归使用两条独立 SQLite 连接和测试驱动屏障覆盖三个时点：首次授权读出后撤权、短回执事务结束后撤权，以及成功验收回执在撤权后重放。角色变更由 owner 通过 `SaveProjectAccess` 提交，之后原审核请求必须返回 403，不得新增验收记录。正常授权状态下的首次验收和回执重放仍须通过。

已复核正式写入事务先取得组织锁并重新加载真实成员/项目审核权限，再读取操作回执和取得 DAG 锁。短回执事务在读取运行证据前结束，避免持有 SQLite 写事务再调用会自行写入的运行读取方法。

## 验证记录

进程环境为 `GOCACHE=D:/codes/ai-dev-os/suma/.cache/go-build`、`GOTMPDIR=D:/codes/ai-dev-os/suma/.cache`、`TEMP=TMP=D:/codes/ai-dev-os/suma/.cache/test-tmp`，未修改全局环境。

- `go test ./internal/platform -run 'TestReviewRechecksAuthorityAfterConcurrentProjectRoleRevocation|TestReviewSuccessReceiptCannotReplayAfterConcurrentRoleRevocation' -count=1`：通过，0.724 秒。
- `go test ./internal/dag -run 'TestCandidate|TestCompensation|TestSubworkflow' -count=1`：通过，0.933 秒。
- `go test ./internal/platform -run 'TestReviewRechecksAuthority|TestReviewSuccessReceiptCannotReplay|TestReviewRejectsWorkspaceCandidate|TestRunnerToReview|TestRunStartReplays' -count=1`：通过，1.275 秒。

未重复执行主集成代理负责的 Go 全套测试或浏览器验收。PostgreSQL/OceanBase 实际集群未在本轮运行。

新增永久回归文件 SHA-256：`6c9217fa75faf3ff567e24963c90120ec2a74c507acb5296ea9047f20ecccefb`。

## 取消中的未知效果

独立探针还复现过：运行已进入 cancelling 时，`Complete(Stopped=true, OutcomeUnknown=true, ExitCode=-1)` 被取消分支覆盖成 node/run 均 cancelled。主集成代理新增永久回归并限制取消覆盖条件后，本代理复跑同一探针确认：节点保持 reconciling、运行保持 cancelling，未将外部效果未知伪装为最终取消。进程停止和效果核实仍是两个独立事实。

上述修复完成后停止拓展审查。当前源码摘要、无凭证的详细探针结果和验证范围统一记录于 `docs/quality/evidence/governance-planning-review.json`。

忽略目录中的复现来源与详细结果分别为 `.cache/candidate-review-probe.go`、`.cache/candidate-review-probe-result.json`、`.cache/review-permission-race-probe.go`、`.cache/review-permission-race-probe-result.json`。测试夹具为独立数据库，清理前均记录文件清单和 SHA-256，不包含用户正式运行数据库。
