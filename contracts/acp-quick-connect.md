# ACP 原生工具快速连接与模型发现

T-ACP-QUICK-CONNECT-171；速码 suma 1.0.0；2026-10-31。沿已批准本机工作台、统一工具与模型管理架构实施；完整31／P0–P7／58限定／34待交付／92总账保持有效。基于[当前ACP执行](acp-cli-execution.md)，不新增派发器或账号体系。

## 完整用户链路

原工具管理Qoder入口：检测本机 → 明确读取已选工具模型 → 展示实际模型选项并确认 → 原托管Runner连接 → 启动后新水位库存 → 暂停登记 → 明确启用。高级手动配置继续使用同一固定ACP执行合同。检测或模型列表不是登录健康、任务资格或实际模型调用证明。

本阶段优先采用当前官方推荐的原生独立程序，保持170原生可执行、固定`--acp`及版本／摘要复核。官方npm仍是第三方安装形态，完整目标中保留其后续受审入口解析；不能用平台“不做历史兼容”消掉真实集成范围。本阶段不执行shell包装器，也不把npm程序伪装原生。原生未发现／仅包装器／无法核实应准确显示不同原因，并提供固定官方原生安装入口；不自动安装或修改用户PATH／登录／工具设置。

## 分层与闭合接口

- `acpcontract`新增安全目录与发现结果，统一Go／TS合同。
- `acpstdio.NewModelDiscovery`复用成熟SDK和严格帧／模型选项验证，但使用独立无目标入口：Initialize → NewSession →读取准确category=model/select目录。禁止SetModel／Prompt／Authenticate／resume、文件／terminal及任何批准；收到正文、工具或权限活动即失败关闭。
- Runner的本机发现扩展`qoder-acp`，只选择本机原生候选、实际完整版本及可执行SHA。明确的私有stdin模型发现命令接收主进程选中的安装依据；网页不能提供exe／argv／env／URL。
- 桌面主进程保管安装及目录引用，renderer只交闭合adapter、原引用及模型ID。原主窗口／主frame／origin、当前user／tenant／project、原HMAC context与workspace binding／HEAD前后复核继续有效。
- 原managed controller／加密journal／private启动输入扩展准确ACP选择；status／stop沿原配置，不接受编辑参数。新增字段仅允许Qoder，其他adapter不能借字段获得ACP权限。

## 公开观察事实

`ACPModelChoice {id,name,configuration_sha256?}`；`ACPModelCatalogue {source,protocol_version,config_id,current_value,models}`。source=`acp_session_config`，最多256个唯一精确模型ID；展平分组但不猜默认值、不把名称当ID。当前值未提供或不在列表时明确未核实，用户仍只能确认实际列出的选项。

`ACPModelDiscoveryResult`绑定实际可执行SHA、完整规范化版本、安全AgentInfo、目录及宿主停止事实。无session、路径、原始配置、凭据或原包；命中秘密的ID直接拒绝，不能改写后生成新身份。模型名称可以安全脱敏；目录不带价格、模型健康或硬预算。

主进程为观察分配原`probe_id`及安装引用，保管准确scope、workspace binding／HEAD、版本／SHA、可选凭据引用和最多60秒有效期。模型选项的配置摘要由Go按实际固定规格计算；采用前再核原引用、期限、安装及当前context，选择必须仍在原目录。重新读取原观察不延长期限，不自动新建会话。

## 生命周期与副作用

模型发现是明确的低副作用观察，不是静态文件扫描。NewSession可能创建供应方会话、读取CLI配置或联网；“零Prompt”只证明速码未通过ACP发送开发目标，不推定零费用或CLI内部零行为。目录只在真正的进程Wait／输出Finish及停止事实核对后返回；Cancel或关闭pipe不算进程停止。

使用独立本机probe cwd、有界时间／输出、干净环境和既有进程控制。SDK仅在准确声明session close能力时有界关闭会话；不声称删除所有厂商历史。意外活动、未知退出或不能确认停止时不返回可采用目录。

不把模型发现放进GET、页面打开、库存心跳或自动重试。一次明确操作只建立一个观察；同一操作的未知回执核对原观察，不重新执行。探测结果与请求正文／scope持久或有界保管；应用恢复不能把旧PID或旧目录当活进程、不能自动再发会话。

凭据仅支持现有CLI登录或明确的本机环境变量引用；实际值不进IPC结果、公开DTO、argv、配置摘要或日志。仅所选受审引用值可进入对应子进程，并映射官方变量；平台／其他工具／系统保留变量继续拒绝。环境变化或安装变化不能悄悄复用旧目录。

## 托管连接与正式任务

确认的模型、完整版本、可执行摘要、可选凭据引用和配置摘要进入原私有启动输入及加密journal。Worker重新解析实际原生安装并核相同依据；原body/key恢复及停止不要求重新采用过期模型目录。连接中配置草稿变化立即失去“此配置已连接”状态，不能覆盖原待核操作。

connected要求真正启动后的新库存和准确ACP capability／model／configuration SHA匹配；不只按adapter/protocol寻找。随后复用原QuickConnect暂停登记、独立enable及任务预览／Start／claim；组织／项目enforce仍拒绝观察预算工具。模型发现不创建任务、业务Run、候选、测试或验收。

## 并行实现与验证

主集成维护合同、纯DTO、平台context／registration闭合、生成类型、文档和Git。协议代理实现无Prompt发现及共享模型提取；Runner代理实现原生发现／私有模式／managed输入和真实进程证据；工作台代理维护桌面controller／journal／IPC与简洁模型选择。原21项WIP不得整文件暂存，必要共享挂点先由主集成确认。

定向验证无Prompt／Set／auth、严格目录与秘密、超时／输出／真实停止、当前scope／安装变化／目录过期／原操作恢复、准确库存和paused→enable。只跑受影响项，复用170未变结果；最后一次真实本机GUI／HTTP／IPC／Runner链验证连接和停止，task／Run／model Prompt0。正式厂商、跨平台安装、多机及完整平台范围继续独立交付。

依据：[官方原生安装](https://docs.qoder.com/cli/installation)、[模型选择与列表](https://docs.qoder.com/cli/model)、[ACP](https://docs.qoder.com/cli/acp)。本轮官方npm元数据为1.1.81，仅作来源记录；不将该包版本或公开网页视为本机已安装证明。
