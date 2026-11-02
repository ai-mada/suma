# Claude SDK 快接架构、部署与 AI SOP

状态：T-CLAUDE-SDK-CONNECT-201限定实现与验证完成；[验证报告](../quality/claude-sdk-quick-connect-verification.md)；suma 1.0.0；关联[PRD](../product/claude-sdk-quick-connect.md)、[合同](../../contracts/claude-sdk-quick-connect.md)、[交互](../design/claude-sdk-quick-connect.md)。

```mermaid
flowchart LR
  A[原生CLI被动发现] --> B[明确模型与环境引用]
  B --> C[主进程重核CLI及可信资源]
  C --> D[原加密Journal与固定节点]
  D --> E[Go复核完整摘要与Git基线]
  E --> F[原Worker的新鲜库存]
  F --> G[准确暂停配置登记]
  G --> H[独立明确启用]
```

Web负责公开意图、当前scope和请求代次，主进程负责私有配置与执行文件来源，Go负责再次验证完整资产、配置摘要和既有工作区限制。控制面仍以原权限、Worker、库存及配置版本决定登记和派发。没有新的配置数据库、独立SDK调度器或第二写恢复机制。

原生CLI与Node是不同资产。CLI扫描只证明程序存在及版本；SDK连接回执同时保留二者不同摘要。Go根据原BuildClaudeSDKCommand固定SDK宿主命令，完整摘要覆盖本机CLI路径、环境引用、Node／CLI／manifest字节和固定版本。浏览器不掌握所有私有路径，因此不复算完整摘要，也不能以模型／环境名称的小摘要代替。

当前setup来源通过可信本机IPC回显的原持久环境变量名称，与当前公开模型／原生SHA共同核对。名称不是密钥值，读取仍先验证当前项目／身份。不能凭同模型／CLI猜测环境引用；完整connected匹配才能恢复原证明，partial starting仅显示进度。首次ACK丢失或页面重开不应强迫停止再连才能恢复同一配置，也不以renderer另建关联或弱摘要弥补缺证。主进程不得在活节点上静默切换新setup，不因回执未知另发新身份。

## 资源拓扑

```text
control/
  suma-control(.exe)
  claude-runtime/
    node(.exe)
    NODE_LICENSE.txt
    desktop-runtime.json
    host/
      suma-claude-runtime.json
      suma-claude.bundle.mjs
      <manifest列明的全部许可证与资源>
```

host目录是严格资产闭包，桌面收据和Node不能放入其中。打包流程复用现有源清单、目标格式、Node版本与许可证核验；仅向任务隔离目录准备资源。开发态也只能使用仓库受核宿主与同目标Node，不能接受渲染器指定任意路径。交叉打包只证明目标格式，不代表目标OS安装／运行已验收。原生Claude CLI不随SDK资源自动下载安装。

## AI开发与排障 SOP

1. 先核当前本机扫描的adapter、product、版本与SHA，不以CLI扫描断言SDK或模型可用。
2. 固定模型和明确环境引用；不读取登录文件、不把密钥值放入命令、Journal、日志、回执或仓库。
3. 主进程核对准确扫描及受信资源，再沿原连接操作持久意图；Go重新核资产和完整摘要。任何漂移先明确失败，不能创建可用假象。
4. 仅完整connected、当前代次与准确模型／环境名称／原生摘要匹配才确认当前setup。未知先查看／恢复原操作；没有准确环境名称时，不能将新草稿附到旧连接。
5. 本次启动后的新库存需匹配worker、Node SHA、原正式Profile.tool_version=2.1.314及完整SDK能力；宿主自身版本输出不替代正式工具版本。登记保持paused。固定配置、模型认证、开发结果和质量门分别取证。
6. 配置变更先完成原节点实际停止和未决结果处置，再新明确连接。旧配置换新仍沿T200正式交接，不重写历史计划。
7. 故障收据区分扫描、资源、配置、注册、库存、停止和未知；保留原ID／SHA／操作记录，不清用户数据或放宽权限来恢复通过。

实现复用成熟官方SDK、已有Go驱动与桌面IPC／Journal，不新增依赖。参考采用见[Multica记录](../integrations/multica-claude-sdk-connect.md)；执行和质量合同继续见[T192](claude-sdk.md)。
