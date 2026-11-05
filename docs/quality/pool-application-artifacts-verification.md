# 共享执行池应用制品交接验证

T-POOL-APPLICATION-ARTIFACTS-251 · 速码 1.0.0 · 2026-11-06。负责人为平台集成。[执行池合同](../../contracts/execution-pools.md#应用构建制品与独立包测试) · [应用构建操作](../development/application-release-sets.md#从应用源码版本构建)

共享池执行节点用专用的构建／包测试档案，按原授权、槽位、派发和围栏上传或读取制品。控制面继续按内容摘要和登记应用范围证明封存。另一节点下载同一清单，执行成功后独立提交包测试证明。业务验收和登记候选各自授权。

## 验证记录

| 范围 | 实际核对 |
| --- | --- |
| Pool 到发布候选 | Windows、SQLite、回环 HTTP、实际 Git 工作区和两个 Node 子进程。两个独立 Pool 身份构建、封存、下载和测试同一清单。测试账号显式验收后登记候选，候选保留确切应用版本、Git 子树和独立执行身份。 |
| 权限与回执 | 同一原解析请求恢复原回执；错误节点、槽位和授权版本被拒绝。当前授权版本变化时，拒绝使用旧回执读取制品或确认新的包测试证明；合法原操作可继续。 |
| Runner 恢复 | 5 项定向测试通过，覆盖项目构建与包测试、Pool 回执丢失后的原操作恢复、专用档案及凭证边界。项目档案继续使用原库存形状。 |
| 项目路径回归 | 5 项原制品上传与恢复测试通过，覆盖字节认证、封存与证明分离、租约失效及人工恢复。 |
| 生成合同 | OpenAPI 488 条路径与 TypeScript 合同一致，Pool 使用独立机器认证并声明完整派发标头。 |

实际联调命令为 `go test ./internal/platform -run '^TestPoolApplicationArtifactsActualBuildAndIndependentPackage$' -count=1 -v`。实际联调通过，耗时 23.13 秒。既有共享池仓库候选、独立 JUnit 与应用源码登记联调回归通过，耗时 18.23 秒。验证使用同一主机上的 Runner 执行代码和真实构建／测试子进程。跨物理主机、数据库集群、共享池制品清理和多范围构建仍需独立交付或验收。

## Multica 参考

参考本地 Multica 固定提交 `e8b6a97643ccc35efbcf2a4675c8d088a288ed6e`。其运行时领取先锁定容量再核对并发额度，避免旧观察推进当前领取。速码独立应用同类一致性原则，执行池制品访问以当前双向授权和已持有的派发为准。[Multica 运行时领取](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/service/task.go#L3663-L3706)
