# 任务批量负责人变更合同

状态：当前1.0.0实施合同，T-MULTICA-BULK-150。依据[Multica复用优先级](../docs/integrations/multica-priority-map.md)与[唯一实施计划](../docs/delivery/implementation-plan.md)。沿现有显式最多16项任务选择、当前项目权限、正式任务更新、通知及原操作回执，不创建第二套任务或调度权威。

## 用户流程与范围

当前人员在任务列表明确选择已加载任务，选择一个当前有效的规范人员资源，读取服务端逐项预览，确认影响后一次提交。原专家团队安排入口保留；本片不包含隐式全选、删除、状态批改、团队或执行者替换、工具启动或正式交付验收。面板采用现有Radix Modal、写操作保护、设计令牌和中英文词条，默认只显示选择数量及入口。

同一负责人的任务明确列为“负责人不变”，不进行空更新、不发送分配通知；如果全部相同，预览不可提交。未变负责人任务仍可能因本次其他前置任务变化受到原有依赖失效规则影响，界面需要明确说明。只修改实际变化的负责人；变更任务按原Update规则转为in_progress并清除verified_run_id，保留运行、代码、测试和Review历史。已验收后续任务可能失效，已完成迭代／里程碑可能重新打开；本片只提供真实通用影响说明，不虚构完整下游计数或固定图快照。

## 当前人员目录

`GET /api/v1/task-owner-targets?project_id=P&q=TEXT&limit=20&cursor=CURSOR`

唯一作用域为当前项目；只接受上述单值查询。q为字面搜索，最多200个Unicode字符；limit默认20、1～100的精确整数。目录采用当前canonicalHumanTargetsWhere(false)：有效human资源、规范user绑定、现存账号和当前组织／项目访问，允许有项目访问的viewer作为负责人，不授予其新权限。当前调用者须为browser／project API人类身份且具备项目写权限。每页按规范名称和resource_id稳定排序，游标绑定原作用域及筛选，拒绝伪造结构／跨范围／重复字段；当前权限每页重查，计数不等于已加载数。无历史快照承诺。

```text
TaskOwnerTarget {resource_id:string, resource_version:int, user_id:string, name:string}
TaskOwnerTargetPage {project_id:string, items:TaskOwnerTarget[], total:int, limit:int, next_cursor:string}
TaskOwnerTargetQuery {q?:string, limit?:int, cursor?:string}
```

## 预览和采用

`POST /api/v1/task-owner-changes/preview`为当前授权的只读业务预览；不记录回执、活动、通知或任务变更，也不调用工具或供应方。

```text
TaskOwnerSelection {task_id:string, expected_version:int}
TaskOwnerChangePreviewRequest {
  project_id:string, owner_id:string, expected_owner_version:int,
  tasks:TaskOwnerSelection[]
}
TaskOwnerChangePreviewItem {
  task_id:string, version:int, title:string, status:string,
  owner_id:string, owner_name:string, run_id:string, will_change:bool
}
TaskOwnerChangePreview {
  project_id:string, owner:TaskOwnerTarget, tasks:TaskOwnerChangePreviewItem[],
  changed_count:int, unchanged_count:int, preview_sha256:string, can_apply:bool
}
TaskOwnerChangeRequest {
  project_id:string, owner_id:string, expected_owner_version:int,
  tasks:TaskOwnerSelection[], preview_sha256:string,
  acknowledge_invalidation:bool, idempotency_key:string
}
TaskOwnerChangeReceipt {
  task_id:string, previous_version:int, version:int,
  previous_owner_id:string, owner_id:string, status:string, owner_changed:bool
}
TaskOwnerChangeResult {
  project_id:string, operation_id:string, changes:TaskOwnerChangeReceipt[]
}
```

`POST /api/v1/task-owner-changes`只接受TaskOwnerChangeRequest。两类请求都是封闭强类型JSON，不接受null／遗漏数组、额外字段、重复或跨项目任务；tasks长度1～16，保留显式请求顺序。版本须为1～2147483647精确整数，实际需要增加版本的对象达到上限时拒绝。preview_sha256是服务器当前目标和所选任务预览依据的64位十六进制摘要，不是访问凭证；采用前重算并核对。acknowledge_invalidation必须明确为true。原JSON数值lexeme保留至类型化解码，不将小数舍入成合法版本。

## 一次事务与恢复

1. 当前身份／项目写授权及项目协调先于原回执；相同操作标识只恢复原正文的不可变结果。相同标识改正文拒绝，当前撤权不因旧回执获得新权限。
2. 没有原回执时先读取并核对全部所选任务版本、目标规范身份／资源版本及预览摘要，任何冲突整批零写入。无实际负责人变更时拒绝空操作。
3. 单项Update与批量命令共享同事务的字段应用、验证、依据重置、CAS和分配通知实现。不能循环调用会自行commit的Service.Update，不能复制一个逐渐漂移的更新版本。
4. 只更新负责人有变化的任务。所有所选任务均完成原版本核验后再写；通知投影和序号在原事务内，失败使任务、通知、活动和回执全部回滚。
5. 对实际变化根任务只调用一次InvalidateDependentsTx(includeRoots=false)，避免前置失效先增加后续已选任务版本造成伪冲突。之后重读所有所选任务，返回真实最终版本／状态；负责人不变的已选后续任务也可能被原依赖规则改变。
6. 原活动、规划失效和统一回执与业务写同事务提交；网络结果未知只能以原账号、项目、请求正文、版本、摘要和操作标识核对，不自动改版本／换标识再试。

前端scope按用户／组织／项目隔离。恢复入口不能依赖刷新后仍有选择；存在未确认写或本地持久化故障时，即使零选择也能看见原操作。原操作未确认时禁止新写。409保留原选择和目标，重新预览是用户明确的新决定；不自动采用新版本。切换范围或当前401／403／404后清除受保护预览和目标，不接纳迟到响应。成功回执与后续刷新失败分别显示。

## 分工和验收

后端负责records.go同事务实现及task_owner_changes*.go／独立测试；前端负责TaskOwnerChanges入口／Modal、模型、测试与两语言目录；主集成负责App最小接线、主HTTP分发及数值护栏、生成合同、CLI可复用接口、文档和必要验证。其他桌面、知识、WorkBuddy149在途文件保持原样。

验证覆盖当前权限、跨范围、精确版本、目标变更、预览过期、整批回滚、通知故障、原回执、重复／乱序输入、selected前置／后续及owner不变条目、原单项Update语义、失权／迟到响应／刷新后恢复入口；界面选取中英文、390px和键盘代表状态。独立测试、组件、真实SQLite／HTTP／GUI、其他数据库和生产范围分别记录；完整58 limited／34 pending／92总项不因此改变。
