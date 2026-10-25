# 平台显示诊断合同

状态：T-P4-013 冻结实施基线。关联FND-I18N-001；完整目标继续以`docs/delivery/implementation-plan.md`及已批准P0–P7方案为准。该合同先实现工具当前资格与图控件，后续持久执行原因、生成业务正文、CLI／桌面／安装／渠道分别贯通，不以此缩小平台范围。

## 事实与显示

原始`reason`、工具Observation、执行／业务状态、原件、事件、幂等请求及历史回执不因语言变化而改写。当前资格说明由控制面受权读取产生；原生工具诊断仍遵守原native合同，不放宽其命名空间或来源证明。用户／工具文字恰好匹配译文或模板，不构成平台来源。

新增只读`ToolProfile.reason_diagnostic?: DisplayDiagnostic`，`DisplayDiagnostic`位于`internal/displaycontract`：

```ts
type DisplayDiagnostic = {
  schema_version: 1;
  message_key: string;
  message_params?: Record<string, string | number>;
};
```

元数据与同对象的原始`reason`绑定。schema/version、闭合字段、领域key白名单、精确参数名／类型／既有目录边界及完整规范原文必须同时成立。显示身份不授予权限，不改变Available、派发、预算、测试、重试或验收。无身份、未知版本、参数错误、目录未加载或无法证明的内容保持原文。原生ToolDiagnostic和通用APIError保持原合同。

Go权威目录增加以下16个`display.tool.*`键，`SourceTemplate`必须显式填写，不注册为全局原文别名。规范原句与现有生产者逐字相同；显示文案可以改善措辞，但不能扩大资格含义。

| 后缀 | 规范原句 |
|---|---|
| profile_revoked | 工具档案已永久撤销 |
| profile_paused | 工具档案已暂停，尚未授权派发 |
| activation_revoked | 工具档案启用人的当前审批权限已失效 |
| worker_expired | 执行节点凭证已过期或撤销 |
| worker_creator_revoked | 执行节点登记人的当前项目权限已失效 |
| worker_unregistered | 执行节点未登记、未获项目授权或机器凭证已撤销 |
| pool_paused | 资源池已暂停，停止新派发 |
| pool_unavailable | 资源池当前状态不允许新派发 |
| pool_expired | 资源池执行节点凭证已过期 |
| inventory_missing | 执行节点尚未上报工具清单 |
| inventory_stale | 工具清单已过期，等待执行节点重新探测 |
| command_missing | 当前执行节点未声明此命令 |
| profile_mismatch | 当前工具配置、版本或程序摘要与固定档案不匹配 |
| reported_unavailable | 执行节点报告工具不可用：{{.reason}} |
| protocol_unverified | OpenCode 当前版本尚未通过平台执行协议验收 |
| dispatchable | 执行节点已报告可领取；模型认证、执行与验收结果需分别核验 |

`reported_unavailable.reason`为既有目录Text参数，外部文字是数据，不能递归解释为键或翻译。当前Text边界（非空、合法Unicode、512字节内、无控制字符）外的原始Observation仍可合法保存；只丢弃不能证明的显示hint，完整原文保留。不得截断参数、抹掉外部原因或把它标为平台认证。

Go接口固定为：

```go
type DisplayDiagnostic struct {
    SchemaVersion int `json:"schema_version"`
    MessageKey string `json:"message_key"`
    MessageParams map[string]any `json:"message_params,omitempty"`
}
type SourceDefinition struct {
    Key string `json:"key"`
    Parameters []localization.Parameter `json:"parameters"`
    SourceTemplate string `json:"source_template"`
}
func BindToolReason(source, key string, params map[string]any) *DisplayDiagnostic
func ValidateToolReason(source string, value *DisplayDiagnostic) *DisplayDiagnostic
func SourceDefinitions() []SourceDefinition
```

构造、验证及返回定义必须深复制可变参数／切片，完整规范原句由现有localization模板引擎验证。Bind失败返回nil，不影响业务事实；只能由具体平台生产者调用，不对任意存储文本执行反向LookupSource。SourceDefinitions仅导出上述显式批准的16条规范定义，确定排序，无时间戳；不暴露全目录私有SourceTemplate。

## 服务端读取与回执

