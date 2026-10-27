# 模型选择、预算与真实执行验证

状态：T-P4-036／1.0.0，限定本地功能验证通过。负责人：平台集成。完整平台仍在实施；本报告不代表厂商、远程IDE或生产验收。

关联：[合同](../../contracts/model-execution.md)、[PRD](../product/model-execution.md)、[架构](../architecture/model-execution.md)、[AI SOP](../operations/model-execution.md)、[English](../en/model-execution.md)、[机器摘要](model-execution-summary.json)、[开发工具连接](../integrations/developer-connection-check.md)。

## 真实执行闭环

`TestModelExecutionCurrentPreviewActualRuntimeAndUsageLedger` 使用独立SQLite、当前真实账号与固定原生程序档案，经实际HTTP预览、Start、runner.Client.Claim，进入真实Runner.Execute、原生CLI bundle、官方MCP工作区和自有loopback模型协议，再经原results回调结算。单次最终联合执行实际报告输入23、输出17、合计40 Token；按冻结运营方两档CNY费率结算57微元，Token／金额预留归零。请求、响应原始字节摘要和模型绑定、attempt／fence逐项一致，管理员源Git HEAD保持。

另一项最终验收为两个独立本地部署各启动一次真实原生进程，检查实际模型、完整私有API路径、单独凭据引用和请求参数；每次报告7输入／3输出，不把该测试称作平台账本事实。最终native bundle摘要 `e2032655eead5d68fd514c4b8169dae8acf4e0f0ab09c8c11965fd9ffae720ac`。自有协议服务不使用厂商账户；正式候选测试和人工验收均为0。定向迭代的协议请求不合并为这一次联合执行的账务。

控制面另验证：报价变化后旧Plan仍按原ID／版本领取和结算、重复回执只记一次、撤下部署后Start拒绝且无Run；错误模型、缺少旧worker绑定回执或超界用量保留已知计数和剩余预留，进入核对。普通参数在预览与进程前均有界；绑定副本自动重试为0，原发布模板保持，编译也拒绝非零重试。HTTP未知／重复／异常整数、租户项目与当前库存资格按现有授权检查。

## 实际界面与必要修正

独立59212夹具使用真实程序DescribeTools观察、两个本地允许列表定义及一份明确运营方费率；没有模型端点、领取或Start循环。实际页面验证未定价不显示免费、不足预算阻止准备、选择准确模型并填写2次调用／100累计输出／20000 Token预算／CNY0.02，再保存唯一固定Plan。中文英文、键盘展开和刷新后同一Plan与预算均保持。Plan为ready／v1，任务仍todo，0 Run、0 usage attempt；闭合数据库准确回读绑定与预算，owned Git HEAD保持，服务正常退出0，临时页签关闭。

首次真实使用发现两条有效反馈：最终SDK构建改变程序摘要，旧已审批档案正确受阻；在同一隔离夹具明确新增、审批准确当前档案并登记报价，原档案保留。另一处为旧ToolWriteRecovery把max_output_tokens／token_limit误认为凭证，导致预览在HTTP前受阻；仅对准确选择路径中的安全整数开放例外，凭证、对象伪装、错误路径继续拒绝，原操作未知回执恢复保留同一ID与参数。针对该缺口先红后绿2项；固定预览现直接显示本次预算。

前端首次构建与实际发现后的最终修正分开保留；最终136项资产，index摘要 `be48d29e68cf665ed26406f3de26c5ef7bef76c95be0ef977a429b18b9cdba47`。仅更新本阶段隔离夹具：保留旧资产、先写新摘要资产、最后原子替换入口。主035预览与其数据库、凭证均未修改或重启。隔离截图和原始记录均留在忽略目录。

## 必要检查与证据边界

- 控制面合同、DAG、派发、库存、原用量与预算事务执行受影响选择；模型HTTP和普通限额／NoRetry负向边界通过。
- 原生Runtime最初完整包75／76，唯一失败是旧目录数量快照；保持原50键摘要后明确登记扩展，受影响22项通过，没有再次运行整包。Go国际化初查也发现两个旧快照断言，按显式扩展集合修正，原46条文案摘要保持，对应两项通过；不宣称同一轮全套验收。
- Web模型读取与中英呈现3项、数值预算安全恢复2项、TypeScript和4452对语言键检查通过；发现过GET调用签名错误后定向修正。最终页面真实完成预览和持久回读。
- CLI新增models options定向检查及已有developer check真实MCP测试通过；共享Go vet及生成合同check通过。官方Go MCP SDK实际initialize→list→当前项目overview，支持当前API／Agent身份，拒撤权、重定向、超时、超量、错误结果与跨项目返回。不是远程工作区broker或IDE已连接。

准确最终运行证据在`.cache/e2e/model-execution-stage-036/runtime/verified.local.json`（SHA `a4f6080ba4d88515c29a6402fc2311efbaefb9a27b0471414aec4c1f37005a6f`），包含非秘密模型绑定／逐次摘要／账本结果和对应wrapper日志；界面与数据库记录在同阶段browser及ui-fixture目录。开发连接独立记录见[专项检查](developer-connection036-verification.md)。只提交必要源码、合同、设计和精简摘要，不提交缓存、私有登录资料、DB、原始日志、截图或产物。

## 仍需交付

独立模型目录与授权、多模型当前报价、外部CLI／专家团队／修复入口的模型选择、厂商账户与账单、tokenizer／硬费用网关，以及远程工作区文件／PTY／LSP／DAP仍待实施。现有context_window×steps是声明边界的保守准入，异常提供方报告超界后保留核对，不是假定精确Token估算或金额保证。完整P0–P7和所有已接受需求保持。
