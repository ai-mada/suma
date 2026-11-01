# 桌面端安装与打包

状态：**1.0.0／T-DESKTOP-108，实施中**，更新于2026-10-28。[English](../en/desktop-installation.md) · [启动与目录授权](desktop.md) · [完整交付状态](../delivery/status.md)。

本页提供Windows、macOS、Linux各自x64／arm64的分发选择和统一打包入口。目标配置、打包成功、真实安装、签名／公证和生产验收分别记录。**当前真实Electron启动仍被GPU子进程`0xC0000135`阻断，具体缺失依赖尚未定位；本页不表示六目标已完成安装或运行验收，也不表示已发布可下载的安装包。**完整P0–P7及需求1–25继续实施，桌面交付不缩减完整本地开发工作台范围。

## 选择系统、架构与形式

先确认操作系统与CPU架构；x64对应Intel／AMD 64位，arm64对应ARM 64位。Windows在“设置→系统→系统信息”查看系统类型；macOS在“关于本机”查看Intel处理器或Apple芯片；Linux运行`uname -m`，`x86_64`选x64，`aarch64`选arm64。优先选择原生架构，不把仿真运行视为该架构验收。

| 系统／架构 | `--platform`／`--arch` | `installer` | `portable` |
| --- | --- | --- | --- |
| Windows x64 | `win32 x64` | NSIS `.exe` | 便携 `.exe` |
| Windows ARM64 | `win32 arm64` | NSIS `.exe` | 便携 `.exe` |
| macOS Intel | `darwin x64` | `.dmg` | `.zip`内含应用 |
| macOS Apple芯片 | `darwin arm64` | `.dmg` | `.zip`内含应用 |
| Linux x64 | `linux x64` | `.AppImage`和`.deb` | `.tar.gz` |
| Linux ARM64 | `linux arm64` | `.AppImage`和`.deb` | `.tar.gz` |

`--format all`生成该系统的installer和portable目标；`dir`只生成未安装应用目录，供本机检查和受控分发。Linux用户在AppImage、Debian系包和压缩包中选一种即可。归档或目录必须完整保留，不能只复制入口程序。便携形式省去安装步骤，**仍使用当前账号的应用数据目录**，不是随U盘移动数据库或授权。

## 获取、校验与安装

从可信维护者或企业分发渠道获取匹配的包、`manifest.json`与`SHA256SUMS`，先对照版本、系统、架构和实际文件名。本阶段没有自动发布或更新服务。SHA256只能验证与可信清单相同的字节，不能代替发布者身份或系统签名。

Windows示例，先进入下载目录，再核对结果与可信`SHA256SUMS`中同名文件的摘要：

```powershell
$artifact = '.\Suma-1.0.0-win32-x64-installer.exe'
Get-FileHash -Algorithm SHA256 -LiteralPath $artifact
Get-AuthenticodeSignature -LiteralPath $artifact | Format-List Status,StatusMessage,SignerCertificate
```

macOS和Linux在包所在目录计算单个文件摘要，同样与可信清单中的同名项比对：

```sh
# macOS
shasum -a 256 ./Suma-1.0.0-darwin-arm64-installer.dmg
# Linux
sha256sum ./Suma-1.0.0-linux-x64-installer.AppImage
```

只有拿到清单列出的完整文件集时，才用macOS的`shasum -a 256 -c SHA256SUMS`或Linux的`sha256sum -c SHA256SUMS`批量核验；只分发单个安装器时，其余条目缺失不能解释为该文件校验通过。

