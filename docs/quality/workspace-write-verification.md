# 执行工作区 CAS 保存：限定验证

状态：1.0.0／T-P4-038，本机功能和实际界面已验证，2026-10-27。负责人：平台集成、Control、Runner。[合同](../../contracts/workspace-write.md)、[架构](../architecture/remote-workspace-write.md)、[AI SOP](../operations/remote-workspace-write.md)、[英文](../en/remote-workspace-write.md)；[产品与交互](../product/remote-workspace-write.md)、[机器摘要和证据SHA](workspace-write-summary.json)。本报告只证明列出的范围，完整P0–P7、需求1–22及生产验收继续实施。

## 关键结果

Control、真实Worker及独立Node原生协议宿主在同一隔离实例完成：完整文件读取→CAS保存→旧摘要冲突→执行许可响应丢失10次并过期→显式inspect→持久not_started屏障，确认没有重新写入。唯一联合命令退出0、PASS，1197.21秒包含GUI等待，不是吞吐或延迟成绩。Control没有本地文件绑定，源管理员HEAD未变，59218已正常关闭；没有重复启动联合宿主或旧模型旅程。

原生准备／未知状态不调用旧的补写恢复；同锁串行human/model CAS只允许一方成功，generation递增使旧checks失效。外部字节恰好命中欲写SHA也不能当成原操作成功，专项先红后绿。晚到结果按原身份／请求／scope／fence排空，关闭目标或撤回写权不抹除已发生事实；不因此授予新执行权。

## 检查与命令范围

| 范围 | 真实执行与结果 |
|---|---|
| Control与受影响观察 | `go test ./internal/platform -run 'TestWorkspaceWrite|TestWorkspaceObservation' -count=1`，8.856s通过。包含原凭据／viewer、task依据变化、Pool当前授权与旧事实drain、唯一许可／超时不重派、inspect原key/version、无正文metadata与空poll |
| 原回执权限恢复 | `TestWorkspaceWriteCurrentAuthorityAndImmutableCommand`先红403再绿0.522s；原caller仅剩Read可恢复同key同body旧metadata，改body409、新key403 |
| Runner | 针对原子CAS窗口、许可与取消、原ACK、started journal恢复、remote prepared不补rename、tombstone、Pool完整envelope及FullFile的定向组通过；准确命令／源码／日志摘要保存在runtime-focused回执 |
| CLI／共享合同 | `go test ./internal/cli ./internal/workspacewritecontract -run TestWorkspaceWrite -count=1`通过；覆盖同内容稳定key、字节SHA、unknown非成功退出、错误URI命令回执拒绝、严格JSON及准确效果转换 |
| 前端 | RemoteWorkspaceEditor6个用例、原RemoteWorkspacePanel3个和IDE2个受影响用例取得通过证据。新增inspect丢回执一项最后单独选择执行，复用其余5项既有通过结果，不将选择过滤误报为未验收 |
| 静态／生成 | 最终TS类型、4563配对词条、受影响Go vet与`go run ./cmd/suma-contracts -check`通过。文档链接和Schema检查见交付汇总 |

Go通过仓库离线工具链wrapper执行，缓存和进程TEMP均留在忽略目录；前端使用已安装Node／Vitest／TypeScript／Vite，无新增依赖或SDK安装。首次构建启动因任务日志目录尚不存在而未执行Vite，创建该任务目录后继续；其后4次实际构建分别由初始集成和真实界面发现的生命周期／ARIA问题推动。

## 实际界面旅程

环境：Windows／IAB，通过CUA，`http://workspace038.localhost:59218`，桌面1265×712。两个自有测试标签页共享同一隔离账号，但各自取得原文件基线。

1. 标签A保留未保存草稿；标签B通过真实Runner成功保存。
2. A按旧SHA保存被拒，原草稿仍在。读取当前完整文件，左右显示真实差异；人工保留B修改和A草稿，明确勾选采用新比较基线后再次保存。
3. 准确回执确认命令 `e2804cefc5396a1192da5b8341cd3c3c`、generation3、提交文件SHA `49918b20e3d073e404805b75454b9b77f0469e71012dd6501c32b40bab4ed0cd`。本浏览器原凭据“最近保存”准确加载3项：2次成功和1次未应用冲突。
4. 切换中英文与刷新恢复；随后只做未提交的比较草稿，验证初次加载与语言更新时左右编辑区名称，并放弃草稿退出，没有新增文件写入。

原生联合预检的文件SHA是 `5423e6f83a23103da5c8d118254c1f055b578c688909e7ed8a46915736626e96`，它是GUI之前的值，不能冒充GUI最终文件。预检与GUI原调用凭据不同，保存历史按各自凭据隔离。

## 实际发现与修复

实际保存与冲突处理正确，但最初离开DiffEditor时出现一次TextModel提前销毁错误。定位已安装React Monaco4.7源码后，改为拥有独立URI的比较模型，先从widget分离，再释放；清理顺序由组件掌握，不忽略错误。

Monaco0.52的Diff内部无关选项更新也会清空内层ARIA名称。直接设置和构造选项不足以保持初始名称；最终使用公共配置事件、当前语言和相等性检查恢复名称，监听与模型同生命周期。真实初次加载、中英切换和退出复核通过，最终关闭宿主前相关console error／warn为0，原失败日志保留。

UI修复均先在新的忽略输出目录构建。保持同一已验证后端与接口，仅向本任务隔离web目录追加新哈希资产并原子切换HTML；旧入口与哈希单独保留，主035预览未触及。没有重开Go宿主掩盖旧问题。

## 证据与边界

精确路径和SHA见机器摘要；原始回执、截图、HTML及日志留在 `.cache/e2e/workspace-write-stage-038/`，不提交Git。最终图为已合并文件左侧与未提交预览草稿右侧；该预览随后明确放弃。两个代理测试标签页均关闭。

本阶段不代表完整远程IDE、PTY／LSP／DAP、跨物理机器或TLS、OceanBase／PostgreSQL容量、移动端或生产通过。0厂商／任务模型调用、0正式任务人工验收操作不等于未做工程测试。保存、候选、正式测试和业务验收仍为独立事实。主035预览保持原版本；后续需升级到当前已验证源码并继续完整平台建设。

收尾已核对240份Markdown本地链接、253条API路径和2291处Schema引用。宿主结束后删除2份逐文件已证明有保留副本的重复构建目录，回收32,571,344字节；清理前文件清单／SHA留在独立回执，最终web-ready、全部原入口和独特历史资产／截图／日志保留。
