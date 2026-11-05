# 登记分支 Git 包导出验证

T-REPOSITORY-BUNDLE-EXPORT-243 · 速码 1.0.0 · 2026-11-05。 [导出合同](../../contracts/repository-integration.md#read-only-export-from-a-registered-delivery-branch) · [部署配置](../deployment/repository-integration.md#启用登记分支-git-包导出)

由管理员为专用入站分支固定同仓库基线。受信服务带确切提交、来源配置摘要和执行代次请求 Git 包。成功结果只含一个确切基线和一个可导入的 `HEAD`，附包摘要、长度与请求摘要。仓库提供方限制请求体、Git 包大小和同时导出数量，独立于合并与核对处理。

| 验证 | 实际结果 |
| --- | --- |
| Windows Go 1.26.10、真实 Git 与独立仓库 | 接收端最初只有基线。通过 Git `bundle verify`、`unbundle` 和检出取得确切来源提交并核对文件内容。提供方的裸仓库 `HEAD` 指向基线，输出仍来自明确指定的入站分支。请求摘要、包摘要和长度匹配。 |
| Git 与授权边界 | 其他服务凭据、项目不符、分支移动、基线移动及分叉均拒绝。导出前后核对源仓库引用、符号 `HEAD`、对象数量与操作记录。并发占用时返回 429。 |
| 合同与回归 | 请求严格拒绝缺失、重复、null、额外字段和混用对象格式。受管仓库原有应用、回执、核对、源码检查和 CLI 测试通过。 |

复用本项目既有受管 Git 命令及候选 Git 包约定。Git 包承载明确的引用和接收前置对象，Multica 将评审绑定到确切 PR HEAD 的做法用于设计参考。 [Git Bundle 文档](https://git-scm.com/docs/git-bundle) · [Multica 评审绑定](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/service/task.go#L1241-L1265)

本阶段使用实际 SHA-1 仓库和测试进程内 HTTP 路由。控制面正式候选准入、WorkBuddy 作者归属、不同执行节点的同提交测试、SHA-256 仓库实跑和多主机 TLS 部署仍待完成。
