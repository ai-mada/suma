# 本机工具发现交互合同

T-TOOL-DISCOVERY-202；suma 1.0.0；负责人：主集成代理。关联[工具管理后续计划](../docs/delivery/tool-management-next.md)与[既有工具管理合同](tool-management.md)。完整31项／P0–P7保持active。

## 目标与确定规则

工具发现应先回答“本机有什么、需要处理什么、下一步做什么”。复用既有九项被动扫描和唯一Quick／远程接入，不增加网络路径、原生探测、安装器、派发器或权限。

- 只有`installed=false && reason_code=not_installed`且不是当前选择的条目进入折叠组；安装已发现、所有其他原因及当前选中项保留在主列表。
- 两组均保留原扫描顺序和原对象；不修改、去重、合并CLI／SDK协议，不推导可执行或模型健康。选择在未发现项也必须可见。
- 纯投影接口`projectLocalToolsScan(items:readonly InstalledToolFact[],selectedAdapterID?:string)`返回`{primary:InstalledToolFact[],other:InstalledToolFact[],detectedCount:number,attentionCount:number}`。detectedCount仅计installed=true；attentionCount仅计installed=false且原因不是not_installed。统计表示安装观察，不表示连接或派发资格。
- 主列表为空时显示简短解释和原节点接入入口（若调用者提供）；其余工具使用原生details/summary默认收起，可通过键盘展开。当前已选未发现项进入主列表，不能被收起。明确选择使行跨组时，只在原动作仍持有焦点的情况下保留该动作焦点，不因外部选择变化抢焦点。
- 缓存、过期、失败、busy、当前身份／项目／调用者代次及原回执语义保持。展开和选择不发起额外探测、配置、安装或HTTP；只有原明确重检触发force扫描。切换scope重置展开状态，旧响应不能复活旧结果。
- 展开后保留每个条目的准确协议、检测详情、已有连接及原配置入口。没有合法唯一适配描述符时不产生猜测动作。
- Claude SDK不支持版本使用准确既有版本提示；CLI已安装、SDK资源、模型认证、节点连接、登记、启用和测试验收分别表达。
- zh-CN/en-US成对词条，长名称换行、触达至少44px、焦点可见，390与1366代表布局不溢出。

## 分工与验证

1. 投影代理独占新`local-tools-scan-presentation.ts`及其定向测试；不改原解析器。
2. 交互代理独占`LocalToolsScanPanel.tsx/.css/.test.tsx`和原local-tools-scan双语目录，消费上述投影。
3. 文档代理只读参考基线，独占本阶段产品／架构／设计／英文四份文档，不改产品源。
4. 主集成代理拥有合同、状态、通用检查、代表GUI、验证摘要和Git白名单。复用已通过的未变更后端／SDK证据，不重复执行厂商、Go或完整测试。

运行必要的新投影行为检查、受影响组件检查、一次类型与国际化检查、一次隔离组件构建和两组代表界面；仅在失败或后续实质变更时续验。真实Electron、厂商模型、多机、跨平台安装及完整平台验收另行推进。
