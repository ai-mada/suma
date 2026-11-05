# Windows 本机工作区打开 Cursor 验证

T-CURSOR-EDITOR-234，速码 1.0.0，2026-11-05。 [桌面合同](../../contracts/desktop-workspace.md#windows-原工作区在-cursor-中打开) · [证据摘要](cursor-editor-launch-summary.json)

在 Windows 桌面的项目本机工作区，当前已绑定目录旁提供“在 Cursor 中打开”。主进程核对当前身份、项目管理权限与原目录绑定，定位本机原生 Cursor 后再读取同一授权上下文。原目录、身份、提交和绑定继续匹配时，请求在独立编辑器窗口打开。页面只接收启动请求结果。

| 验证 | 结果 |
| --- | --- |
| 真实 Windows 安装与窗口 | 原生选择器找到当前登记的 Cursor 3.27.22，实际 `--version` 返回同一版本。生产参数核验为 `--classic --new-window` 加已授权目录。随后真实程序在独立桌面与临时配置中显示 `verified-cursor-workspace - Cursor`。 |
| 桌面逻辑 | 5 项原生启动检查与 51 项托管控制器检查通过，含拒绝跨窗口、附加路径、旧绑定、查找期间换绑、链接目录和程序更换。 |
| Web 交互 | 3 项定向检查通过。Chrome 390×844 使用隔离原生接口夹具核对单次键盘触发、原绑定摘要、等待及中英文结果，无页面横向溢出与浏览器错误。 |
| 类型与双语 | IDE 和本阶段组件的定向 TypeScript 检查通过，中英文 10,184 对词条通过。工作区全量类型检查另外报告并行开发文件错误。 |

隔离窗口夹具前两次遇到 GPU 子进程和渲染进程启动故障。最终运行使用测试专用配置与扩展目录，并增加仅用于隐藏桌面的渲染和沙箱参数、回环代理。生产入口的参数另行按精确列表检查。真实编辑器验证使用一次性测试目录，结束后释放夹具进程树。正式桌面包的默认配置启动、其他操作系统以及 Cursor Agent CLI、账号和模型执行仍待验证。

参考 Multica 本地固定提交的窄桌面桥与主进程路径解析，沿用速码既有桌面权限与绑定合同。Cursor 的编辑器启动和 Agent 使用分别查证。 [Multica 主进程](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/apps/desktop/src/main/daemon-manager.ts#L1575-L1592) · [Cursor 编辑器启动说明](https://forum.cursor.com/t/cli-launch-always-opens-agent-panel-instead-of-ide/166537/7) · [Cursor 项目使用文档](https://cursor.com/help/getting-started/first-project)
