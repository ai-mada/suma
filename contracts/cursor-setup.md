# Cursor 执行节点配置与服务生命周期

T-CURSOR-SETUP-191；速码1.0.0。完整31／P0–P7／58限定／34待交付／92总账继续。接续189／190已实现的包、ACP、原输入与质量链，补齐高级／浏览器／独立节点配置及已完成RPC的宿主停止语义。

## 配置流程

界面只收目标Shell、官方包根目录、完整核对版本、准确模型ID、凭据环境变量名和全新commands文件路径。Node与index.js由固定包结构确定，不填argv、摘要或密钥值。生成可复制命令：

`suma-runner --prepare-tools <全新绝对路径.json> --prepare-acp-stdin`

私有stdin严格五字段：adapter_id=cursor-cli、executable=包内固定Node绝对路径、reviewed_version、model、credential_env_ref。Go复用BuildCursorACPCommand计算完整包闭包与固定配置；先拒绝危险／已有目标再读输入或构建，DecodeCommands后O_EXCL／Sync写新文件。stdout复用原ToolPreparation，只证明文件准备，不证明版本、凭据、模型或健康；此步不读取token／凭据、不启动CLI或模型、不连接／Claim／启用。共同语言环境读取不属于凭据读取。

PowerShell／Bash采用真实逐字参数引用与UTF8输入，不能把JSON.stringify当Shell转义或修改全局环境。界面明确将非权威shell／文件路径草稿交给原执行节点指南；不重复身份，Cursor不能再走缺参数的generic prepare。随后明确运行原describe／report，沿当前授权、fresh库存、暂停登记和独立CAS启用；现有配置文件和未知回执不覆盖。

## RPC与进程事实

固定Cursor ACP是服务；Prompt返回终态不承诺宿主自行退出。标准session/cancel为通知，只有真实写入事实，没有独立取消ACK。Discovery字段明确使用session_cancel_requested／session_cancel_written，不保留虚构close／ack别名。

ACP执行与目录证据新增host_process_exit_code（真实OS退出值）及host_stopped_after_rpc（已完成准确RPC后受控停止宿主）。此停止仅允许Cursor，在原SDK成功结果、无pending／截断／错误／未知写入、原ctx未取消或到期、原配置／包未变且全树停止得到证明时成立。执行还需原end_turn、工具完成及所有原输入闭合，发现还需完整合法目录、无Prompt／配置写入／意外活动并实际写出cancel通知。

业务操作成功不等于宿主退出0；真实host exit必须原值保留。取消、超时、部分写、未答复、错误终态、未证实全树停止及Unix当前Stop错误仍保持unknown，不重发Prompt或答复。质量、候选、同提交独测与验收不因宿主主动停止获得放宽。

## 分工与验证

Root拥有共享证据／验证／Schema、文档和Git。Runtime拥有CLI及配置准备／原writer；Backend拥有现ACP执行／发现／停止门及克隆；Web拥有唯一Cursor指南、原路由／节点草稿衔接及证据消费者／双语。无新表、派发器或HTTP接口。

复用成熟Go文件／SDK／进程控制与原权限／回执；只新增及直接影响检查。真实包可用于一次只读构建配置，无厂商账号或Prompt。受控长驻宿主证明停止门，不能冒充真实认证后厂商执行；Windows已确认树停止与Unix未覆盖分别披露。现有SDK、190交互／完整链及原版本探测不重复。
