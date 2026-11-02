# 共享资源池节点安全配对

状态：本阶段限定验证；版本1.0.0；阶段T-POOL-RUNNER-PAIRING-208。原31项／P0–P7完整目标继续。负责人：主集成／控制面／Runner／Web代理。

## 目的与权威

组织资源池管理员在原节点页创建10分钟待办。目标机器生成随机凭据并先保存至系统凭据库，以同一公开身份文件持久化原proposal操作后只提交摘要。管理员核对完整指纹、机器自报名称、登记名称、并发槽位和有效期，按准确版本明确批准。批准事务只在原Pool节点与凭据表绑定该摘要，浏览器和数据库业务记录都不接收明文token。原手工登记保留独立次入口。

本领域目的固定`pool_runner`，独立于项目Runner配对。不能用空project、项目token或另一个机器身份取得资源池资格。配对不创建项目Grant、不扩大项目读取权、不自动运行Worker、上报库存、启用工具或派发任务。已有Pool双边授权、项目路由、资源预留、fence、未知效果核对及质量链完全复用。

## 冻结的类型与路由

唯一共享类型在`internal/poolrunnerpairingcontract/pairing.go`；平台使用别名，Runner复用同一合同。状态为`waiting|proposed|approved|rejected|expired`。response的`purpose=pool_runner`、owner_tenant_id、pool_id、pool_version、name、max_slots、ttl_seconds和control_url在创建时固定；机器仅提交device_name、credential_sha256和原操作键。approved才包含runner_id、worker_id、credential_version和credential_expires_at。`can_approve`／`reason_code`只属于当前读取，不写入正式事实或原receipt；机器响应恒can_approve=false。

| 接口 | 作用 |
| --- | --- |
| GET `/execution-pools/{pool}/runner-pairings` | 当前Pool管理员分页读取；status、q、limit、cursor |
| POST `/execution-pools/{pool}/runner-pairings` | CreateRequest；body pool_id必须等于路由 |
| GET `/execution-pools/{pool}/runner-pairings/{id}` | 准确当前授权读取；禁止其他query |
| POST `/execution-pools/{pool}/runner-pairings/{id}/commands` | approve／reject；CAS＋原receipt |
| POST `/pool-runner-pairings/{id}/proposal` | 窄机器入口；Bearer原随机token与声明摘要一致 |
| POST `/pool-runner-pairings/{id}/status` | 同一pool／原proposal操作／原token的只读核对 |

字段必须闭合，拒绝未知／重复／缺失／null及错误类型，机器调用不接受浏览器cookie代替原证明。机器接口在原常规Pool认证之前精确识别，不把未批准token交给PoolWorker认证或人类Actor。

Create：登记名称1–200字节、max_slots1–128、ttl_seconds3600–30天，沿原Pool验证；这三个值不能由proposal或approve变更。机器自报device_name沿原机器名称上限1–100 UTF-8字节，与登记名称分开。最多8个未到期waiting／proposed待办，以原组织／Pool锁和数据库时间串行限制。创建与批准要求当前Pool active；Pool版本变化阻止新批准，显示pool_changed；仍可由当前管理员拒绝未终结待办。配对TTL10分钟，读不续期。approved待办过期不撤销原Pool身份；机器后续核对必须重新验证当前凭据版本、撤销、期限、Pool状态与原摘要，不靠旧approved receipt复活资格。

列表默认status=active、limit20／最大100，q最多120字节；支持all、active与五种状态。按created_at降序、id降序稳定游标分页，游标绑定owner／Pool／过滤／最初as_of，重放不跨范围。历史元数据不代表当前在线。准确GET与列表授权均为当前purposeOrganizationWrite＋poolAdminTx；不把授权receipt当作未来授权。

## 同事务与恢复

从原RegisterExecutionPoolRunner最小提取`registerExecutionPoolRunnerDigestTx`，共享当前Pool管理权限、1000有效节点上限、原runner／credential两表和登记事件。原手工注册继续首次响应返回随机token；新配对只传机器摘要。不能调用会另起事务的公开注册函数冒充原子性，也不能借用项目credential Grant helper。

批准、唯一节点登记、摘要凭据、配对CAS、原operation receipt与事件同事务。create／command先核当前权限，重复原body／key恢复原结果；版本冲突不换key自动重做。proposal提交成功ACK丢失仍重放同一持久正文／操作和token；被其他机器绑定则拒绝。状态读取不能延长期限或重新发密钥。批准后轮换／撤销使用原Pool节点管理；原身份仍保留用于核对，不自动重新配对。

## 本机与CLI

复用固定go-keyring0.2.6的RunnerSecretStore，仅读写专属service／account，无明文降级、无枚举或删除其他凭据。独立公开协议`suma-pool-runner-identity-v1`包含purpose、control_url、pool_id、pairing_id、proposal_operation_id、device_name、credential_sha256；批准后保存原runner／worker／credential_version／expiry／max_slots。metadata不含token、project_id或项目Grant。账户键绑定目的、准确控制面、Pool、pairing和规范化文件位置；损坏／链接／占用或凭据缺失时拒绝，保留原件。

`--pair-pool-runner ID --control URL --pool POOL [--pool-identity PATH] [--device-name NAME]`只配对；未指定路径时使用系统用户配置目录中的独立Pool身份位置。原proposal持久化后才HTTP，2秒轮询、最多10分钟，取消保留身份与原凭据。控制地址仅接受operator-controlled HTTPS或回环HTTP，无重定向／userinfo／query／fragment。

`--pool-identity PATH`配合原`--pool-config FILE`供显式Pool启动。必须显式提供`--control`并与批准metadata准确匹配，原Pool配置中的pool_id／runner_id也必须逐项相同，之后才读取专属凭据；原配置没有control_url字段，不新增重复控制地址权威。不读取环境token作为备用，不与项目identity／pairing混用。原配置继续保存每项目仓库和允许命令，默认state从准确身份文件派生`.journal`；原Pool Session和调度不变。身份文件中旧批准不代替服务端当前认证。本机describe不需要身份秘密，不与Pool身份启动混用。

## 简洁交互与分工

原Pool节点tab的主入口为“配对执行机器”，手工登记为次入口。一个短表单固定name和显式展示默认槽位1／凭据24小时，可调整；随后是目标Shell命令、等待机器、完整摘要复核与批准／拒绝。刷新恢复公开locator；原未知write始终可恢复，读取绑定actor／organization／Pool／caller epoch，不能使用假的project给原项目配对组件。锁定事件接入原parent Modal和离开保护，不把内部pending反向当作永久disabled导致死锁。

控制代理拥有新platform Pool配对服务／存储／HTTP／初始化与原credential helper提取；Runner代理拥有独立Pool身份／配对、必要底层私有复用与CLI；Web代理拥有独立Pool配对模型／卡／Panel、PoolWorkspace最小入口与双语；主代理拥有本合同、唯一wire类型、OpenAPI生成、实际整合、文档与Git。共享文件所有权先冻结，禁止跨写。

验收重点为权限隔离、当前Pool变化、事务唯一性、原ACK恢复、未知结果、系统库引用、元数据损坏、CLI scope拒绝、代表中英界面和原Pool Claim路径复用。先做定向新／受影响旧检查，不重复厂商模型或质量全链。注入凭据库、真实系统库、同机HTTP／SQLite、物理多机和正式交付分别记录；完整平台不因本阶段配对完成而关闭。
