# 工具上报反馈限定验证

版本1.0.0；T-TOOL-REPORT-FEEDBACK-204；负责人：主集成代理；日期2026-11-02。状态：本阶段限定通过，完整平台继续。关联[合同](../../contracts/tool-report-feedback.md)、[PRD](../product/tool-report-feedback.md)、[架构与AI SOP](../architecture/tool-report-feedback.md)、[交互](../design/tool-report-feedback.md)及[精简摘要](tool-report-feedback-summary.json)。

## 实现与检查

原指南新增当前库存检查、60秒／3秒的明确等候及准确返回，沿用原API、完整协议匹配、权限和Quick登记／启用。配置编辑和新采用使旧结果失效；隐藏暂停但不续截止，取消拒绝迟到结果，TTL即时撤资格，权限失效清观察。无需填完代码基线即可检查已有库存。

18个不同顶层检查限定通过：模型6项（4新／2直接旧）、组件11项（5新／6直接旧）及真实HTTP／SQLite／浏览器1项（内部两组）；合计10新／8直接旧。未选14项不是已通过。模型原6项绿色后发现时钟回退P2，原纳秒检查补反例实际RED→GREEN，仅重该1项，其他5沿用；不重复计数。初Remote新增接线1项真RED，其他4项新组件首次GREEN。

一次完整Web类型检查exit0；一次i18n检查9430对词条、0生产硬编码；一次Vite生产组件构建exit0，输出仅忽略目录，现有预览未覆盖。成熟Radix模块的use-client打包提示保留于原日志，不作为失败。无新增依赖、迁移、产品API或第二调度器。

## 实际接口和界面

最终真实HTTP／SQLite与生产Remote／Quick组件两组通过：中文1366浅色从未上报建立基线→接收新报告→键盘明确选择准确工具、Worker、命令→Quick重新核对当前事实；英文390深色读取当前清单→旧ACK原时间不变→一次真实轮询仍等待→取消→重查→撤销节点→真实403清除观察。Workspace原草稿清理和同目标保留由组件检查覆盖，不声称本夹具是完整App。

最终接口25次，其中浏览器18 GET及1次原只读POST匹配；夹具6次真实请求负责独立Worker登记、session、原报告与撤销。浏览器业务写入0，任务／配置／执行计数不变；报告发送3次／两个原操作，重复ACK不刷新库存。库存是自有元数据，原生CLI、厂商、模型、Worker任务、配置登记和启用均0。

Root已查看最终两张局部原图；390／1366无横向溢出，按钮及原因摘要44px。图片均带唯一运行名及SHA，未覆盖历史截图。三次Electron主进程正常exit0、Node／Go最终exit0，Root观察三主PID均已退出；没有强制停止。每次夹具cleanup前留存一致SQLite，全部私有数据与图片保存在.cache，不入Git。

初两次Go实际调用exit1原件保留：首次隔离Cookie写错会话，未到报告；第二次实际新报告／准确选择已发生，但验证器拦截原只读匹配POST并错误假定单节点存在下拉框。修正仅涉及验证代码，复用原产品、类型、双语及唯一界面构建；第三次实际调用通过。模型摘要命令的一次PowerShell解析失败也原样记录，不算测试执行。

## 可复现入口和证据

使用已安装Node、Go、Electron和Playwright，无安装／厂商请求。先运行 node tests/integration/tool-report-feedback/build.cjs；然后设置 SUMA_TOOL_REPORT_FEEDBACK_204_ACTUAL=1、SUMA_TOOL_REPORT_FEEDBACK_204_NODE为Node绝对路径、SUMA_PLAYWRIGHT_MODULE为已安装Playwright目录，运行 go test ./internal/platform -run ^TestToolReportFeedbackActualHTTPAndBrowser$ -count=1 -v。每次生成独立.cache/tool-report-feedback-204/actual/UTC目录；临时登录和夹具密钥只经stdin到Node／Electron主进程，不传给renderer。正式产品不包含夹具路由。

31项当前源与原输入SHA一致；模型、UI、命令、实际接口、图片、私有快照和原失败摘要位置及SHA见精简JSON。Root原命令、原始报告保留在.cache/tool-report-feedback-204，不提交缓存。

## 边界与后续

库存时间不证明某个operation／文件或复制命令已执行；严格因果关系需单独来源版本设计。真实厂商／模型／计费、完整App及桌面／设备主题、多机、OceanBase／PostgreSQL、安装包、规模和生产验收未由本阶段证明。原31项／P0–P7／58limited／34pending／92total保持active。
