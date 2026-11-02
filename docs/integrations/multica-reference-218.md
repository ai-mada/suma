# Multica 参考基线与本阶段采用范围

状态：T-TEAM-WORKSTATION-RECOVERY-218，限定设计参考。负责人：主集成／Runtime。关联：[恢复合同](../../contracts/team-workstation-recovery.md)、[需求与 AI SOP](../product/team-workstation-recovery.md)、[验证报告](../quality/team-workstation-recovery-verification.md)。

## 固定版本

本阶段通过官方 GitHub ref 获取 main，锁定提交 [`a7a61f15f148ba8c901fe24a36a61d6d73e247d6`](https://github.com/multica-ai/multica/tree/a7a61f15f148ba8c901fe24a36a61d6d73e247d6)。提交日期为2026-11-03T00:26:13+08:00。随后仅获取该提交对象；`D:/codes/github/multica` 原 HEAD `e8b6a97643ccc35efbcf2a4675c8d088a288ed6e` 与工作区状态保留，没有切分支、覆盖本机修改或合并参考仓库。

读取以下路径，并以原始 Git blob 比较已审基线与最新提交，8份均无变化。工作区 CRLF 与 Git LF 的差异不视为代码变化。

| 参考路径 | 采用方式 |
|---|---|
| `LICENSE` | 核对完整许可及附加条件。 |
| `apps/desktop/src/main/daemon-quit.ts` | 参考单一退出协调者、忙碌状态与默认取消；使用速码原有服务 owner 实现。 |
| `apps/desktop/src/main/daemon-quit-dialogs.ts` | 参考未知活动和取消失败的明确处理；速码使用独立中英文离开确认。 |
| `apps/desktop/src/main/daemon-manager.ts` | 参考桌面生命周期边界，不增加第二个节点管理者。 |
| `server/internal/daemon/shutdown.go` | 参考停止领取后等待既有任务收尾，保留速码原 stop／Journal 事实。 |
| `server/internal/daemon/daemon.go` | 核对原有执行生命周期分工。 |
| `server/cmd/multica/cmd_daemon_shutdown_result.go` | 参考退出结果与实际停止分别呈现。 |
| `server/cmd/multica/cmd_daemon_stop.go` | 参考明确停止流程，不以 UI 关闭冒充远端进程停止。 |

## 实现与扩展边界

本阶段未复制 Multica 源码或界面。速码继续使用原有组织所有权、双边 Grant、准确工具版本、持久 Journal 和当前授权；桌面使用 Electron 官方事件与对话框 API。共享节点恢复新增一个受当前所有者权限约束的准确读取接口，复用原配置准备、连接和换新合同。

`Multica License` 由 Apache 2.0 正文及附加条件共同组成，不能标为纯 Apache 2.0。附加条件涉及向第三方提供托管服务、商业嵌入、品牌和归属信息等；后续若采用实际代码，必须按锁定版本的完整[许可](https://github.com/multica-ai/multica/blob/a7a61f15f148ba8c901fe24a36a61d6d73e247d6/LICENSE)逐项登记采用文件、修改、通知及所需授权。本阶段没有因参考设计引入新依赖或转移平台执行权。

## 可追溯记录

本地忽略目录 `.cache/team-workstation-recovery-218/` 保存 `reference-latest-ref.json`、`reference-fetch.json`、`reference-latest-code.json`，分别记录远端引用、准确对象获取及逐文件摘要。源码路径摘要如下；验收报告保留这些记录的 SHA-256，临时 Git 对象与原文件不提交到速码。

| 文件 | Git blob SHA-256 |
|---|---|
| `LICENSE` | `ec78c4feacec52c7dad8ed5ffdd79033fc8c58e320a3c5ebe9a00286cab37f63` |
| `apps/desktop/src/main/daemon-quit.ts` | `804e057603151f9c68d7bde354a5a600584e4084e2d906859bee2d3ec556424e` |
| `apps/desktop/src/main/daemon-quit-dialogs.ts` | `417eaf1b37df824a30aeb8f06dc98b868037bf1375ff19cae4b8845f1501e365` |
| `apps/desktop/src/main/daemon-manager.ts` | `8cd7aaf6ec5f94d397fc6a3930c079c4751d7d6af28a2d0de629432e745af371` |
| `server/internal/daemon/shutdown.go` | `0e5412bc6e2d57e9ebaed9ac6443d447bfe363e53f74351ec29b13af6064266e` |
| `server/internal/daemon/daemon.go` | `7f070fd54e4b61a246441edf5e8d5bcf77360e48ce8cfb039747985dd1a43e92` |
| `server/cmd/multica/cmd_daemon_shutdown_result.go` | `3efb6ba3a83f5110b3c7a010c0e00e4f3ccc96cfa4f06c52e3ab929dd83337fe` |
| `server/cmd/multica/cmd_daemon_stop.go` | `a2c29f99c2c38cb83f788d1ddfe9c23cb72dd9982a5301c24001626adc27c1e6` |
