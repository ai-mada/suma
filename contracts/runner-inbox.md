# 执行节点配对收件箱与身份复用

T-RUNNER-INBOX-185；suma 1.0.0。接续 runner-pairing.md，保持原31项／P0–P7完整目标。本阶段复用既有配对、项目管理员权限、系统凭据和 Worker，不增加新的凭据或任务派发权威。

## 当前项目收件箱

`GET /runner-pairings` 仅当前真实浏览器项目管理员可用。查询：`project_id` 必填；`status` 默认 active，可选 waiting/proposed/approved/rejected/expired/all；`q` 为 trim 后最多200 UTF-8字节的字面搜索（ID、自报设备名称、Worker ID）；`limit` 默认20，范围1–100；`cursor` 为有界严格游标。先核验当前权限再解释游标或查询数据。

返回共享 RunnerPairingPage：project_id、status、q、items、total、limit、可选 next_cursor、as_of。total 为当前完整筛选计数；不是当前页长度。每页使用数据库当前时间，waiting/proposed 超期投影为 expired。active 为未到期 waiting/proposed。稳定按 created_at DESC、id DESC seek；游标绑定当前租户、项目、用户、规范化筛选和 limit，不能扩大查询范围。列表是实时分页，as_of 不构成历史快照，状态变化后刷新。

直接复用已保存 payload 的 created_at 与原三方言 JSON 字符串表达式，不增加表列／回填。使用已有 EnsureIndex 增加 tenant/project/state/expires 索引，仅优化范围与有效待办筛选，不宣称覆盖 JSON 排序。

浏览器当前详情和列表新增可选 reason_code：awaiting_machine、approval_required、control_changed、identity_approved、pairing_rejected、pairing_expired。它不进入保存原件、历史写回执和机器响应，不替代 can_approve 或正式节点在线／授权检查。控制地址漂移仍拒绝 approve 与机器 proposal/status；当前管理员可按原 CAS reject 未结束、未过期待办，仅关闭原意图，不注册身份或撤销已批准身份。

## CLI 复用

`--paired-runner ID` 是已保存默认身份的公开定位选择器，供原 `--report-tools` 或明确启动普通 Worker 使用。它定位原系统用户配置目录下该 ID 的 identity.json，不创建／配对／续期／发放凭据。与 `--identity PATH`、`--pair-runner` 互斥，空值和不合法 ID 在执行前拒绝。

加载器先校验完整元数据和 pairing_id 与 selector 相符，再读取系统凭据；显式 control/project/worker 继续准确断言，失败在网络和 journal 变更前结束。自定义路径保留独立的 --identity 入口。无新 SDK、凭据格式或明文 fallback。

## 简洁交互

原配对 modal 内增加筛选、搜索、下一页和刷新；选择记录只保存公开 locator，并重新读取详情。不能从陈旧列表直接批准或自动选首条；原 pending、持久化失败、权限拒绝与跨项目保护同样覆盖列表。审批保留自报提示、完整指纹与未勾选确认；收到机器 proposal 后将终端命令按需展开。

批准后的准确 pairing/control/project/worker 公共目标传入原工具接入流程，绑定当前用户／组织／项目，切换即清除。指南支持 environment、paired、identity 三模式：常用 paired 仅引用原 ID，高级 identity 可填自定义路径。paired 生成命令携带控制面／项目／Worker 断言，不猜服务地址或其他节点。

准备、单次上报、持续启动分别复制命令。启动草稿仅在用户于目标终端明确执行时启动原 Worker 循环，可领取已授权任务；浏览器不自动启动／Claim／启用配置。已批准、已上报、在线和可派发分别显示。

## 交付与证据

后端、CLI、Web 并行修改独立文件；Root 维护共享合同、生成接口、集成与 Git。只运行新增及受影响的定向检查，最后一次类型／中英文检查和代表性界面验证；复用未变184系统库实测，不重复厂商、全库或安装包验证。源码、合成界面、本机系统库及真实多机证据分别记录。
