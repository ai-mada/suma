# 当前人员任务与统一命令入口合同

状态：T-MULTICA-136；初始1.0.0；负责人：项目协作／工作台。关联原P3个人工作、全平台易用性及[主设计](../docs/suma-architecture-design.html#s21)。完整平台58限定实现／34待交付／92总项不变。

## 当前人员任务

GET `/my-work?project_id=…&q=…&status=…&limit=…&cursor=…`仅接受这些单值参数。`limit`默认20、最大50；`q`最多200个Unicode码点，按标题／说明进行字面匹配，`%`、`_`、`!`不解释成查询通配。大小写按数据库自身LOWER规则，SQLite不据此宣称完整Unicode大小写折叠。

`status`默认all，可选draft、todo、in_progress、in_review、accepted、blocked、cancelled；排除archived。返回的状态汇总和All列表保留当前其他正式业务状态，客户端不能因为出现自定义状态而丢弃整页。业务状态用已有标签或原文呈现，不能由状态推断测试通过、执行成功或正式验收。

服务器从当前凭证解析账号，再通过`suma_human_resources`规范绑定取得当前项目人员资源；任务`owner_id`等于该资源ID，不能与账号ID直接比较。资源须是当前active的human、其来源账号与当前账号相符。未绑定返回unbound，资源失效返回inactive；两者精确返回空任务和空汇总，不自动创建人员。拒绝调用者指定user_id／owner_id。浏览器和当前账号绑定的项目API可读，Runner／Agent凭证不可代替人员身份。

`MyWorkPage`返回`project_id,user_id,resource_id,resource_name,resource_status,items,total,limit,next_cursor,status_counts,as_of`。items是既有Record；total覆盖当前人员、文字及所选状态的完整集合，不是已加载数量。status_counts应用相同人员和文字条件，覆盖全部未归档状态，先于所选状态过滤。as_of是当前授权检查时间，非历史快照。

有界SQL按updated_at倒序、id正序读取，追加人员／更新时间普通索引。游标为规范不透明编码，绑定租户、项目、当前账号、人员资源ID及版本、完整文字／状态／页大小。当前授权每页重新检查；改派、状态或更新时间改变属于实时集合变化，使用刷新核对，不能把多页声称为不可变快照。没有新业务表、第二调度器或旧数据导入。

开发→我负责的任务支持URL恢复文字与状态、精确数量、明确加载更多、任务详情及其开发工作区入口。详情、讨论、派发、模型预算、独立测试和验收继续使用原有领域。读取失权或切换身份／项目立即清除旧结果；旧请求不能恢复旧私有事实。暂未建立本人资源时给出明确说明和资源管理入口，不自动扩大权限。

## 统一命令面板

Ctrl/Cmd+K打开现有Radix弹窗，默认提供当前上下文页面与管理目录；输入文字后250ms查询当前项目原`/search`。页面入口与授权业务结果分组，支持方向键、Home／End、Enter、Escape、原控件焦点恢复及中英文。Windows／Linux和macOS显示各自快捷键。

静态页来自明确目录，管理项复用现有managementAreas；服务端结果通过sourceDestination校验当前项目、对象种类和标识，只能进入受控业务路由，不执行任意URL。输入变更立即隐藏旧查询结果；取消、代次、原查询和当前身份共同隔离迟到响应。错误与无匹配分别显示，显式重新读取保留当前查询。完整页／对象导航都经过既有未保存和结果未知保护，拒绝离开时保留弹窗。

最近入口仅在内存保存当前账号／组织／项目下的静态页面ID，最多5项；不保存对象标题、片段、日志、秘密或跨身份历史。关闭清除本次查询，重开可见安全页面入口。打开／关闭弹窗不更改底层任务或批准任何操作。

## 当前范围

正式个人审核分配、用户通知已读／归档、待接收交接箱及看板服务端分组分页继续独立交付。当前交接接收者在版本化payload中；本阶段不扫描所有payload来伪造收件箱。服务端个人集合不能由前端过滤有限项目页替代。许可与参考源码采用见[复用记录](../docs/integrations/multica-adoption.md)，验证边界见[报告](../docs/quality/multica-collaboration-verification.md)。
