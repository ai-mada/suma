# 工具配置自动体检

T-TOOL-DOCTOR-181；suma1.0.0；原31项／P0–P7完整目标保持。

用户选择正式工具档案后自动读取四项体检：配置授权、节点身份、库存时效、完整配置。只给准确等待原因和原管理入口；不打健康分、不调用模型／CLI／SDK，不修复、派发、续期或启用。节点身份有效不代表机器在线；新鲜探测、模型登录、任务预算、执行和验收分别处理。

## 当前事实接口

`GET /api/v1/tool-profiles/{id}/diagnostics?project_id=...`，当前browser-project-read与准确resource-id授权先于档案查询；API／Agent／Runner／Pool令牌不获此权限。沿原withCurrentRead事务、数据库时间、toolProfileObservationLockedTx／toolActivationLockedTx／toolWorkerReasonTx。观察与诊断必须共用内部collector，不能复制一套可派发规则或从中文Reason反推原因。

读取可显式携带`X-Suma-Expected-User`／`X-Suma-Expected-Organization`，各字段独立核对单值、ID格式与真实当前身份；缺省不强加读取头。CLI scoped GET保留用户指定断言，浏览器体检携带捕获的两字段。此通用读取规则也覆盖正式POST查询；`tool-profiles/matches`按只读分类。断言无效返回428，身份不符返回403；它们不选择身份或增加权限。

响应`ToolProfileDiagnostics {tool:ToolProfile,checked_at,expires_at,checks:ToolProfileDiagnosticCheck[],next_action}`，checks固定顺序且恰四项：`authorization,worker,inventory,profile`。Check闭合为`{code,status,reason,reason_diagnostic?,next_action}`；status仅`pass,blocked,not_checked`。Diagnostic只携展示身份，动作来自固定领域结果，不从Key或任意原文推断权限。

next_action仅`none,review_profile,open_worker,register_profile,refresh`。暂停或启用人权限失效→review_profile；已撤销→register_profile；节点授权／凭据／池或库存问题→open_worker；完整配置改变→register_profile；其余未知需要重读→refresh；所有已知前提通过→none。总体优先第一个阻断项，次序与原Available判断一致；不另造ready布尔。

未启用仍可读取当前节点与库存事实。worker检查只复用现有私有／Pool身份资格，不伪造心跳或在线。库存未上报／过期／未来时间时inventory阻断，profile为not_checked，不能用旧Observation显示pass。库存新鲜时profile检查原command存在、完整EqualProfile及Observation.Available。原Tool.Available、Reason及Observation语义保持；体检不改变业务或操作回执。

checked_at为同一数据库时刻；expires_at至多checked_at+60秒，有新鲜库存时不得超过原库存期限。它是显示观察期限，不是授权租约。界面以请求起点的单调计时扣除网络时长；过期后隐藏可操作建议并提供重读。当前权限可立即变化，所有后续写入继续原当前权限、CAS和操作回执。

## 简洁交互与CLI

原工具详情增加一张紧凑体检卡，默认显示优先原因与下一步，四项事实可展开。首次选择／准确来源变化自动读取，显式重新检查只读。原pending写优先，不能以新体检替代或清除未知写入；当前API caller、账号、组织、项目、工具ID／版本变化后丢弃迟到响应，不用旧报告解锁原动作。

只调用父级已经提供且与当前准确来源绑定的动作：review_profile打开原CAS启用确认（当前非撤销enabled可明确重新审阅）；open_worker定位原准确节点管理，不自动新建或冒充本机探测；register_profile进入原快捷配置；refresh重读。没有准确管理入口时只显示原因／重读，不伪造可操作按钮。报告返回不同版本时提示刷新原详情，不沿旧版本执行。

CLI新增`suma --project PROJECT tools doctor --id PROFILE_ID`，复用原登录会话／当前视图／client，读取同GET并严格核对scope、完整Tool、四项、期限。命令只输出诊断事实，不自动运行建议。中英文界面、帮助和展示原因完整；沿成熟组件、原数据源和原写入确认。

只验证本次授权、四项独立事实与原Available一致、时效、CLI只读、当前来源及受影响交互，复用未变SDK和180证据。实际HTTP／SQLite、合成UI与真实厂商运行分别记录；完整远程／多机、异常副作用、数据库、安装和全平台继续交付。
