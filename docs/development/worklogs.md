# 工时操作与AI开发SOP

状态：T-WORKLOG-135；负责人：计划领域／工作台。关联[合同](../../contracts/worklogs.md)、[架构](../architecture/worklogs.md)与[证据](../quality/worklogs-verification.md)。

1. 当前人员进入开发→计划→工时，核对项目、所选周和本人资源。按具体任务登记日期和分钟，填写必要说明。记录来源始终是人员登记。
2. 管理者代办选择真实人员，明确代办确认；记录修改者和被登记人员。CLI项目API只能登记当前账号本人，不能凭客户端参数扩大权限。
3. 更正先读取当前头和当前任务版本，保留人员身份，填写完整替换字段及原因。错人作废后新建；作废不删除原件。
4. 结果未知保留原操作标识、正文及原页面身份，明确重试同一操作；不要改正文或生成新键。403核对当前身份；409保留草稿并读取当前依据。列表游标过期从第一页重载，不拼接旧页。
5. 不把登记分钟写入任务验收、执行额度或供应商账单，不自动将估算和运行日志转换成人员工时。智能体可以解释和辅助准备输入，不能代替人员声明。

CLI使用现有`--control`控制面地址、`SUMA_TOKEN`凭证及`--project`全局参数；具体旗标以`suma worklogs --help`中英帮助为准。示意：

```text
suma --project PROJECT worklogs list --from-date 2026-10-26 --to-date 2026-11-01
suma --project PROJECT worklogs create --task TASK --task-version 3 --member HUMAN_RESOURCE --date 2026-10-29 --minutes 90 --note "需求设计" --operation EXPLICIT_UNIQUE_KEY
suma --project PROJECT worklogs correct ENTRY --version 1 --task TASK --task-version 3 --date 2026-10-29 --minutes 60 --note "扣除休息" --reason "核对后更正" --operation ANOTHER_UNIQUE_KEY
suma --project PROJECT worklogs history ENTRY --limit 20
suma --project PROJECT worklogs void ENTRY --version 2 --reason "重复登记" --operation VOID_UNIQUE_KEY
```

开发定位：`internal/platform/worklog_*`负责权威、生命周期、整数台账和分页；`cmd/suma-contracts/worklog.go`从同一Go DTO生成线Schema；`internal/cli/worklog.go`只负责明确输入和复用HTTP；`WorklogsView/Editor`使用现有写操作及离开保护。新语言增加配对locale域和后端精确消息，用户说明原文不翻译。

验证只运行本轮受影响的HTTP／SQLite／CLI／合同、工时组件和必要计划回归，再做类型／语言／隔离构建检查。保留初次失败及修复证据，真实浏览器／其他数据库的未测范围明确列出；不重复全平台测试，不把源码完成当生产验收。
