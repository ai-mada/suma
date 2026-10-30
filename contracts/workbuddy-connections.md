# WorkBuddy 授权连接与单次令牌操作

T-WORKBUDDY-CONNECTIONS-147，初始产品1.0.0。沿[143原任务提交](workbuddy-cloud-submission.md)和[唯一实施计划](../docs/delivery/implementation-plan.md)实现正式授权连接，不另建执行、预算或验收权威。连接代表已接纳的用户授权；它不代表工具已可派发、云任务已停止、模型健康或生产验收。

## 身份与职责

`WorkBuddyConnections`组合现有`Service`、操作员固定的应用配置及独立主密钥。数据库仍是原控制面的事务权威。所有操作使用真实当前浏览器Actor、项目权限与原所有者；项目管理员也不能借读取接口获取其他用户的私有授权。列表和连接详情需要项目读取权；发起、兑换、刷新和断开需要当前项目写资格。原授权实例的只读查询可能返回私密链接，还必须核验原浏览器会话和当前项目写资格。外部`open_id`不构造速码Actor，不授予平台权限。

配置固定官方`https://www.workbuddy.cn`；普通请求不能指定provider URL、client secret、回调地址或HTTP头。构造器仅为受控本机协议测试接受显式literal-loopback origin。正式控制程序由已验证的`SUMA_PUBLIC_URL`派生精确回调`/api/v1/workbuddy-oauth/callback`，不根据Host／转发头推导凭据目的地。

## 公共接口

| 方法与路径（均在`/api/v1`） | 作用与结果 |
| --- | --- |
| GET `/workbuddy-connections?project_id&limit&cursor` | 当前所有者的安全分页；默认20、1–50；返回`configured`、`available_scopes`、`can_authorize`、`items`、`limit`与`next_cursor`，没有伪精确总数 |
| GET `/workbuddy-connections/{id}?project_id` | 当前所有者的安全连接事实及当前允许动作 |
| POST `/workbuddy-authorizations` | 强类型`WorkBuddyAuthorizationRequest`；返回201及准确授权实例，浏览器明确点击返回的固定授权URL |
| GET `/workbuddy-authorizations/{id}?project_id` | 原所有者、原浏览器及当前写资格下读取安全状态；有效pending实例才重建授权URL |
| POST `/workbuddy-connections/{id}/refresh` | `WorkBuddyRefreshRequest`，显式单次刷新，返回200安全事实 |
| POST `/workbuddy-connections/{id}/disconnect` | `WorkBuddyDisconnectRequest`，本地断开，返回200安全事实 |
| GET `/workbuddy-oauth/callback` | 无需会话的隔离回调落地页；不消费code、不交换token、不写业务 |
| POST `/workbuddy-oauth/callback` | 落地页同源内存请求；当前cookie、原视角、原会话nonce及当前项目写资格核验后才兑换；返回安全`WorkBuddyCallbackResult` |

其他字段、重复字段、无效数值／路径和超限输入明确拒绝。普通写请求携带稳定`idempotency_key`。已有对象请求`expected_version`为1–2147483646；新建授权的`connection_id`为空时版本为0。持久版本允许到2147483647；已有连接重新授权前须为Start、兑换准备及结果三次CAS，以及相应授权／令牌代次预留空间。兑换和刷新外发前至少预留两次CAS，断开预留一次；余量不足保留旧凭据、原操作和状态，不发送请求。当前允许动作复用同一规则。列表游标最多4096字符，绑定租户／项目／所有者，按稳定ID读取；不同页面是新的当前授权读取，不宣称跨请求快照或时间排序。

断开原因为非空合法UTF-8，最多2000个Unicode字符、8000字节且无NUL。Callback只允许`state`加`code`，或`state`加`error:access_denied`；不接受调用者提供项目、身份、令牌或操作ID。State最多256字节、code最多8192字节。回调POST使用由授权实例ID确定的原操作标识；不进入通用浏览器草稿／写恢复存储。

## 授权实例与回调

服务端生成随机实例ID，用独立主密钥HMAC绑定实例、连接、租户、项目、所有者、原浏览器credential摘要、应用身份、请求范围及数据库时间5分钟期限。只保存state摘要，可对仍有效的原实例重建同一个URL。回执只保存安全对象引用，不保存state、code、URL、client secret或token。授权码仅以摘要参与原请求核对。

