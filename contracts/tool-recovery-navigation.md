# 工具诊断到准确节点连接准备

版本1.0.0；阶段T-TOOL-RECOVERY-NAVIGATION-217；状态：实现合同；负责人：主集成／工具工作台。T216已交付，完整31项／P0–P7继续。

## 用户闭环

当前配置的Doctor建议打开执行节点 → 定位准确节点卡并给出“准备连接” → 原目标工具专用指南、准确Worker → 用户在目标机器明确准备／启动 → 原等待新上报 → 回原Quick核对配置。复制命令、生成配置、打开指南不算执行、在线、启用或修复成功。

现状接缝：`ToolProfileDoctor`已在有效诊断内核对`open_worker`，但`ToolWorkspace`的`onWorker`／`onSharedWorker`只滚动和聚焦节点；普通节点卡主要提供安装检测及撤销。新增的是这段准确的导航与交接，不重复检测、配对、CRUD、核对、换新或调度能力。

## 本阶段范围

- 优先支持当前项目的普通Runner，工具必须能由原`toolPreparationTarget`准确解析（现有九个工具／协议组合）。不得把通用command、suma-native、Octop或未知协议悄悄映射为其他工具指南。
- 共享Pool节点保持原边界：项目工具管理权限不能生成普通项目Runner身份或扩大Pool权限。继续显示原共享节点事实；没有原已授权Pool入口时给出管理员处理说明，不调用普通Runner准备作为替代。
- 撤销／到期身份不以“准备连接”冒充复活。需要重新配对时沿原明确配对入口，批准后才按新准确身份继续；不自动换Worker或旧凭据。当前身份阻塞时只进入原明确配对入口；创建、批准及启用仍分别由原操作处理，不复活旧身份。

## 目标与当前性

恢复目标是本页面的短期公开意图，不是第二配置权威：原API caller、账户／组织／项目、单调代次、profile ID／version／SHA、Worker、adapter／protocol及准确目录descriptor。不得存凭据、私有路径、argv、模型值或报告内容；不持久化为健康事实。

Doctor callback继续使用原`onWorker(workerID)`签名，由父闭包捕获当前profile；匹配项目、当前详情、版本、Worker及SHA后才定位。目标与页面/API/actor/项目/profile选择或依据一起失效；ABA、迟到catalog/diagnostics结果不能恢复旧目标。

在节点的明确“准备连接”操作中，先读取原当前`GET /tool-profiles/{id}/diagnostics?project_id=…`，复用`readToolProfileDiagnostics`、原deadline与profile一致性。再准确读取原adapter catalog，要求唯一已实现的同adapter/protocol及原`toolPreparationTarget`。失败保留可解释状态与重读入口，不自动选择另一工具或节点。

## 原操作优先

- 原ToolWriteRecovery pending／busy、配对未结、原检查／核对本地意图存在时，继续原恢复保护，不启动新写、探测或准备意图。
- 新诊断若指向原`view_inspection`／`recover_inspection`／`view_recovery`，先沿原`inspectProfile`及准确locator核对原操作；不能用连接准备掩盖unknown占用，也不clear Journal。
- 诊断／权限／profile依据变化时，刷新并要求用户重新选择；不要猜“暂无检查”或把读取错误当作可恢复。

## 原指南与上报交接

先用`beginPublicDraft`保留准确adapter、Worker、原command/name和原profile作为换新来源，再调用原`openToolPreparation(adapter, workerID, true)`。这样原sameTarget规则不清空草稿，且新的报告可继续原明确配置换新，而非重复登记。

仅同Worker、同项目且原当前批准身份才带入配对信息；另一节点的pairing不带入命令。原Guide仍要求用户明确配置文件及执行模式，不自动安装、登录、调用模型、派发任务或启用工具。

原ToolReportFeedback继续以明确等待的baseline、新报告及有界TTL判定观察。返回时沿原`returnFromReport`，恢复导航额外核对其保存的Worker／adapter和当前connection nonce，不能用报告自身的Worker作为唯一期望。回Quick仍需重读原catalog、当前库存与精确matches；新建paused／换新／启用分别保留原明确确认和回执。

## 最小交付与验收

不新增后端路由、表、dispatcher、检测协议或修复执行器。操作名称、状态、失败和键盘反馈中英文；节点动作44px，焦点通过原`focusNavigation`及当前代次核对，展开原More/Guide而不堆叠Modal。

只验证新增的准确目标、原pending/unknown优先、来源ABA、迟到读取和报告交接；复用原配对、prepare、inspection/recovery、远程扫描及报告算法证据。代表界面只证明生产组件导航与明确选择，不冒称目标机器实际修复或厂商执行。本阶段单独提交，不把配置准备或只读界面验证记作实际远程修复。

## 常用操作与视觉层级

工具详情中的调整、启用／暂停、移除和用于任务沿用原action-bar，移到名称／状态之后、诊断与历史之前。仅改变优先展示位置，继续使用原版本确认、当前授权、影响和未确认写入保护。连接准备动作使用现有按钮、节点卡和Guide，窄屏顺序自然换行，不新增常驻控制台或嵌套弹窗。

指南内明确换选另一工具属于新的接入意图：原Guide按当前localTool和navigation代次核验报告，返回新Quick时清除旧配置换新来源，不把旧source移交给另一工具。未明确选择的迟到跨工具报告仍不得采用；不能仅用报告自己的adapter作为当前来源证明。
