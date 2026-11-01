# 远程执行节点配对

T-RUNNER-PAIRING-184；suma1.0.0。原31项／P0–P7完整目标继续。接续182自动配置与单次上报，复用原项目管理权限、24小时独立Runner Grant、节点登记、库存与Go派发；不改桌面HMAC／回环限制，不增加任务派发权威。

## 确定流程

浏览器在当前项目明确创建10分钟配对待办，复制不带凭据的命令。在目标机器执行后，Runner先在系统凭据库保存随机token，原请求和公开身份元数据持久化后才提交摘要。浏览器刷新该原待办，显示机器自报名称及完整摘要指纹；管理员核对后按原版本明确批准。服务端原子创建原Worker与24小时Runner Grant，仅保存摘要。机器凭原token确认原登记元数据，保存公开身份并退出。其后通过`--identity`复用原单次报告或显式启动普通Worker，配对不自动报告、Claim、启用或运行任务。

节点登记、凭据当前资格、库存新鲜、持续运行、配置启用、模型登录与业务验收分别表达。机器名称是自报信息，locator自身没有授权能力；不得省掉最后浏览器核对或把配置登记显示为在线。

## 固定HTTP与类型

共享类型在`internal/runnerpairingcontract`，平台公开别名与Runner复用，不复制两套合同。状态闭合为`waiting|proposed|approved|rejected|expired`，版本从1递增。`can_approve`仅为本次当前浏览器读取的资格；机器结果恒false。

| 接口 | 身份与效果 |
| --- | --- |
| POST /runner-pairings | 当前真实浏览器项目管理员；RunnerPairingCreateRequest→201 RunnerPairing |
| GET /runner-pairings/{id}?project_id=… | 当前真实浏览器项目管理员；同租户同项目准确读取 |
| POST /runner-pairings/{id}/commands | 当前真实浏览器项目管理员；RunnerPairingCommandRequest，action approve／reject，原CAS及receipt→200 RunnerPairing |
| POST /runner-pairings/{id}/proposal | 窄公开机器入口；RunnerPairingProposalRequest+Bearer原本机token，SHA须一致；只允许该既存待办首次绑定原项目、名称、摘要、操作键；准确重放恢复，其他主体冲突 |
| POST /runner-pairings/{id}/status | 窄机器读取；RunnerPairingStatusRequest+Bearer同一原token，只读该原proposal元数据；批准后须重新核验原Runner当前资格，不能凭旧批准恢复过期／撤销身份 |

控制地址由operator-controlled Server.PublicURL固定，保留准确路径；仅HTTPS或本机回环HTTP，无userinfo／query／fragment。不从Host／转发头或客户端批准体选择地址。无法提供安全地址时明确拒绝创建。locator为随机32位小写hex；每项目最多8个未到期未结束待办，创建必须已认证，机器不能任意新增待办。

创建及批准先核当前授权再查原receipt。approve仅接受未过期proposed原版本；同事务登记服务端生成的remote-节点、无发布profiles／deployment、原独立Grant／slot／digest／Runner scope及metadata receipt。reject不撤销已批准节点，撤销仍走原节点入口。所有结果都不含token；浏览器不接触机器秘密或cookie转交。不能复用首响应才返回token的普通RegisterReleaseWorker，也不能用重放会换token的auth/tokens充当恢复。

机器proposal／status仅在准确ID、项目、原操作键和摘要证明一致时返回；不能列举项目或查别的机器。提交后ACK丢失沿原请求恢复；不替换候选摘要、不重新发token。不支持的字段、冲突版本、过期或拒绝均明确失败。批准后的短效待办过期不撤销已发Grant，但机器读取仍须当前Grant有效；TTL不能复活原身份。数据库时间控制期限，读不延长它。

## Runner与凭据

`suma-runner --pair-runner ID --control URL --project ID [--identity PATH] [--device-name NAME]`只配对；未指定identity时定位系统用户配置目录下该配对的公开元数据。默认机器名取本机hostname并作有界校验。网络轮询每2秒按原proposal操作读取，最大至10分钟／服务端期限，取消退出保留原件；系统凭据库可能需要先由用户解锁。stdout为`suma-runner-pairing-v1`有界非秘密状态，必含已保存的credential_sha256供终端与浏览器完整比对，复制命令不含token。重启同命令恢复原identity，不重提新请求。

使用固定`github.com/zalando/go-keyring v0.2.6`；Windows凭据管理器、macOS Keychain与Linux Secret Service由SDK适配。仅Set/Get当前专有account，不枚举或删除用户其他凭据。该版本macOS通过security -i的stdin提交，不将秘密放入argv。无安全后端、身份已存在而secret缺失、摘要不匹配时拒绝，不降级明文文件。独立测试只用注入provider，真实系统库检查另计。

新身份元数据协议`suma-runner-identity-v1`：control_url、project_id、pairing_id、proposal_operation_id、device_name、credential_sha256，批准后增worker_id／credential_expires_at；文件不含秘密。使用原本机锁／有界严格读取与原子写，拒绝链接、越界和损坏。先持久系统库和完整原请求再HTTP；网络发生后不能换原正文或键。系统库account绑定准确控制面、项目、pairing与身份文件位置，防止其他identity路径覆盖它。

`--identity PATH`可供原`--report-tools`及明确启动普通Worker使用。读取批准元数据并从系统库取原token；显式提供control／project／worker时必须准确相同，否则在网络与journal变更前拒绝；通用CLI未给时采用身份中的准确值。浏览器项目指南生成identity上报命令时仍携带当前公开control／project断言，不能让另一项目或服务的身份文件无声改变当前页面的目标。state默认置于准确identity文件的独立`.journal`目录，原inventory scope约束不变。配对模式拒绝与探测／执行模式混用，空flag不能落入Worker.Run。过期续期与重新配对是独立后续授权，不自动重发凭据。

## 简洁交互与验收

在原Worker登记modal增加远程配对入口，不另造管理页；当前项目自动固定。创建后只展示目标shell命令、等待机器、原摘要审核和准确结果。unknown写上报parent locked，保留原body/key；读取有caller/project/abort/stale保护。公开locator按原user／organization／project保存以支持刷新，秘密不进入通用pending或sessionStorage。批准后准确刷新原workers，并明确回原配置复用／登记及独立启用；不猜第一节点。

后端、Runner、Web并行拥有独立文件；Root拥有本合同、共享类型、go.mod/go.sum、生成接口及最终集成。只跑新权限／CAS／重放／期限／本机安全持久化与消费者必要定向检查；一次最终类型／词条和代表交互，复用未变SDK与质量链。真实系统库、受控HTTP／SQLite、合成UI、物理多机及厂商执行分别留证，不能互相替代。完整平台继续交付。