`pending → exchanging`的准备事务先重核当前资格并持久取得一次发送许可，再在事务外调用固定OAuth端点。接纳响应时重新检查当前身份、连接版本、活动操作、授权代次和固定应用。重复回调不能取得第二次兑换许可；更换code、会话、操作标识或范围会被拒绝。期限到达不恢复发送资格。拒绝授权只取消原pending实例，不请求供应方令牌接口。

现有会话cookie保持SameSite=Strict。跨站首次GET可能没有cookie，因此GET仅交付隔离bridge：nonce CSP、无第三方资源、no-store、no-referrer，立即清除地址栏查询。Code／state只存在该文档的内存闭包；随后同源读取当前session和语言，再以原视角头POST。会话变更由原credential绑定拒绝。成功或已记录未知事实后，只定位带安全项目／连接ID的工具页面；没有postMessage、浏览器token存储或弱化原来源校验。

普通浏览器沿原同会话链路；桌面采用同Session隔离授权子窗口。窄IPC只传项目、授权实例、原用户及组织四个ID；主进程先读取当前session，再读取准确授权实例，核对当前身份、所有者、项目、pending与期限后才打开服务端返回的固定官方URL。本机client／scope配置只能收紧约束，不替代服务端连接权威。子窗没有preload／Node，其他弹窗仍拒绝；原state回调完成后，只接受原项目／原连接的安全回跳，关闭子窗并聚焦父窗，不覆盖父窗口草稿。打开或返回都不是已连接事实，仍须原连接读取核对。不能转移Cookie、code或token到外部浏览器。独立模块及本机原生验证见[窗口证据](../docs/quality/workbuddy-desktop-oauth-verification.md)；主程序接线、完整页面及厂商验收分别记录。

## 刷新、断开与未知结果

连接状态为`authorizing`、`connected`、`refreshing`、`authorization_unknown`、`refresh_unknown`、`reauthorization_required`或`disconnected`。操作状态为`pending`、`exchanging`、`succeeded`、`unknown`、`failed`或`cancelled`；授权读取可派生`expired`，不通过读取改写业务历史。

刷新在数据库中固定唯一活动操作和版本fence，不能用进程互斥代替跨副本争用。成功后`token_generation`递增，`grant_generation`保持；供应方未返回新refresh token时明确保留旧值，不把SDK自动填充值当作供应方轮换证据。身份变化或刷新后scope变化不能静默接纳，进入重新授权；重新授权不把旧连接改绑到另一个`open_id`。

请求超时、丢失结果或保存失败不能证明供应方未处理。`exchanging`、`unknown`或租约过期不自动重新POST；原持有者仅能在仍匹配的fence和当前资格下保存已观察响应。断开会清除本地密文、作废活动资格并推进授权代次；迟到响应不能恢复它。配置缺失、允许scope收紧或密钥不可用时，仍为当前有权所有者保留本地断开入口。断开不声称供应方已撤销令牌或远端任务已停止。

## 凭据与官方协议

复用已安装的`golang.org/x/oauth2`，固定`AuthStyleInParams`，关闭认证方式探测重试。每次兑换／显式刷新独立边界，只允许一个准确POST，禁止重定向和Cookie传播。响应先作大小、UTF-8、重复字段、Bearer、实际scope、`open_id`及到期校验，再只把白名单字段交SDK。实际scope可缩减，不能扩大到请求之外。SDK原始错误正文和令牌不能进入诊断。

密文使用AES-GCM与随机nonce，AAD绑定租户、项目、连接、所有者、应用、外部主体、scope、授权与令牌代次及到期事实。SQLite可保管独立本机密钥；共享数据库必须显式配置共同的`SUMA_WORKBUDDY_MASTER_KEY`。不回填旧凭据、不重置原数据、不静默改用明文。新两表使用原platform组件基线及严格结构／唯一键检查，未登记或不匹配结构在写前拒绝。

[WorkBuddy官方Open API](https://open.workbuddy.cn/docs/openapi)提供授权码和刷新端点。当前未核实云OpenAPI的PKCE、远端撤销、内省或供应方刷新令牌重放保证；[连接器认证](https://open.workbuddy.cn/docs/connector)的反向MCP OAuth不能替代此方向证据。

仍需独立交付ACP进度／输入／权限请求、当前授权网关、执行预算、云产物与固定代码候选、独立测试、厂商应用登记及真实多机验收。143的`CredentialVersion`应绑定授权代次，正常token轮换不能使已知task ID失去原身份。当前不注册云执行能力，也不将授权连接关闭为完整WorkBuddy支持。
