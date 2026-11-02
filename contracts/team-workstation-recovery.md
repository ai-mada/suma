# 团队工作台：共享节点恢复与桌面离开保护

版本1.0.0；阶段T-TEAM-WORKSTATION-RECOVERY-218；负责人：主集成／Pool工作台／控制面／桌面运行时。完整31项／P0–P7继续。基线d13191e；不新增产品兼容分支或另一套调度／凭据权威。

## 共享节点的准确来源

项目工具Doctor定位当前sharedWorker后，保留原公开配置草稿及sourceTool；明确‘管理共享节点’进入原组织Pool工作台。公开PoolToolRecoveryPin闭合10字段：pool_id、runner_id、worker_id、tenant_id、project_id、tool_profile_id、tool_profile_version、profile_sha256、adapter、protocol。panel为pool-tool-recovery加固定十值冒号tuple；ID受限、版本正整数、摘要小写SHA256、worker严格pool-<runner>，不含名称、模型、凭据、文件路径或命令正文。URL仅定位意图，不授予任何权限，也不自动切组织。

当前项目来源须由当前完整profile及原sharedWorkers唯一行构造；允许库存失联来恢复，不据此宣称当前健康。进入Owner页后复核当前组织Pool管理权、准确Pool、准确Runner和准确租户／项目原readiness。新增GET /execution-pools/{pool}/runners/{runner}复用poolAdminTx＋poolRunnerTx同一当前事务，只原公开DTO；拒所有query，no-store，无凭据或消费项目读取。首50条Runner或Grant不作为目标不存在的依据。

Owner工作台只从原readiness.scope当前approved Grant元数据预选准确公共scope，源Runner须当前active、身份可用。准备前明确点击并重读当前依据；本地最多30s保守快照减请求耗时、夹身份到期，只控制展示及进入原准备，不是服务端授权租约。没有自动轮询、Grant扩大、配对、旋转、探测或启用。foreignowner／消费项目受限不提升为项目权限，原can_open_project仍是回项目唯一门。

原准备表单带入唯一公开tenant/project及Grant，私有路径、Shell、文件及预算均由原表单明确填写，不猜远程机器路径。打开的同source表单在只读资格到期时保留用户私有草稿，禁生成／复制／继续并允许重读；真正caller／组织／Pool／Runner／原scope来源变化按原规则清空，不能为保值继续采用旧权威。

原Connection仅对同source tuple保留返回pin，否则沿原通用项目工具定位作为新接入。回项目后重读准确profile版本／SHA／Worker、当前shared tuple和原诊断；原未确认写入／未知检查优先核对。原Quick保准确换新source，不自动创建或启用。A→B→A及迟到读取用原单调owner隔离，原诊断截止在目录返回后继续核对。原pool-tool格式保留用于没有旧配置source的新接入，属于不同用途，不作历史版本分支。

## 桌面真实离开与原收尾

当前owned BrowserWindow的renderer beforeunload由原生主进程控制器接住。原生文案中英固定，默认Stay、Escape／取消／错误／来源变化均Stay，不读取renderer传来的敏感内容或授权令牌。

已准确捕获的close／quit／同源主frame导航，先取消原卸载，再异步显示一个有父窗口的原生对话框。明确Leave后仅当当前window／webContents／origin／document epoch／意图都一致才精确重放一次；新will-prevent-unload同步消费一次许可。不得await后对已经返回的旧event调用preventDefault，不允许永久bypass、destroy或猜另一目的地。

公开事件未提供准确意图的reload等情况采用Electron官方showMessageBoxSync，仍默认Stay，在当次事件同步决定；这类原生对话框会短暂阻塞主进程，不能宣传全部异步。外部来源不授予放行；单一pending不重复排队授权。

app.before-quit先请求原window.close，经renderer／用户明确离开且实际closed后才原唯一disposeManagedRunner／installedTool与stopService／最终quit。Stay不能先拆IPC、停止服务或运行节点。Leave只是离开页面，不是业务验收或任务已停止；最终关闭仍沿原Journal、停止回执和排空语义，不制造成功、清未知或重启进程。

## 必要验证与证据

精确Runner接口验证当前权限／撤权／跨Pool／不存在／页外目标／闭合query／无secret及同一生成合同。Pool与工具交接验证准确来源、owner受限、页外Scope、TTL保草稿／禁采用与重读、A→B→A、原未知优先及合法新接入。桌面验证默认Stay、单一pending、明确一次重放、来源变化及实际close后原drain；真实Electron证明事件和存储保留，注入对话回应或模拟服务的范围分别记录，不能冒充真人OS按钮或完整桌面／Worker验收。

只复验受影响检查，不重复整包或厂商模型；完整目标、Multica参考与许可、跨主机／数据库／安装／规模／生产仍分别留正式证据。
