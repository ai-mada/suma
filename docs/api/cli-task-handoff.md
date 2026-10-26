# CLI任务交接

CLI沿用[任务交接合同](../../contracts/task-handoff.md)，以当前账号调用正式API，不授予权限、不切换接收账号、不自动执行或验收任务。全局`--control`、`--project`、`--language`放在`handoffs`前；凭据建议通过`SUMA_TOKEN`提供，浏览器凭据继续保留原界面身份参数。

```text
suma --project PROJECT handoffs list --task TASK --limit 20
suma --project PROJECT handoffs create --task TASK --version TASK_VERSION --to-resource HUMAN_RESOURCE --note "交接说明" --operation ORIGINAL_KEY
suma --project PROJECT handoffs accept HANDOFF --version HANDOFF_VERSION --reason "已核对依据" --operation ORIGINAL_ACCEPT_KEY --acknowledge-invalidation
suma --project PROJECT handoffs return HANDOFF --version HANDOFF_VERSION --reason "需要补充交付" --operation ORIGINAL_RETURN_KEY
suma --project PROJECT handoffs withdraw HANDOFF --version HANDOFF_VERSION --reason "撤回本提案" --operation ORIGINAL_WITHDRAW_KEY
```

以上大写值均须替换。`create`可选`--to-team TEAM`；`list`下一页使用响应的`next_cursor`作为`--before`，不得跨任务复用。任务版本、交接版本和接收资源均从当前列表读取，不自动刷新或猜测。

接收必须显式提供`--acknowledge-invalidation`：它会变更负责人，使原执行及验收依据失效，并失效下游验收；原运行、测试和审查保留，不创建新run。仅指定接收账号可接收／退回，仅发起账号可撤回，服务端始终重核当前权限。

写操作要求8–128字符的`--operation`和正整数`--version`。网络结果未知时保留原账号、请求路径、完整正文和同一操作标识；CLI不会隐式重试或生成另一操作。响应JSON、用户原文和数值按既有客户端合同保留；`--language zh-CN|en-US`与`handoffs --help`提供双语帮助和本地错误。
