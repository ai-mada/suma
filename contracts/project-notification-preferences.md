# 项目个人通知偏好合同

T-NOTIFICATION-PREFERENCES-167；速码1.0.0；2026-10-31。负责人：主集成代理。状态：个人项目讨论设置及限定真实业务／视觉核验完成。完整31项需求／P0–P7／58限定＋34待交付＝92项不变。上一阶段166已提交并推送`1af836a`；本阶段继续Multica团队协作能力，不将连接指引当作完整工具执行。

## 产品行为

个人收件箱提供一个项目通知设置入口。当前成员可静音**当前项目中已关注任务的新讨论通知**；它不取消任务关注，不隐藏历史收件箱记录，不影响精准提及、交接、验收、告警等其他来源。人／用户API消息及正式保存的最终assistant消息共用原关注投影；流式模型片段与运行日志仍不产生讨论通知。

设置仅属于当前账号＋组织＋项目。viewer可管理自己的偏好而不获得业务写权；浏览器／当前用户API可用，Runner／Agent／Pool身份及代他人写入拒绝。每次读取、写入与原操作核对都重验当前项目权限；请求不能指定user／tenant／recipient。项目切换不会把一个项目的选择应用到另一个项目。

## API与数据合同

- `GET /api/v1/notification-preferences?project_id=...`：仅返回当前人的 `ProjectNotificationPreferences`，没有记录时version=0、followed_discussions_muted=false、updated_at为空，不写数据库。
- `POST /api/v1/notification-preferences/commands?project_id=...`：闭合 `ProjectNotificationPreferencesCommand`，四个字段全部必需；未知／重复字段、null、无效ID／版本／操作标识及额外查询参数拒绝。正文project_id必须与URL一致。

```typescript
type ProjectNotificationPreferences = {
  project_id: string; user_id: string; version: number;
  followed_discussions_muted: boolean; updated_at: string;
};
type ProjectNotificationPreferencesCommand = {
  project_id: string; expected_version: number;
  followed_discussions_muted: boolean; idempotency_key: string;
};
type ProjectNotificationPreferencesReceipt = {
  idempotency_key: string; preferences: ProjectNotificationPreferences;
};
```

服务方法为`GetProjectNotificationPreferences(ctx,actor,project)`与`CommandProjectNotificationPreferences(ctx,actor,project,req)`。版本范围0…2147483647；操作标识沿现有8…128非控制字符约束。原请求与key绑定，当前权限核验后先恢复原回执，再处理当前版本；相同key的异正文拒绝。CAS冲突不自动换版本重发。

真正改变时版本加1，其他情况下回执保留原版本及时间；首次取消默认静音状态的无副作用操作保持version0且不新建偏好行。旧回执只证明原提交，不能覆盖后续设置；恢复后另读当前状态。不创建任务、运行、通知或业务活动来“验证设置”。

单独初始领域表`suma_project_notification_preferences`：id、tenant_id、project_id、user_id、version、followed_discussions_muted、updated_at；id主键，tenant/project/user唯一。初始结构验证在任何DDL之前，复用SQLite／OceanBase／PostgreSQL方言与现有基线守卫；没有历史兼容分支、回填、重置或覆盖用户数据。

## 投影与并发

仅在`projectTaskSubscriptionFollowersTx`的当前有权关注者集合查询中按同tenant/project/user排除静音成员；不要在通用通知插入器、提及投影或读收件箱时过滤。不逐接收人重新获取身份锁，不改变原组织／项目授权与锁顺序。偏好命令复用当前项目权威事务，消息与偏好并发按同一权威序列确定；设置生效只影响其后提交的讨论投影，不能补发静音期间历史消息。

精准提及先沿原事务投影；关注通知仍与提及去重。偏好命令及no-op回执原子提交，关注投影仍与消息／正式assistant结果事务一致。其他通知来源与现有来源核对、权限过滤、未读计数和独立已读命令不变。

## 简洁交互与恢复

收件箱标题旁一个“通知设置”入口，默认收起；展开后只显示“静音关注任务的讨论通知”开关、当前项目范围和不会影响精准提及／交接的说明。沿现有React／Lucide／设计令牌，中英文完整，窄屏单列、键盘可操作，状态不只依赖颜色。入口显示当前设置，不能把未知读取当默认false。

成功写入后显示确切回执并重读当前状态。沿useWriteAction保留原body／path／key；未知提交明确核对，防重复点击，存储不可用不外发，冲突保留原意但不自动重试。账号／组织／项目／API caller变化同步退休旧数据与旧响应；401／403／404清除受保护内容。刷新不发写请求，标已读与修改偏好分开。

## 分工与限定验收

Go代理独占新`project_notification_preferences*.go`领域、HTTP、schema与定向测试，以及原`task_subscription_projection.go`的一处集合过滤。Root独占database初始化接线、http路由、localization目录、合同生成器／生成类型、canonical文档及Git。Web代理独占新ProjectNotificationPreferences组件／model／双语域和NotificationInbox最小入口接线；不改原21项未提交文件。实际联调／审阅代理只使用本阶段忽略目录，独立运行范围由Root在源冻结后分配。

必要验收：viewer／当前用户API／撤权／跨项目与机器身份；只读不写、默认／真实改变／no-op、CAS、同key恢复／异正文、旧回执不覆盖新状态、消息与偏好并发；human／最终assistant关注静音、精准提及保留、其他项目及历史未读不变；初始schema在写前拒绝残缺结构；闭合HTTP、同源OpenAPI／TS、当前范围异步隔离和中英代表性交互。只跑改变路径的必要检查，不重复166或无关全套。真实SQLite与受控实际界面、其他数据库、多机及生产分别记证据。
