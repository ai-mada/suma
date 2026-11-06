# 快捷接入与准确工具档案复用

T-TOOL-PROFILE-REUSE-180；suma1.0.0。继续原31项／P0–P7目标，不新增工具执行器或授权来源。

快捷接入在当前项目、节点和完整不可变Profile下查找已有档案，避免断线重连后重新登记。名称、局部配置摘要或列表前200项均不能证明配置相同／不存在。采用当前`toolcontract.EqualProfile`比较全部字段。

## 共享接口

`POST /api/v1/tool-profiles/matches`是只读查询，当前browser-project-read授权先行；不接受Runner／Pool令牌，不写业务、操作回执或审批。请求闭合为`{project_id,worker_id,profile}`，使用完整`ValidateProfile(profile,true)`。项目与身份沿原HTTP和CLI合同核对。无需幂等key，不创建票据或匹配授权。

HTTP200返回闭合领域对象`ToolProfileMatchList {project_id,worker_id,profile,items:ToolProfile[],total}`；items完整、无分页、无截断，总数等于items长度。读取当前tenant/project/准确worker的全部记录，按完整EqualProfile过滤；项目现有1000项上限保持。引用的Profile及Worker必须与请求完全相同；正文及items继续沿原非秘密Profile合同。没有完整可信响应时，界面阻止新建而非默认空集。

paused可显式采用已有档案并进入原enable确认；enabled可显式打开已有档案，保留准确Available/Reason，不自动视为健康；revoked只显示撤销历史，不可复活。多个可用匹配必须明确选择，不能默认第一项。只有没有paused/enabled匹配时才提供明确新建，已有撤销历史仍须用户选择新建。

## 创建、竞争与恢复

不可变配置身份是tenant/project/worker及完整Profile，名称仅展示。普通Create仍创建paused；在原授权事务、原操作回执恢复之后、INSERT之前重查完整EqualProfile。已有paused/enabled时返回409、稳定错误码`tool_profile_match_exists`，不返回另一操作的201或回执；界面重查当前匹配并保留名称草稿。revoked不阻止明确新建。沿原组织／项目授权锁串行化竞争，无需新的JSON唯一索引或表；原当前权限及Worker来源门禁保持。

只有本次操作的稳定key/body才能恢复本次原创建／启用。原pending写入优先，新的只读匹配不能替代、清除或换key重发未知原写入。启用沿原expected_version CAS；状态变更／撤权／节点或配置改变后，重新读取当前事实，不自动重新启用。

## 交互与CLI

选择工具／节点并取得准确新鲜库存后自动查匹配；显示加载、失败重试、空集、单个、多个、撤销历史和版本状态。已有档案入口与新建互斥；启用仍单独确认，不能由匹配自动发生。当前项目、账号、组织、adapter、worker、完整Profile、库存截止时间或API caller变化时，迟到响应不能恢复旧选择。正在查找和原pending写入时阻止父级切换、登记；草稿、断线恢复与180前原连接语义保持。

CLI新增`suma --project PROJECT tools matches --file REQUEST_JSON`（`--file -`读取stdin），严格有界解析同一请求，核对项目，直接调用原client；不添加操作key或confirm，不自动创建／启用。查询超时按只读错误返回，不标为未知写入。帮助与界面完整中英。

### 从当前节点上报复用档案

`tools inventory --worker WORKER` 使用已有登录会话读取指定节点最近一次工具上报。共享节点沿当前项目路由、双边授权及节点凭据核对。返回 `fresh`、`stale` 或 `unreported`，后两种状态的工具明细为空。身份资格、上报新鲜度与单项可用性分别处理。

`tools matches --worker WORKER --command COMMAND` 使用同一读取，选择唯一且可用的准确命令。客户端按响应中的服务端时间和本地经过时长核对剩余有效期。随后使用完整原档案调用已有只读匹配。若上报过期、未上报、找不到指定命令或单项不可用，命令退出并提示重新读取。两种输入方式互斥，配置启用仍沿独立确认。 [终端操作](../docs/operations/task-dispatch-options.md#从节点上报查找已有工具档案)

只验证准确匹配、完整集合、授权、创建竞争／原回执、CLI只读分类及受影响交互；复用未变SDK与原连接／恢复证据。当前实现和实际验收分别记录，不能由查找成功推定真实模型、分布式主机或全平台已验收。
