# 远程工作区保存 AI SOP

状态：T-P4-038／1.0.0，2026-10-27。负责人：当前项目操作人、Control／Runner运维。字段见[合同](../../contracts/workspace-write.md)，内部边界见[架构](../architecture/remote-workspace-write.md)。本地GUI与真实联合范围见[限定报告](../quality/workspace-write-verification.md)，不等同生产验收。

## 正常操作

1. 核对真实当前用户、项目和运行目标。目标必须是仍运行的准确原生 attempt；`can_write`只反映当前写权限／任务资格。Pool需要当前双边授权和准确机器凭据。
2. 用只读 `read` 的 `full_file:true`获取完整现有UTF-8文件，保留相对路径、scope、实际 policy SHA 和完整原字节SHA。原新文本均≤48KiB；分页或截断内容不得保存为整文件。
3. 审阅内存草稿，明确提交完整正文与正文SHA、原 `expected_sha256`和稳定操作key。新写需要当前项目写权限，关联任务的当前run／basis未变且未accepted。旧观察到期不单独使草稿失去CAS依据，但不跳过新授权。
4. 保留命令ID和准确状态。`applied`只确认这次文件转变；重新运行当前generation的原生checks，之后仍需准确候选SHA的独立正式测试和人工验收。

CLI使用相同Control接口和出站Runner路径：

```text
suma --project PROJECT workspace save --file save.json --wait
suma --project PROJECT workspace save-status COMMAND_ID --wait
suma --project PROJECT workspace inspect-save COMMAND_ID --expected-version VERSION --operation STABLE_KEY --wait
```

`save.json`承载合同的准确scope／policy／路径／原SHA／完整正文／内容SHA和key。请求超时后保留原JSON与key恢复，不复制一份新保存。即使写权撤回，原会话仍有项目读取权时也能用同key／同请求找回已存在metadata；这只读取原回执，新key和首次execute仍需写权。CLI可计算缺省新内容SHA，但不替代原完整文件SHA。CLI不连接宿主MCP；不要把正文、密码、Bearer或私有journal放进截图、普通日志或交付材料。

工作台交互为简洁草稿编辑、明确保存／核对、冲突Diff人工采用及“最近保存”metadata入口；真实GUI结果另记。草稿只在当前视图内存；关闭待核对编辑器后可找回命令metadata，不能恢复原正文或承诺跨刷新持久草稿。

## 不确定和冲突

| 状态／事实 | 操作 |
| --- | --- |
| requested | 请求最多120秒；等待当前Worker，不另建保存来催促 |
| leased | execute只发一次，最多10秒；禁止重派写入 |
| reconciling／unknown | 保留原命令，显式请求inspect，只核对原native operation；不得自动循环或换key重写 |
| inspect响应丢失 | 同命令ID、原key、原expected_version恢复；新的核对仅在此前核对终结后由人明确发起 |
| not_applied／file_changed | 保留草稿，完整重读最新文件，审阅Diff后明确采用新基线；不得force或静默覆盖 |
| not_applied／not_started | 宿主已持久建立同锁终态屏障；确认无效果后才能决定新的保存 |
| 其它not_applied／expired | 查看封闭错误原因；修复当前权限、路径或内容条件，重新审阅当前基线 |
| 宿主关闭／原账本不可证明 | 保持unknown；不打开历史路径，不根据当前文件“看起来相同”宣称成功／未发生 |
| 401／403 | 核对原会话当前权限；不借新会话读取旧命令，也不把授权失败自动解释为写入未发生 |

原会话当前读取权下可用 `GET /workspace/writes?project_id=…`找回最近50项metadata，按准确 `total`和 `truncated`理解范围；按命令ID查询已发生效果不要求目标仍在线。查询不重新读取文件、不恢复草稿正文。

## 运行和权限边界

授权检查通过和远端rename之间不是分布式原子窗口。撤权后已发出的原10秒短许可仍可能产生效果；当前原机器可继续排空固定事实，Pool撤回或原caller登出不能丢掉已发生效果。事实排空不允许新写、重新领取正文或更换assignment。

Control独立delivery正文在超时、到期或收到事实时清除，每轮最多处理100项；metadata和无正文ACK保留。Runner私有journal／账本有独立恢复与保留规则，不宣称所有磁盘副本同时删除。空轮询不落operation，实际非空lease／ACK仍持久幂等。不要清理活动attempt或唯一证据来“修复”未知。

必要验证复用[Control／HTTP用例](../../internal/platform/workspace_write_test.go)和[Runner用例](../../internal/runner/workspace_write_test.go)。当前Control与037观察合并定向检查通过（8.856秒），实际联合、GUI与生产验收分别记账。完整远程IDE、远程Git写、终端、PTY／LSP／DAP、物理多机／TLS及生产隔离继续待办。
