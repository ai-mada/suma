# Cursor 连接核验：限定验证

T-CURSOR-CONNECTION-166，速码1.0.0，2026-10-31。负责人：主集成代理。状态：被动发现与连接指引限定核验完成；真实执行适配仍planned。范围以[合同](../../contracts/cursor-connection.md)、[架构](../architecture/cursor-connection.md)和[交互规范](../design/cursor-connection.md)为准；机器摘要见[阶段摘要](cursor-connection-summary.json)。

## 已核验边界

| 范围 | 实际结果 |
| --- | --- |
| Go发现器 | 首批15个顶层／27子场景通过（4新／11原）；末审发现Windows UNC／设备路径访问边界，补1个精确回归，8类拒绝路径的Lstat调用为0，普通盘符对照通过。只重跑新增1项，未重跑旧15或实际命令。 |
| 能力目录 | 2项不同检查通过（1新／1原）。首次新断言误要求planned一律unsupported，修正为不得supported后仅重跑该新测试；没有升级执行资格。 |
| 桌面IPC | 13项通过（2新／11原）：闭合事实、当前授权、固定helper及原停止边界；不向renderer传执行命令。 |
| Web | 34项不同检查通过（20新／14原）：闭合reader、5类事实、scope/API/bridge变化、重复点击、原登记／启用／回执。初次测试夹具及locale命名空间收集失败保留；末次文案呈现不重复34项，由最终类型／语言／浏览器覆盖。 |
| 静态集成 | 一次TSC退出0；7853对中英词条及占位／复数／markup检查通过，生产硬编码0。公开HTTP／Schema没有新增，无需重复生成合同。 |

65个不同自动化顶层检查不含3个组件浏览器场景；子场景不重复计入顶层数。Go首批15项及实际命令对应源摘要`3d0485aa6e45365fed82ff47086fb6d0da5e3bc7375575e4601cb715d989785d`；后补路径回归对应当前摘要`22799688f1a8fe663541f6b903a82340f51c1ecb9c14af627bef5ad298df90e0`，不将早期实际结果归于后改源码。

## 实际命令与界面

仅一次`go run ./cmd/suma-runner --language=en-US --discover-installed-tools --local-tool cursor-cli`退出0，真实返回`cursor_scan_incomplete`、installed=false、launch_supported=false，无命令／版本／路径。没有全工具探测或Cursor程序执行。cmd/main的后续帮助文案单独记录，不冒充已保存的实际编译输入。

最终18个输入摘要门禁后，唯一Vite SDK单组件构建退出0，未构建或覆盖活动完整App。首次verify因已安装Playwright模块读取EPERM而退出1，0场景／0图／0请求，Electron尚未启动；静态服务正常退出。先归档10个原始收据与日志，再经本机权限许可复用原out续验，没有重复构建。

续验verify／Node／外层均退出0，3个真实组件场景通过：中文1366下选择Cursor并显示受控editor-only结果；切项目使旧pending响应失效、新范围必须另点检查；英文390无桌面桥仅显示准确指南。非GET、外连、页面错误、登记／启用等写callback均0。它们使用真实组件与受控catalog／内存bridge，**不是真实IPC、账号、服务、厂商或完整App验收**。

主集成者独立查看两张最终原图，文案可读、窄屏按钮完整；不外推全设备／主题矩阵。page、Electron与静态服务正常关闭，before/will-quit及退出0留证，6个记录PID均不存在，44009无监听，无强停。14项安全文件及清单已逐项核对并导出本机`dist/quality/cursor-connection-166/`；清单摘要`9c48c33229758ff11d350655c7346124de4200d9d30b53f175b011633a2e7936`。缓存、图片、包、运行库、原始日志及临时文件均不进入Git。

提交检查发现两份新增语言JSON的CRLF被仓库识别为行末空白，提交前仅规范为LF，逐项解析后的JSON内容相等。验证时18输入、author摘要和图片原样保留，最后2个文件的新旧摘要单独记录，不重建或重跑语义未变的65项／3场景。一次文档检查通过484篇Markdown、375条API与3464引用；后补仅上述格式收据／计数文字，没有新增链接或Schema。

## 参考材料与未验范围

当前Multica参考固定于`e8ae72c9dfc1b4474b61a9087f51b33cbee6a048`，没有复制本阶段产品源码。Windows官方固定包的静态目录及5个顶层文件表明入口为cmd→ps1→打包Node/index；未运行包、版本、help、登录或模型，未安装用户环境。静态包摘要与协议实测、账户授权、文件隔离各自分开。

本阶段不开放Cursor执行Profile、模型选择／预算执行、写工作区或候选生成。其完整执行适配仍是必须交付的planned项；后续复用原调度、租约、工作树、候选、独立测试与人工验收，不引入第二执行权威。Windows路径边界不外推POSIX双斜线、映射／挂载盘的远程访问保证。目录卡片直达指引可在后续交互批次补齐，当前入口为工具→快捷接入→Cursor。其他操作系统、真实厂商、多机、安装包、规模及生产均未由本次限定验证证明。完整31项／P0–P7／58限定＋34待交付＝92项保持不变。
