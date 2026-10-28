# 桌面启动、跨平台构建与本地项目目录

状态：**1.0.0／T-DESKTOP-108**，2026-10-28。Electron管理本机Go控制面，复用React、当前权限、DAG与Runner合同。完整P0–P7及需求1–25继续实施。[中英文安装与打包指南](desktop-installation.md) · [English](../en/desktop-installation.md) · [工作台架构](../architecture/local-development-workbench.md) · [目录授权合同](../../contracts/desktop-workspace.md) · [本阶段验证](../quality/desktop-packaging-verification.md)。

## 快速准备与启动

开发电脑需要仓库固定的Node／pnpm、Go工具链和依赖；安装后的客户端自带Electron、控制面与网页，不需要用户另装Go或Node才能打开工作台。外部CLI、Runner及完整开发服务仍按各自能力配置。

```sh
pnpm desktop:plan
pnpm desktop:prepare
pnpm desktop
```

准备命令按明确的目标编译控制面、构建网页并校验资源；开发启动只读取当前宿主的`.cache/desktop/<platform>-<arch>/control`与`web`。不使用共享的上一架构程序，也不覆盖活动浏览器预览的`apps/web/dist`。`--plan`与`--help`仅读取配置，不编译、下载或修改目录。

```sh
node apps/desktop/scripts/package.cjs --platform win32 --arch x64 --format all
node apps/desktop/scripts/package.cjs --platform darwin --arch arm64 --format installer
node apps/desktop/scripts/package.cjs --platform linux --arch arm64 --format all
```

安装／便携包在对应系统构建；六种目标、离线资源输入、当前维护者与错误处理见安装指南。输出默认进入`dist/desktop/<platform>-<arch>`，包括实际产物、`manifest.json`和`SHA256SUMS`。已有非空输出不会覆盖，后续构建可用`--out-dir`选择新的仓库交付子目录。源码准备及SDK临时数据留在目标专属`.cache`，不进入Git。

当前仅**未签名**打包可用；`--signing auto|required`在准备、SDK及证书使用之前拒绝，不能据SDK配置冒充签名完成。正式Windows签名、macOS Developer ID／公证及签名后的内容溯源仍待实现和独立验收。不得通过关闭系统或渲染进程保护绕过此边界。

## 按需原生CI

`.github/workflows/desktop.yaml`只接受人工触发，可选单一目标或六目标、安装／便携／目录／全部形式。一个网页构建供选中的原生系统任务复用；任务校验真实宿主架构，固定依赖，并归档公开交付目录，保留执行权限和应用链接。工作流仅有仓库读取权限，不接收签名证书，不发布Release或执行部署。

当前尚未执行远端六系统任务；工作流须先存在于仓库默认分支，具备权限后才能从Actions手工选择引用运行。目标配置、控制程序交叉编译和本机Windows打包不等于macOS／Linux安装包已实际通过。

## 运行与数据生命周期

主进程持有单实例锁，选择loopback空闲端口并启动专属SQLite控制面，健康就绪后加载窗口。打包后读取`resources/control`与`resources/web`。关闭时请求子服务退出、等待并核对终止；当前不会自动启动Runner或自动恢复服务。

业务数据位于`<Electron userData>/data/suma.db`，目录私钥位于`userData/desktop-workspace.key`；原项目文件留在所选目录。启动拒绝链接或错误类型的密钥／数据位置，清除或覆盖继承的数据库、工作区根与服务地址配置。数据库、私钥、原始制品及项目目录身份须一起考虑备份恢复，不能靠删除密钥／数据修复权限。

开发隔离验证可同时设置`SUMA_DESKTOP_USER_DATA`、`SUMA_DESKTOP_CONTROL_BINARY`、`SUMA_DESKTOP_WEB_DIR`三个绝对路径，均须位于仓库`.cache`且路径链无链接。打包模式忽略这些开发覆盖；不得修改真实桌面数据以强行通过验证。

## 原生选择与项目绑定

登录后在当前项目开发工作空间选择目录，确认后明确绑定；取消不发绑定，未保存或待核对文件阻止换根。主进程核对准确窗口、主frame与本机origin，签发绑定原账号／组织／项目／路径的短期证明；控制面复核当前权限、CAS和原操作。

原ACK是该操作提交时的事实；界面另读当前绑定后才展示目录，读取失败只刷新，不重复POST。账号／项目切换忽略迟到结果。未知请求仅保留于有上限的内存，关闭／刷新前提示，不能承诺进程重启后恢复该内存槽。

读取、目录、Git与保存复用项目根证明及已打开目录句柄；无效证明不降级为宽根。窗口保持sandbox、contextIsolation、webSecurity及禁用nodeIntegration；preload只暴露平台／版本和单用途目录选择。平台与已知诊断支持中英文，代码、用户正文和未知系统错误保留原文。

## 当前验证与待办

Windows x64安装版、便携版及校验清单已在本机由官方SDK生成；六目标Go控制程序的实际格式／架构已核验。未执行安装／卸载或新安装包GUI，未使用证书，未发布下载版本。[准确证据与源码范围](../quality/desktop-packaging-verification.md)分别列出各层。

此前实际Electron开发启动出现GPU子进程`0xC0000135`，具体依赖仍未定位；本阶段没有将打包成功宣称为修复成功。原生目录GUI、正式安装／升级、目标系统矩阵、品牌图标、完整IDE、远程连接与签名交付继续保留待办。