| 形式 | 安装与启动 | 卸载程序 |
| --- | --- | --- |
| Windows NSIS | 以日常用户身份打开安装器，按向导选择目录；完成后从开始菜单启动Suma | 使用系统“已安装的应用”中的Suma卸载入口 |
| Windows便携EXE | 将已校验文件放在当前用户有写入权限的普通目录，双击启动 | 退出Suma后删除该便携程序 |
| macOS DMG | 打开DMG，将完整`Suma.app`复制至`~/Applications`或获准的应用目录，推出磁盘映像，再从Finder启动 | 退出后将安装位置的`Suma.app`移至废纸篓 |
| macOS ZIP | 解压，保留完整`Suma.app`，移至所选应用目录并从Finder启动 | 同上 |
| Linux AppImage | 给已校验文件增加可执行权限，再以普通用户运行；参见[AppImage官方运行说明](https://docs.appimage.org/user-guide/run-appimages.html) | 退出后删除AppImage文件 |
| Linux deb | 在Debian／Ubuntu系系统用包管理器安装，依赖应来自受信任软件源；从应用菜单启动 | 使用包管理器移除Suma包 |
| Linux tar.gz／未安装目录 | 在用户应用目录解压，保留所有资源和权限，从目录内`suma`入口启动 | 退出后移除该应用目录 |

Windows NSIS配置为默认当前用户安装、允许选择安装目录、不主动请求提权、安装完成后不自动运行；安装器配置中文／英文语言选择。向导若选择所有用户安装，需预先具备组织批准的权限，数据仍按每个运行账号隔离。这些是配置事实，实际中英文安装与卸载仍待验收；选项含义见[NSIS官方配置](https://www.electron.build/v26/docs/api/app-builder-lib.interface.nsisoptions/)。

Linux命令示例，文件名以收到的清单为准；deb安装需管理员批准，运行Suma使用普通用户：

```sh
chmod u+x ./Suma-1.0.0-linux-x64-installer.AppImage
./Suma-1.0.0-linux-x64-installer.AppImage
# Debian／Ubuntu，先选择与系统匹配的deb
sudo apt install ./Suma-1.0.0-linux-x64-installer.deb
```

如SmartScreen、Gatekeeper或企业策略阻止启动，停止安装并核对来源、签名、公证和组织批准的版本。不要关闭系统防护、删除隔离属性或传入`--no-sandbox`／`--disable-web-security`。当前签名、公证、发行版依赖及最低系统版本仍需按目标机验收；“能生成包”不表示可绕过这些要求。[electron-builder签名说明](https://www.electron.build/v26/docs/features/code-signing/)区分Windows签名与macOS签名／公证。

## 首次启动与使用

Suma启动内置Go控制服务，只监听动态分配的`127.0.0.1`端口；健康检查就绪后打开共享工作台，无需手动寻找固定端口。首次使用按界面创建账号、组织和项目，已有数据则登录。关闭Suma会终止其子服务；再次打开使用同一用户数据。当前不会自动启动Runner、安装外部工具或自动恢复已退出的控制服务。

在当前项目的开发工作空间点击“选择项目文件夹”，确认项目和路径后再点“绑定目录”。选择本身不会绑定；取消不改变项目。文件、目录、Git和保存继续校验服务器权限及项目根证明。有草稿或待核对保存时先完成处理，再更换目录。原生GUI完整旅程尚待实际验收，协议及当前证据见[桌面说明](desktop.md)。

工作台支持中文／英文偏好。启动诊断优先读取本进程`SUMA_LANGUAGE=zh-CN`或`en-US`，其次系统语言，默认中文；它不改变业务内容或执行日志。原生目录框使用工作台当前语言。操作系统、未知错误、第三方控件及安装器的完整中英文体验须单独验收。

## 数据、备份与卸载边界

打包应用采用Electron的`userData`目录；本机Windows x64包实际使用下列位置，系统重定向时以实际路径为准。macOS和Linux位置仍需在对应安装包实机确认。[Electron路径说明](https://www.electronjs.org/docs/latest/api/app#appgetpathname)定义了各平台默认位置。

| 系统 | 当前账号应用数据位置 |
| --- | --- |
| Windows | `%APPDATA%\@suma\desktop`（本机Windows x64包实测） |
| macOS／Linux | 以实际Electron `userData`目录为准，待目标系统实测 |

- `data/suma.db`及同目录业务文件保存本机状态；`data/automation-ingress.key`在自动密钥模式下保护已保存的入口秘密。
- `desktop-workspace.key`是32字节本机目录授权密钥；数据库中的项目根证明还包含目录的操作系统身份。
- 绑定项目的源文件仍在所选原目录，不会复制进应用数据；模型密钥、外部CLI配置和项目仓库需按各自存储规则保护。

备份前正常退出Suma并确认子服务已停止，安全备份完整`userData`及项目原件；不要运行中单独复制SQLite文件。密钥与数据库必须配套保留并限制访问，不能写进Git、工单、日志或公开清单。恢复后核对账号、项目、权限、原件与摘要；复制到其他机器或更换目录后，操作系统目录身份可能不同，需要明确重新选择绑定，不可靠路径字符串相同恢复授权。当前没有通用数据导入、库升级或密钥轮换迁移向导。

卸载应用程序与清除业务数据是两个动作。NSIS配置`deleteAppDataOnUninstall:false`；便携／归档／目录文件的移除不会清除`userData`。不要把删除数据库或授权密钥当作修复步骤。需要彻底清除数据时，先完成可验证的备份，再单独确认准确目录；不要删除绑定的项目原件。目标系统卸载保留行为仍需安装验收。

## 从源码统一打包

在仓库根目录操作，需要Node.js 22+、仓库固定的pnpm及Go工具链；当前`go.mod`为Go1.26.0／toolchain1.26.10。依赖按锁文件准备，Electron SDK必须匹配目标系统与架构。终端用户安装完整包不需要这些构建工具。

先查看计划，不编译或打包；六个目标均可在同一宿主检查计划：

```sh
node apps/desktop/scripts/package.cjs --platform win32 --arch x64 --format all --plan
node apps/desktop/scripts/package.cjs --platform win32 --arch arm64 --format all --plan
node apps/desktop/scripts/package.cjs --platform darwin --arch x64 --format all --plan
node apps/desktop/scripts/package.cjs --platform darwin --arch arm64 --format all --plan
node apps/desktop/scripts/package.cjs --platform linux --arch x64 --format all --plan
node apps/desktop/scripts/package.cjs --platform linux --arch arm64 --format all --plan
```

确认计划允许构建后，在目标系统宿主运行对应命令。例如Windows x64：

```sh
node apps/desktop/scripts/package.cjs --platform win32 --arch x64 --format all --signing off
```

省略平台／架构时使用宿主值，默认形式为`installer`，本阶段仅支持默认签名策略`off`。脚本显式设置Go目标系统和架构，在独立暂存目录构建控制程序和网页，不覆盖活动预览的`apps/web/dist`。输出为`dist/desktop/<platform>-<arch>`；过程文件、缓存与临时文件位于仓库`.cache/desktop/<platform>-<arch>`，环境覆盖只作用于本次进程。

| 可选参数 | 用途与限制 |
| --- | --- |
| `--electron-dist ABS` | 使用已准备的目标Electron发行目录，版本、文件格式和架构必须匹配；默认获取正确目标SDK，不复用其他架构宿主SDK |
| `--out-dir ABS` | 使用新的绝对输出子目录，必须位于本仓库`dist/desktop`或`.cache`之下，且不能与本目标暂存目录重叠 |
| `--language zh-CN`／`en-US` | 选择打包诊断语言，不代表安装器语言已验收 |
| `--maintainer "姓名 <邮箱>"` | 设置Debian包公开维护者，覆盖环境变量`SUMA_DESKTOP_MAINTAINER`及仓库默认值 |

输出目录已有文件时构建会拒绝覆盖；复验请指定新的输出子目录并保留既有产物与证据，不为重新打包删除用户文件。

Linux的`installer`／`all`包含deb，必须有有效维护者。当前仓库根`package.json`的`config.desktopMaintainer`已采用获准公开的`zhaoyingchao <zyc2012518@126.com>`；优先级为命令行`--maintainer`、本进程环境`SUMA_DESKTOP_MAINTAINER`、根配置默认值。使用当前默认值无需另外设置环境变量；企业重分发可显式覆盖真实维护者，例如：

```sh
node apps/desktop/scripts/package.cjs --platform linux --arch x64 --format installer --maintainer "zhaoyingchao <zyc2012518@126.com>" --plan
```

Linux的portable／dir不要求维护者；缺少deb维护者会使计划不允许构建，格式无效则拒绝参数。维护者是包内公开元数据，不能填写秘密、虚构联系人或自动取用私人Git身份。

跨系统installer／portable执行会被拒绝；macOS的`dir`同样要求macOS宿主，其余跨系统`dir`仍需匹配的Electron SDK及输入校验。计划中的`build_allowed`只表示系统／策略允许继续，不能证明资源校验、打包或安装成功。推荐使用各目标系统／架构的原生构建与实机验收，macOS签名必须在macOS进行；参见[electron-builder多平台构建限制](https://www.electron.build/v26/docs/features/multi-platform-build/)。

当前Windows、macOS和Linux交付均仅支持`--signing off`。`auto`和`required`会在准备资源、调用SDK或使用证书前明确拒绝，不是可用的签名交付路径。生产代码签名、签名产物核验和macOS公证仍待实现与验收；当前清单记录未签名状态，安装、界面、签名和公证验证均不标为通过。不得通过关闭系统防护来替代正式签名交付。

成功完成时写入`manifest.json`和`SHA256SUMS`，以本次实际输出记录文件与摘要。文件名为`Suma-1.0.0-<platform>-<arch>-installer.<扩展名>`或`-portable.<扩展名>`，未安装目录为`Suma-1.0.0-<platform>-<arch>-dir/`。清单记录实际交付文件；校验清单包含普通文件和`manifest.json`，目录内符号链接另见清单条目，分发时须保持完整结构和权限。只共享对应的当前产物，不把计划或过往文件当作已生成结果。分发前仍需在目标机核验安装、首启、原生选择／取消／绑定、中文／英文、重启、退出和卸载保留数据。

## 离线与企业部署

完整桌面包包含Electron、Go控制程序和网页，安装后本地工作台不要求Node.js／Go或持续连接下载服务；外部模型、工具、知识服务和远端协作按配置另需网络及授权。单机桌面以当前操作系统账号、SQLite和本机目录授权为边界；它不自动建立企业共享服务、跨设备同步或SaaS租户运营。企业共享部署见[服务器部署](server.md)及[容器部署](containers.md)，显式远程连接管理仍待实现。

企业可在受控构建机生成并校验包，连同清单转交隔离网络。需要复用已构建资源时，可传入绝对路径；下面的路径须替换为实际受控目录：

```sh
node apps/desktop/scripts/package.cjs --platform linux --arch arm64 --format portable --web-dir /approved/suma/web --control-binary /approved/suma/suma-control --control-manifest /approved/suma/suma-control.manifest.json --plan
```

去掉`--plan`才执行。网页输入会经过文件树检查并复制到暂存目录；预编译控制程序必须有默认侧车`<binary>.manifest.json`或显式`--control-manifest`，匹配当前源码摘要、1.0.0版本、目标和二进制摘要，并通过实际文件头架构检查。正常源码准备会生成侧车及`prepared.json`，例如`node apps/desktop/scripts/prepare.cjs --platform win32 --arch x64`；产物位于该目标的`.cache/desktop`暂存目录。复用时将完整资源和侧车放到暂存目录之外的受控输入位置。没有为未知程序补造来源清单的命令，不要手写或修改清单来绕过不匹配。

预编译输入避免重新编译网页和控制程序，不保证打包器全程断网；当前没有`--offline`开关。还需预备锁定依赖、目标Electron SDK及所选打包目标所需的工具缓存，离线构建需另行验证。

## 故障处理

| 现象 | 处理 |
| --- | --- |
| 计划显示不允许构建／架构不符 | 核对平台、CPU、Electron SDK和预编译清单；换到适合的原生宿主，不修改摘要或强行套用其他系统的包 |
| 缺控制程序／网页、服务未就绪 | 保留启动诊断，重新核对完整包与摘要；开发验证按[桌面说明](desktop.md)使用独立`.cache`资源 |
| 密钥或数据目录不可用 | 核对权限、普通文件和真实路径；保留原密钥与数据，不删除重置 |
| 目录授权失效／绑定冲突 | 明确刷新当前绑定后重新选择；结果不明时先“核对原绑定”，不要改成新操作 |
| Linux AppImage报告FUSE／系统库缺失 | 查阅发行版与AppImage官方要求，使用组织批准的依赖或匹配deb／tar.gz；不以关闭Electron沙箱替代依赖修复 |
| GPU子进程`0xC0000135` | 当前已知真实启动阻断；保留原错误和环境信息，继续定位依赖并重跑原生旅程；Node／网页检查通过不证明已修复 |

提交问题时附版本、包名、摘要、系统／架构、安装形式和脱敏错误。不要上传数据库、工作区密钥、令牌、完整私有路径或项目源文件。[当前交付状态](../delivery/status.md)是安装、签名及完整平台剩余工作的统一入口。

源码观察器使用固定目标Node与许可资源；原生目标默认当前Node，跨目标需管理员通过`--node-binary`、`--node-license`提供对应绝对文件路径。参见[当前源码观察资源合同](application-source-inspector.md#桌面与容器资源链)。
