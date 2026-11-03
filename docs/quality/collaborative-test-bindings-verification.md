# 协作测试配置与工具自动核对：限定验证

速码1.0.0 · T-COLLABORATIVE-TEST-BINDINGS-223 · 2026-11-03。负责人：主集成代理；完整31项／P0–P7继续，58limited／34pending／92总账不变。关联[合同](../../contracts/collaborative-test-bindings.md)、[产品](../product/collaborative-test-bindings.md)、[架构与AI SOP](../architecture/collaborative-test-bindings.md)、[交互](../design/collaborative-test-bindings.md)、[摘要](collaborative-test-bindings-summary.json)。

## 实现与必要检查

讨论、明确推荐采用及团队创建／修订接通同一原测试选择与六字段冻结快照。团队省略／[]／非空分别表示保留／恢复发布定义／替换，null拒绝；未知写入继续恢复原body/key。推荐模型五字段Choice不变，按真正采用开发Worker复验独立性。

当前选中工具复用原live开关自动GET诊断：可见、到期且距上读至少15秒；后台暂停，失败等待明确重试。详情后台刷新保留已结算和在途原读取，真实写锁／来源变化取消，挂起旧DOM回调被拦截。自动核对不触发修复、注册、启用或模型。

- Go：13个不同定向top（9新／4直接旧），包括三个HTTP入口边界、冻结与provenance、保留／清空／上下文、原回执和原合同生成；全部通过。
- Web：18个不同定向top（13新／5直接旧），含三入口原选择器、来源ABA、未知恢复、自动核对以及真实Workspace 10s轮询接缝；最终全部通过。重复调试次数不计新覆盖。
- 完整Web类型首次13诊断，修正新泛型读取及Node夹具；6受影响文件窄语义0，最后Doctor2文件窄语义0。未重跑完整旧套。9891对中英文词条，生产硬编码0。
- 原Go生成器999模型／452路径；0新路径、表、队列、调度器或生产依赖。git diff --check通过。

## 真实执行（独立于界面夹具）

一次隔离SQLite、三个任务和同一发布流程。讨论使用原受控已完成只读回复夹具，仅预览；推荐经真实原native bundle／Go模型网关向自有确定性loopback提供方请求1次，再明确采用，不开始开发；团队创建、保留配置修订、激活／tick，执行一次真实Git开发和另一Worker同提交JUnit。

代码候选与测试提交均为73344fd58352caab7aba14c4854716851f189421；JUnit passed，业务开发运行succeeded。三入口冻结同一准确tester；原管理员工作区和发布流程未改；日志Outbox清空；0商业模型调用、0Node复制、0CLI binary构建、0Review／客户签收。运行成功不等于业务已验收。

私有证据保留于.cache/collaborative-test-bindings-223/actual/20261103T000201947215600Z；receipt SHA1b62475f951cd447c434049f469ddc54be27815ef9320a1e2214a0d55e07fffc。编译与单次实际运行的参数、退出码和原输出在阶段go目录，Go缓存／临时输出仅过程指定系统tmp，未改全局环境。

## 代表界面

四组代表生产组件合成HTTP验证通过：CN讨论显选／冻结预览，CN团队创建及明确[]修订，EN390深色推荐采用保持五字段Choice，以及Doctor可见16秒读数1→2、暂停后30秒增0。通过组4次合成POST、全部尝试5次，0真实数据库写／Worker／模型／探测。真实数据库／Worker证据单列，合成收据不计实际派发或验收。构建共2次：首构建因其后的Doctor在途owner修复失效，保留原清单；最终构建只输出阶段cache，不覆盖活动预览。

界面共4次启动尝试，后3宿主正常退出0、端口关闭、无强制停止；首启动退出码未知，PID后来观察消失。0页面／控制台error，桌面夹具CSP warning保留，不宣称生产安全验收。讨论／推荐测得控件最小44px；团队旧桌面控件最小36px，仅确认无横溢出，不宣称全44px。Root查看CN讨论、EN390推荐与Doctor三张原截图。组件夹具不代表完整App／全设备主题验收。浏览器摘要SHA8d92e3ddf4f4c94364c25741b3bdb489c36e100c53121d68a858e34f0fd54916。

## 保留的失败与边界

Doctor计时夹具曾在异步响应体稳定前跳时钟，原失败保留；修正为成熟Vitest异步计时／等待。检查也修复了真实来源owner、旧回调及早timer重设问题。类型原13诊断完整保留。浏览器首次缺directory／partition导致超时，原PID后来观察消失，退出码未知不补0；后续team夹具漏勾明确修订任务，推荐夹具coverage.complete=true却给空reason，被产品正确拒绝；均仅修夹具，已通过组不重复。

仅新增与直接受影响范围运行必要检查；未重播T222基线／Review链、旧全量SDK或整Go套。真实商业CLI／供应方、物理多机、其他数据库、完整主题／设备／App、安装器、规模及生产仍待分别验收。缓存、截图、日志、数据库、二进制与两份原无关冲突报告不提交。
