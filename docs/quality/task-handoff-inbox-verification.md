# 交接请求收件箱与准确定位验证

T-HANDOFF-INBOX-148，初始产品1.0.0，2026-10-30。状态：scoped_verified_visual_image_pending；限定实现、检查及实际GUI／HTTP通过，视觉图片和最终Git交付待记录。负责人：任务协作／通知／前端交互与主集成。完整P0–P7、需求1–26及58 limited／34 pending／92总项保持进行。

## 验证范围与来源

[合同](../../contracts/task-handoff-inbox.md)、[产品](../product/task-handoff-inbox.md)、[架构](../architecture/task-handoff-inbox.md)、[交互与AI SOP](../design/task-handoff-inbox.md)及[English](../en/task-handoff-inbox.md)对应原交接事务内的请求通知、私有反向绑定、准确读取和任务详情接续。待接收通知不改变负责人；接收后的task_assigned独立，责任接收不等于正式Review。

最终候选从已提交基线`a7f5b01784313aabab2ee693fe2255b2da3e9af2`加明确148文件构成，独立生成合同和构建，147保持独立进行，本候选排除147、桌面、控制入口及其他未提交WIP。来源摘要为`387404bba5b20ed091f42f6ad4ac5895b9d5771abf93e6c27f684e9358600562`；不是整个活动工作区的摘要。冻结清单位于`.cache/e2e/handoff148/candidate/source-snapshot.json`，完整命令、退出码、日志与制品摘要见[机器摘要](task-handoff-inbox-summary.json)。

## 已核对的限定结果

| 检查 | 实际结果与计数边界 |
| --- | --- |
| 平台回归 | 44个独立顶层、36个子场景通过，含子场景共80个；新增8个顶层已包含，不是全后端套件 |
| 同源CLI | 候选源中1个顶层及10个子场景通过，覆盖准确GET、闭合查询和原写合同 |
| Web组件／模型 | 43个独立用例通过；收件箱29、初始面板／模型13、评审新增1；受影响4项复验包含在43内 |
| 导航 | 最终候选源5项独立定位／离开保护检查通过，和上述43项分开；合计48项限定Web检查 |
| 合同 | 集成工作区cmd/suma-contracts完整包检查通过；最终候选独立生成及`--check`均为746模型／346路径，并复验148准确合同 |
| 类型与语言 | 最终候选`tsc --noEmit`退出0；7323对词条及占位／复数／标记检查通过，生产硬编码0 |
| 隔离构建 | 最终候选Web构建退出0，52.34秒；独立控制程序构建退出0；大分块提示保留，不声明性能达标 |

候选源跨平台包／合同／CLI的10个顶层、26个子场景是准确来源复验。其中平台8个顶层和合同1个为已有覆盖，CLI1个已列在CLI行，不再次增加独立数量。完整cmd/suma-contracts包记录来自集成工作区；最终候选只称已执行的准确合同复验，不称其完整包或全平台套件通过。

主要命令均来自已有回执，本次仅整理证据，未重复执行：

- 平台：`go test ./internal/platform -run '^(TestTaskHandoff|TestNotificationInbox|TestTaskReview|TestTaskConversationMention)' -count=1 -json`。
- 合同完整包：`go test -json ./cmd/suma-contracts -count=1 -timeout=90s`；候选生成：`go run ./cmd/suma-contracts`及`go run ./cmd/suma-contracts --check`。
- 候选复验：`go test -json ./internal/platform ./cmd/suma-contracts ./internal/cli -run '^(TestTaskHandoffInbox|TestHandoffsRequestContract)' -count=1 -timeout=120s`。
- 组件：收件箱两个文件完整运行；面板／模型13项和新增准确读取失败回归，最后只复验4项受影响决策。准确命令见机器摘要；导航报告来自最终候选的专有测试文件。

## 覆盖的业务与失败边界

原请求通知、pending槽位、活动、回执和序号同事务；恢复不重复通知。私有payload.notification_id持续保留，不进入公共DTO／metadata，无DDL或历史回填。同tenant／project／task、行与payload身份／版本／状态、原接收user／resource、原任务版本及反向ID必须一致，损坏来源隐藏跳转，SQL故障仍报错。

准确GET只接受project_id和task_id两个单值规范查询，核对准确任务并重算当前动作；跨任务、机器身份、越权及未知／重复参数拒绝。数值原始lexeme继续进入强类型解码，不把小数版本舍入为合法CAS。

收件箱明确定位task-handoff:id，独立读取页外请求，不遍历历史或更改页计数。准确读取失败不能用旧历史页恢复决策；当前caller失权清除受保护来源，旧响应不能恢复。发起／决策草稿分开保留，原回执只清理匹配部分，版本采用与接收影响确认分别明确完成。中英、焦点和44px样式有组件／源码检查，实际布局仍需GUI。

## 红灯、修正与警告

- 最初三项后端检查准确暴露“未生成请求通知”；同时两项夹具误用不存在的收据表。先修正夹具后，回滚检查仍显示通知失败未阻止交接提交，准确GET仍404。接入原事务通知、反向绑定及准确HTTP后定向转绿，再完成44项顶层回归；失败记录保留。
- 收件箱最初2项红灯定位新kind／准确回调未接通；面板最初4项红灯覆盖页外焦点、独立草稿、明确版本采用等缺口。实现后29项与13项检查通过。
- 独立评审发现P2：准确读取失败后仍可能借历史项启用决策。新增回归先红，再要求每个编辑决策具有成功的准确来源；4项受影响决策复验通过，只有新增1项加入独立计数。
- 原收件箱未知写测试产生1条React act()警告；断言均通过。该警告保留，不声称零警告。Web构建的大分块提示亦保留。

## 实际GUI／HTTP与视觉图片边界

本机43961使用Electron35.7.5及已安装Playwright SDK创建新的自有隐藏窗口，不连接外部账号。`.cache/e2e/handoff148/browser/receipt.json`记录16项独立实际GUI／HTTP断言通过；原Go／Web／构建结果未重复运行或增加计数。

- 原来源pending；当前接收人从真实收件箱打开匹配project／task／handoff的URL，准确卡片取得键盘焦点。打开产生0次业务写，不自动已读，也不改变原pending／版本。
- 用户明确退回只发出1次原commands POST；独立exact GET核对原请求v2／returned，原请求通知仍unread。刷新后准确终态保留，无后续决定入口。
- 英文准确请求及returned状态呈现；390px下交接DOM几何未越出视口，语言和窄屏读取不产生业务写；未观察到renderer错误。

瞬态成功文案未跨父刷新观察到，不把该文案列为通过；真实来源和独立准确读取结果一致。第一次脚本误读通知上不存在的state字段，已纠正为read／archived；前后两次均为读取原数据，没有重复提交决定。

隐藏窗口截图两次timeout，没有可接纳的视觉图片，因此状态保留visual_image_pending。390px的DOM几何检查不能代替截图布局、主题或视觉图片验收。此前Windows目标URL审核未获确认后已停止该次尝试；本次独立自有Electron／本机证据与该次尝试分开。

本阶段没有模型、工具执行、外部供应方或正式Review，不构成商业模型、物理多机、其他数据库、安装、规模或生产验收。Git提交／推送由主集成按最终实际结果另记，完整58 limited／34 pending／92总项不变。缓存、数据库、原始日志、凭据、二进制和截图不纳入Git交付。

提交前统一CRLF为LF，仅规范文本换行，冻结构建源码与产物摘要仍保留原值；准确映射见本机staged-manifest。该格式调整无语义变化，不重复测试或构建，提交前另核对暂存内容与空白检查。
