# Octop 项目接入与统一交付读取验证

状态：1.0.0、T-OCTOP-116，限定实现通过，完整平台仍在实施。日期：2026-10-28。源码清单：`bc05a3a0477b670ebcdf62192eeda2a74a42c71a068f123ee758690bca65b3ab`（22项）。[精简摘要](octop-integration-summary.json)记录实际命令、来源、原件摘要和边界。

## 本阶段实现

两份新增分享可见正文已恢复并按领域采用，精确范围和截断见[来源映射](../integrations/shared-roadmap-adoption.md)。采用统一实体／事件／权限、任务驱动、渐进披露和受限计划的方向，继续使用用户已批准的Go控制面／唯一DAG／Runner；不因参考方案更换FastAPI/Celery或增加历史兼容层。58项有限实现／34项待办／92项总范围不变。

Octop当前官方1.0.3b1的MCP连接器契约已经源码核对，当前方向为 **Octop→速码项目MCP**。现有开发者连接面板保留Codex，增加Octop配置片段及项目专用key、固定身份和语言头、字面令牌占位符和默认私有设置。用户在Octop的受控凭据编辑器人工合并，保留既有servers；速码不自动外部写配置、不注入令牌、不把模板标为已连接。HTTP头不会自动展开环境变量，普通headers可被Octop API明文回读，说明中已明确。

共享目录新增 `project.task_delivery` 和 `project.quality_execution`，对应MCP名`suma_project_task_delivery`／`suma_project_quality_execution`。二者仅供当前项目API读取：同事务复用原任务交付和逐例质量结论，严格绑定task／plan／run。用例默认10条、最多20条，每例最多3证据引用、来源提交最多16条，保留总数、原状态及完整性标识，续页仅用实际next_case。没有XML、日志、命令、完整正文或秘密输出，原64KiB结果上限保持。

Agent仍只有原四读工具；原三项草稿写入仍用原权限和幂等回执。目录不是授权，初始化／缓存不是当前权限；未增加批准、派发、验收、测试事实写入或部署。MCP服务实现版本统一取buildinfo1.0.0，协议版本与官方SDK1.8.0分别记录。

## 实际检查

| 层面 | 结果与证据 |
| --- | --- |
| 真实SDK／HTTP | 官方Go MCP客户端完成initialize/list/call；7个平台顶层用例通过，其中2个新增，覆盖真实交付、逐例页、固定来源、当前权限、撤权、跨任务隔离、四读范围与原草稿回执 |
| 领域读取 | 失败用例与DAG成功分别保留；既有计划修订／草稿变更不改运行固定次序，错误cursor、错task/run、null／未知参数被拒；3条证据／16提交上限保留总数和完整性 |
| 生成合同与语言 | 生成器1项、诊断1项通过，新增4DTO、REST路径不增加、共享工具17项且写工具仍3；5180对中英文键检查通过，自有生产硬编码检查0；类型、生产构建、合同漂移和1.0.0身份检查通过 |
| 组件 | 2项新交互＋11项既有，共13通过；切换客户端隔离晚到复制结果，Codex导出保持，Octop模板无真实token及伪造env插值 |
| 实际界面 | 真实生产构建与独立本机Control／新SQLite测试空间：中文登录、Octop选择、复制成功反馈、英文切换及语言头、默认折叠／展开／收起均已观察；当前成品1265×720无横向溢出，警告／错误控制台记录0；此前640宽实际观察用于发现长说明并收拢，不冒充390px完整矩阵 |

共22个不同顶层定向用例；子场景和修复后复验不重复计数。新增测试先实际RED，缺少两工具的断言失败，随后GREEN。没有重复全套或厂商模型测试。质量fixture为受控报告数据；真实Node报告和双Runner代码测试在[上一阶段报告](test-plans-verification.md)，不重复计成本，也不冒充本阶段真实厂商任务。

浏览器先通过新Cua通道取得明确本机URL，解决本轮原浏览器地址识别阻塞；一次严格label定位未匹配后刷新AX并通过真实控件选择，没有绕安全策略。实际发现说明过长后仅将高级说明折叠，进行一次必要后续生产构建并真实确认。共2次Web构建、1次Control构建；Runner／原生构建0、模型／厂商调用0。两个隔离预览均确认关闭，用户原预览／数据库未修改。旧质量页的浏览器验收缺口没有因此自动变成通过。

## 界面证据与操作

本机证据：[中文主要操作](../../.cache/e2e/octop-116/screenshots/octop-zh-main.jpg)、[英文主要操作](../../.cache/e2e/octop-116/screenshots/octop-en-main.jpg)。截图、原AX、原件、数据库和日志留在忽略目录并记录SHA，不进入Git；最终源码与报告可正常推送dev。

路径：隔离空间登录 → 设置／接入令牌 → 开发者接入 → Octop → 复制配置反馈 → English → 语言头en-US → 展开／收起详细边界。没有在界面签发凭证、配置外部实例、调用模型或启动DAG。

## 剩余交付边界

**本阶段没有安装或运行真实Octop实例，也没有完成ACP编码派发。** 官方ACP当前cwd不直接决定Agent文件后端；OCTOP_HOME、workspace_dir与backend需绑定本次私有工作树并核对实际scope，权限请求、取消和原操作未知结果需独立证明。不能用ACP end_turn作为进程已停止依据。完整接入设计见[架构](../architecture/octop-integration.md)，使用步骤／AI SOP见[指南](../integrations/octop.md)。

完整专家／记忆／技能／渠道、远程桌面、模型预算、真正多机／数据库／多端和SaaS仍按完整实施计划推进。当前紧凑质量响应不含XML，但内部原quality读模型仍会解码私有attempt结果JSON，不声称数据库零XML读取。成熟连接器依据[Octop官方源码](https://github.com/TencentCloud/Octop/blob/890111b7879665840414ca28ab168b05b0db0921/src/octop/infra/connectors/custom_mcp.py)。
