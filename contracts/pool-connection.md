# 已准备配置到真实Pool运行与项目工具

版本1.0.0；阶段T-POOL-CONNECTION-212实施合同。完整31项／P0–P7继续。关联：[机器配对](pool-runner-pairing.md)、[配置准备](pool-config-preparation.md)、[安全诊断](pool-preparation-diagnostics.md)。

## 同一执行内核与清晰事实

继续使用原PoolRunnerIdentity、系统凭据库、PoolSession、PoolWorker、库存owner／Journal、双边Grant、项目路由、工具配置与Doctor，不新增调度器、健康表或重复权限模型。配置准备、单次身份检查、持续维护、工具资格与业务执行是不同事实。文件字节SHA不等于解析后运行配置SHA；historical_last_seen_at不证明当前连接或凭据代次。

### 本机检查

`suma-runner --check-pool-connection --control URL --pool-config FILE --pool-identity IDENTITY`仅做有界读取、原身份精确匹配和当前PoolSession检查；除语言／帮助／版本外拒绝其他显式选项，包括false执行开关。复用普通文件／父目录稳定与2 MiB限制，保留正常相对配置路径的CLI用法；输出原文件字节摘要。先核control／pool／runner再读系统秘密，永不退回环境token。

当前session的required字段、唯一键、完整有界JSON、scope标识／去重再次校验，允许原完整Runner DTO明确字段，不另起HTTP客户端。绑定同一Runner／Worker／凭据代次，槽位超配拒绝，不自动clamp。响应为`PoolConnectionCheck`：11字段和配置顺序的scope交集；不把server额外Grant注入本地。pool_active=false或granted=false明确保留，成功认证不是可派发结论。无Worker、Journal、任务领取、工具探针、模型或状态写入。

### 明确启动

有效当前准备预览后按需展开“连接这台机器”。沿当前Shell／输出文件和可见控制地址，明确填写目标机器身份文件；默认状态目录从输出文件派生`.runner-state`，可明确调整，不猜用户目录或删除旧Journal。

单次检查和前台启动命令分别复制，复制不执行。检查安装模式要求用户明确允许固定目录工具检测，使用原`--maintenance-only --allow-installed-tool-discovery`，不领取开发任务。任务模式明确选择后才加入`--allow-exec`；可选安装检测同样单独授权。`--describe-tools`不得混入身份启动。所有路径与URL沿原Shell引用；不会使用环境凭据备用、生产后台自启或隐式注册／启用工具。

## 唯一读取接口

GET `/execution-pools/{pool}/runners/{runner}/readiness?tenant_id=...&project_id=...`，仅两个必填scope参数，无token／路径／配置摘要。DTO统一位于`internal/poolconnectioncontract`并进入原生成合同。

返回`PoolRunnerReadiness`：pool／runner／worker、pool_status、服务器checked_at、runner当前登记／凭据事实及historical_last_seen_at、准确scope公开Grant元数据、can_open_project、route_state、source_current／source_reason、库存观察／失效时间；只有当前相符库存才提供inventory_item_count，0明确表示空清单。不存在总ready／connected或自动工具资格结论。

当前组织Pool管理权限先核，准确Grant未知404。Pool拥有者不获得消费项目权限：无当前同tenant项目读取proof时route_state=restricted、source_reason=project_access_required，库存时间为空、count缺失，绝不查询项目路由／库存／maintenance／档案。正式项目读取沿原规范proof和当前权限，无嵌套Service事务，最终关系在同事务复核；DB错误不当作无权限或空清单。

route_state为restricted／unconfigured／different／current。credential_state为valid／expired／revoked／version_mismatch。source_reason为project_access_required／route_not_current／inventory_unreported／inventory_stale／maintenance_unknown／maintenance_stale／observation_mismatch／current。source_current仅在current为true：原maintenance identity重核当前project／pool／Grant版本／凭据版本及摘要，原能力supported且新鲜，与原库存observed_at完全同一报告，原库存新鲜及有效解码。薄提取原库存读取规则复用，不复制另一套TTL。Pool库存写入统一原高精度toolInventoryClock／RFC3339Nano，回执重放不续时，无列或兼容迁移。

字段为checked_at时事实，不是网络在线。界面明确读取时间，按原单调租约和最早库存／凭据到期失效当前标记，或始终明确历史as-of；不能永久保留“当前有效”。正式资格仍在原ToolsPage／Doctor按准确档案重核，不以count或enabled状态冒充任何任务可派发。

## 准确项目交接

仅当前服务端can_open_project允许链接：`?project=PROJECT&view=settings&sub=tools&panel=pool-tool:POOL:RUNNER`，URLSearchParams编码。固定三段、受限标识，不接受其他格式；worker始终准确派生`pool-<runner>`并与当前项目的原sharedWorkers行id／pool／worker匹配。ToolWorkspace在当前actor／组织／project／API来源的准确catalog和sharedWorker读取后消费一次locator，只打开原Quick／Guide；不自动scan、创建、启用、派发或跨组织切换。

准备内容、节点、当前来源或scope变化使连接命令／读取立即失效；异步读取与来源A→B→A绑定当前revision／intent，未知写入继续原恢复逻辑。新连接区与安全诊断均渐进展开，中文／英文、窄屏、键盘、加载、无权限、空库存及失联分开设计。

## 分工与必要验证

主集成拥有本合同、5个共享DTO、OpenAPI生成、实际纵向集成、设计文档与Git。控制代理拥有当前读取服务／HTTP／proof、原库存读取薄提取和Pool报告精度；Runner代理拥有安全loader、单次check、原Session完整校验和CLI入口；Web代理拥有连接区、当前读取和准确ToolsPage交接。

定向验证权限隔离、凭据轮换后旧库存／lastSeen、来源配对、空库存、重放不续时、原Session响应完整性及混合模式／秘密前范围核对、Shell／源变更和准确导航。一次有用纵向旅程用真实HTTP／SQLite、原已配对身份和PoolWorker发送当前库存，经新接口进入准确项目工具，再继续原资格与执行；合成工具、注入或真实系统凭据、实际进程、真实开发任务／独立测试各自标记，不把代码或模拟当生产验收。源未变的旧成功结果复用，避免全量重跑；完整31／P0–P7保持进行中。