在现有受权事务内，为所有ToolProfile资格分支及create的paused说明附加身份，包括项目Runner与共享池Runner的权限、凭证和库存分支。允许重构内部reason返回以携带可信字段；不改变权限或资格判定。ToolProfile和ToolProfileList不再经localizePresentation覆盖原始Reason；其他显式展示投影继续原行为。

GET列表／详情以及新建／启用／暂停／撤销响应共享同一来源构造。旧持久操作响应没有hint时保持原样，不回写、不替换其原始reason；新操作请求哈希和幂等语义保持。ToolProfileCreateRequest、命令、Observation不接受调用者注入reason_diagnostic；真实读取、命令权限和CAS仍由服务端核验。语言是显示选择，不创建额外读取或执行。

## 前端显示与图控件

Root从同一Go来源生成`packages/contracts/src/display-sources.ts`：导出`displaySourceDefinitions`，每项恰含key、parameters、source_template。Root同时生成正式DisplayDiagnostic及ToolProfile类型与OpenAPI；任何手工维护的第二套规范原句均不接受。

新增独立`renderDisplayDiagnostic(source: string, diagnostic: unknown): {text:string;localized:boolean}`。只消费上述批准来源、严格参数和同对象raw reason；用生成的完整模板重建并精确匹配原句，再用已加载的正式目录显示。目录迟到允许重显，无效hint不得兜底调用不核对原文的通用renderDiagnostic。不得对普通业务title／description／output启用该renderer。

ToolProfileFacts、ToolBindingFacts和配置／启用／暂停对话框共用renderer，保留独立“原始原因”披露；无hint的历史响应保持原文，原有六个机器reason枚举可保持其既有明确映射。未知原文不可被泛化提示吞掉。语言切换不重取、不写入、不重建草稿、行或仍存活的披露／焦点；保持现有主题、44px窄屏操作及加载／失效状态。

React Flow使用官方`@xyflow/react`固定12.12.0（registry元数据及校验摘要先保存至ignored证据）；匹配官方`@xyflow/system`0.0.83，维护锁文件，使用安装时禁用生命周期脚本及仓库内缓存。React19继续满足官方peer约束。安装后独立检查解析版本、原tarball integrity、许可、公共类型及依赖差异，不能凭搜索摘要宣称兼容。

使用正式`ariaLabelConfig`覆盖该版本暴露的全部内部文案，包括node／edge说明、移动播报、controls按钮、minimap／handle等。可见控制按钮title和ARIA同时切语言；动态参数保留节点标识与坐标。保留React Flow署名／许可证及编辑与执行的真实键盘语义；不操作私有store／DOM、不禁用键盘来隐藏英文。布局、节点选择、精确attempt、viewport和dirty draft在语言切换时保持。Monaco本轮不改，后续单独核验官方ESM/NLS与语言服务及无损草稿切换。

## 验证与归属

后端独立任务拥有新displaycontract、Go权威目录、ToolProfile生产者／投影与对应Go测试；不改生成器或前端。前端独立任务拥有renderer／配置消费者、React Flow与依赖锁、本地化及对应测试；不改Go／生成器／生成文件。Root拥有本合同、生成器／生成结果、设计文档、实际服务和浏览器验证及阶段必要文件commit。共享同一仓库按不重叠文件并发，禁止worker提交／push／派生代理；源码保持可审查。

必须有实际RED→GREEN：所有16种资格说明正确身份、raw Reason在中英文HTTP并发下相同、参数快照、canonical源不符／未知键／原生键／多余参数／坏Unicode／超界外部原因安全fallback、普通API注入hint拒绝、原回执／Observation／计数不变。前端真实消费者验证locale与目录迟到、无额外GET／POST、raw披露／focus／draft、known拒绝，不能只测实现镜像。

图测试不能mock ReactFlow或Controls为null然后宣称内部语言通过。真实DOM／浏览器检查控件title及ARIA、node／edge describedby、键盘播报、Enter／Space／Arrow／Escape、只读执行与编辑差异、四宽度及主题，保持原任务／attempt选择。完整套件／类型／合同／构建、独立任务复核及全变更复核后，再启用实际新控制面／Runner做资格生命周期与图页面检查，原数据库与执行计数对比。实际执行原因、系统问题正文、CLI／安装器／渠道、Monaco、分布式与全部P0–P7仍逐项实现验收。
