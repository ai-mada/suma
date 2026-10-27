# 执行工作区只读链路：限定验证报告

状态：1.0.0／T-P4-037，2026-10-27，本地界面与最终联合复核已通过，范围见[机器摘要](workspace-observation-summary.json)。负责人：平台集成。关联[产品与交互](../product/remote-workspace.md)、[合同](../../contracts/workspace-observation.md)、[架构](../architecture/remote-workspace-broker.md)、[AI SOP](../operations/remote-workspace.md)、[英文](../en/remote-workspace.md)。完整P0–P7、需求1–22和生产验收仍在进行。

## 实际验证范围

| 验证 | 结果与可证明范围 |
|---|---|
| 控制面／HTTP | 原调用凭据、当前项目／attempt／fence／worker、私有与Pool当前双边grant、失效正文清除、跨项目和重复查询／字段拒绝；原DAG状态版本未被读取请求改变 |
| Runner | 只保留当前活跃os.Root句柄；真实原生list/read路径复用；越界、大小写、敏感正文、原结果ACK丢失重放、Pool完整派发及旧租约拒绝 |
| 资源修正 | 空轮询原先使operations9→10，新增专项先红后绿；重复空轮询不增长，后来同key可领取新观察；非空lease准确重放仍只有一份receipt |
| CLI／合同 | targets、准确scope读取请求与稳定key、错误JSON不发请求；17个新增反射Schema与7条新路径，已有路径／Schema结构未变。机器路线明确独立Runner或Pool身份、不接受项目query／浏览器身份头 |
| UI定向检查 | 初次7项通过；最终修正日期时间后仅复核受影响5项通过。覆盖只读代码、中英切换、身份切换迟到结果隔离、本地编辑草稿／保存不回归 |
| 静态检查 | TypeScript与4497配对词条通过。受影响Go vet与合同生成最终复核记录在摘要；复用已通过而未改变的检查 |

原生联合验证使用真实Go Worker、原生CLI协议进程与当前工作树，控制面没有本地文件绑定。文件正文完整摘要为 `73cfeba9dce884604ca986ae5856c7df109997707f9299273de1e81e10274f3c`。不调用模型、不执行正式功能测试或人工验收，不改变源仓库HEAD。

## 实际界面旅程

环境：隔离 `http://workspace037.localhost:59217`，Windows／IAB桌面1265×712，使用CUA；不使用外部浏览器或另装工具。登录自有测试账号→开发协作→工作空间→执行工作区→准确native attempt→真实文件列表→source.txt只读正文→展开摘要。切换中英保留当前事实，左右方向键选择来源并正确移动焦点，刷新恢复来源／当前仍有效目标并重新列文件，链接定位到准确运行／节点／attempt。

页面身份、有效主内容、无框架错误覆盖、目录与代码真实返回、摘要一致、刷新、导航和console检查通过；观察到相关warn／error为0。实际检查发现读取时间只有日期，已改为本地化日期与时分秒；第二次构建仅用于此修正，同一GUI宿主刷新完成复核。

最终截图与摘要位于忽略目录 `.cache/e2e/workspace-observation-stage-037/browser/`；精确路径和SHA见机器摘要。截图不提交Git。最早一次hold因web尚未就绪正常关闭，没有GUI声明；随后唯一最终GUI宿主取得上面的界面证据。

## 长等待与最终源码边界

最终GUI宿主使用空轮询优化前的Go二进制，其真实list/read／授权逻辑没有改变。后来的空轮询优化由定向红绿测试覆盖，最终Go源码再执行一次默认短联合流程，不重开GUI或重复模型链路。长GUI等待超过60秒后，最早读取正确为expired且无正文；旧测试断言原先只接受cancelled，因此保留旧失败原日志并将断言纠正为两种合法清除状态。具体最终退出和证据见摘要，不能将陈旧断言失败计作已通过的测试。

## 保留边界

本阶段仅当前原生attempt的只读list/read。未验收跨物理机器／TLS、OceanBase／PostgreSQL并发、容量、完整远程IDE写入、PTY／LSP／DAP、移动视口或真实厂商工具。Monaco内置菜单仍英文，已有大型延迟加载包继续发出体积提示；没有据此宣称低配置或万节点性能通过。

Control拒绝到期披露并清逻辑正文，不承诺WAL／备份／全部磁盘副本安全擦除；Runner私有journal在后续checkpoint裁剪，不等同60秒同步物理删除。主035预览未修改或重启。源码、文档、协议模拟、真实本机进程和生产验收分开记录。

最终Go源码短联合复核通过，5.72秒，回执SHA `54797251ee652effe6583f5fa74642b10baa0ee5c7bf462f1562c6afea053f82`；结果结束后旧正文隐藏、新读取409、源HEAD不变。长GUI宿主394.79秒的陈旧断言失败原日志保留，实际expired／无正文；最终59217监听已关闭，测试页已关闭。
