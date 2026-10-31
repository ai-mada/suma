# OpenCode快捷接入：当前参考与复用边界

T-OPENCODE-QUICK-CONNECT-177；2026-10-31；仅补充参考记录，不替代[本阶段合同](../../contracts/opencode-quick-connect.md)、[原OpenCode执行合同](../../contracts/opencode-sdk.md)或[当前交付状态](../delivery/status.md)。

OpenCode原生包位置按固定提交的[官方构建脚本](https://github.com/anomalyco/opencode/blob/148842ef7cb1bc34192b3fff57d0f9721ab9c801/packages/opencode/script/build.ts)核对，Windows包平台名为`windows`，原生程序在平台包的`bin`目录；[同提交package.json](https://github.com/anomalyco/opencode/blob/148842ef7cb1bc34192b3fff57d0f9721ab9c801/packages/opencode/package.json)记录1.19.4。发现器只解析声明的平台包和真实原生文件，不执行npm／shell包装脚本，不依据网页新版本自动升级本地固定合同。

## 本次参考来源

本地 `D:/codes/github/multica/.git/refs/heads/main` 读取值为 `22a05d832bbe8aee9b0873ff6bb31ae1fe122fc5`。本次只读下列有限文件，没有fetch、切换或改动参考库；文件摘要记录本次所读内容，不声明远端最新或整个参考工作树无修改。历史采用记录中的旧提交继续表示其原阶段依据，不能统一替换为本次提交。

| 参考文件 | 本次读取SHA256 | 可参考的行为 |
| --- | --- | --- |
| [LICENSE](https://github.com/multica-ai/multica/blob/22a05d832bbe8aee9b0873ff6bb31ae1fe122fc5/LICENSE) | `36ee69dfa6cc2bca7caffd27334792286bb7e40503b353803a926d1b7e3a6cd9` | Apache正文与附加条件组成整体 |
| [NOTICE](https://github.com/multica-ai/multica/blob/22a05d832bbe8aee9b0873ff6bb31ae1fe122fc5/NOTICE) | `4664f302153a9123352f3c2a31b3b3fe023ea5cb0be309ec17917621584fb380` | 原版权、许可范围与来源说明 |
| [runtime-choice.ts](https://github.com/multica-ai/multica/blob/22a05d832bbe8aee9b0873ff6bb31ae1fe122fc5/packages/views/agents/create/runtime-choice.ts) | `6345a021acf10a550ad74f228a32410c65d122e79beaab6b6e3ccb56bf3acc5a` | 按当前成员可用范围组织本机、远端与云运行时，展示来源及版本 |
| [models.ts](https://github.com/multica-ai/multica/blob/22a05d832bbe8aee9b0873ff6bb31ae1fe122fc5/packages/core/runtimes/models.ts) | `df853e43108d8ca4af0d11d8e32d89eca041db3472b7ea1a613ea5a468173e9c` | 区分进行中请求、明确完成、失败、缓存结果和显式刷新；失败不能伪成空目录成功 |
| [commands.ts](https://github.com/multica-ai/multica/blob/22a05d832bbe8aee9b0873ff6bb31ae1fe122fc5/packages/core/runtimes/commands.ts) | `79f7c20ce238c6c1106780f830f38138a2a0be6e7d7f4ed98de1ab2c8bc4d25d` | 区分云／自托管目标；当前配置未就绪时不生成可执行接入命令 |

该版Multica License保留第三方托管／商业嵌入、界面品牌及非界面归因等附加条件，不能只依据其中Apache正文把新版源码视为普通Apache素材。本次没有复制其产品源码、UI、样式、图标或依赖；只从行为与责任边界形成Suma自身实现。历史准确快照的既有许可与来源清单仍单独适用，不能据此采用后续新增源码。

## 本阶段采用方式

复用Suma已经存在的 `opencode-sdk` / `opencode-sdk-v1`、固定SDK／server 1.19.4和完整受信资产清单。快捷接入仅把明确的本机安装和固定提供方配置接入原托管Runner，再以启动后的新库存登记暂停档案，由用户独立启用。它不新增运行时、调度器、模型服务或数据库。

现有 `OpenCodeConfigurationDigest`、`validateOpenCodeRuntimeAssets`、原隔离工作树／宿主工具、固定 `OpenCodeExecutionCapability`、任务预览／Start／Claim和未知副作用核对继续是执行依据。原生OpenCode SHA与库存Node SHA是不同事实，必须分别比对；安装存在、配置有效、Worker登记和连接健康也不能合并。

不照搬参考实现的目录缓存时长、后台刷新或通用选模行为。OpenCode本阶段仍是管理员明确固定模型，任务读取原 `fixed_opencode`；没有新增模型目录发现CLI。原body/key恢复不更换配置，当前scope／工作区／原身份持续核验，新配置只能在原进程确认停止且无未结操作后明确采用。

本阶段平台改动仅为 `desktop_runner.go` 原登记白名单加入 `opencode-sdk`，保持HMAC、当前审批权限、原回执和空Worker profiles。平台定向验证为新增1个顶层用例RED→GREEN，受影响Codex登记旧用例1个通过；完整桌面／Runner／Web交付结论由本阶段统一报告记录，不能由这两项推定。

## 供当前能力表合并的修订建议

下表是对[现有参考能力表](multica-priority-map.md)的更新建议，原历史调查段落和阶段证据保留。

| 当前表项 | 应补充的限定事实 | 继续保留的缺口 |
| --- | --- | --- |
| M09／M27的“偏好降噪” | [167本人项目通知偏好](../../contracts/project-notification-preferences.md)已覆盖关注讨论静音，不能再把全部个人降噪写成未开始 | 其他通知分组、渠道及完整规模验收 |
| M15技能与复用 | [172正式交付知识来源](../../contracts/delivery-knowledge.md)及[173准确发布技能原生文件](../../contracts/codex-native-skills.md)已形成限定链路 | 其他工具原生技能、支持文件／外部包、自动优化评测与真实模型应用 |
| M11–M20工具／团队执行 | 在153／155／158／159之外补[170 ACP执行](../../contracts/acp-cli-execution.md)、[171 Qoder快捷接入](../../contracts/acp-quick-connect.md)、[174 Codex人工确认](../../contracts/codex-human-requests.md)、[175 Codex快捷接入](../../contracts/codex-quick-connect.md)和[176扫描预检](../../contracts/tool-onboarding-scan.md)的各自限定事实 | 真实厂商／账号／模型、物理多机、跨系统、续权及完整平台范围；171显式GUI停止既有缺口不隐去 |
| OpenCode当前目录与接入 | 原SDK执行和平台档案合同已有实现；177正在补桌面快捷入口，不应写成新执行适配器，也不能提前写成完整快捷接入验收 | 以177最终统一证据为准 |

完整31项／P0–P7与原58／34／92范围不因本次参考或登记白名单发生变化。
