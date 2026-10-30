# WorkBuddy原云任务会话加载验证

T-WORKBUDDY-CLOUD-COLLAB-164；产品1.0.0；2026-10-31。负责人：控制面、工作台领域与主集成。关联[会话协作合同](../../contracts/workbuddy-cloud-collaboration.md)、[原云任务执行合同](../../contracts/workbuddy-cloud-execution.md)及[机器摘要](workbuddy-cloud-collaboration-summary.json)。基线`24cce4a11922bbe2df70f9f61273f5bb67343df3`，完整平台与完整人工协作目标继续保留。

## 本阶段实现

当前有权的原提交owner可明确连接原会话，使用仓库固定的ACP Go SDK完成initialize／session/load并保存本次观察。平台存档GET及本地分页不Attach；本次外读需要显式POST，不能称为供应方无副作用的只读请求。浏览器不接收票据、ACP URL、RPC引用或任意协议发送权。

原来源、当前真人项目权限及本人连接资格在外读、逐项采集和最终发布时重新核对。head、load、entry独立持久；原operation_id、head_version_at_start和submission版本绑定回执，同key核对原load，不再次Attach。首次head版本必须明确为0。数据库截止与限额保护授权前缀和上次已加载观察，HTTP与RPC均确认后才发布loaded；coverage始终为`unverified`。

真实Run节点的原云工作区挂载独立会话区域，沿现会话与ActorView发送请求。正文只展示有界字面文本和安全摘要；旧范围、失权或来源变化清除自身受保护内容，不擅自清除仍有权读取的原节点事实。读取已存记录、明确外部加载、未知原操作恢复和本地“更多”分别呈现。加载确认不代表完整历史、任务完成、候选、独立测试或业务验收。

## 定向检查与复核

| 范围 | 通过 | 新增／原有 | 结果依据 |
| --- | ---: | ---: | --- |
| Go平台域 | 14 | 12／2 | `backend-final.jsonl`，8.219秒，退出0 |
| Go云合同包 | 14 | 5／9 | `backend-contract-final.jsonl`，0.150秒，退出0 |
| Go公开合同 | 1 | 1／0 | 原工具记录`1cdc19`，退出0 |
| Web | 33 | 25／8 | 模型16＋新组件9＋原Workspace8，按不同测试身份去重 |

后端覆盖准确owner／来源、原key重放、外读中撤权、授权前缀与last_loaded、项目四并发、旧回调fence、数据库截止、HTTP／RPC不一致和schema写前拒绝。并发限额使用两个Service及独立SQLite连接；不据此声称物理分布式或其他数据库通过。

Web模型16例与新组件9例共25新增，原Workspace8例保留。来源409清除正文的新回归组合真实原Panel与协作组件；最后9例组件结果包含先前新增例，不再次累加。初次类型检查发现负向`overdue:null`夹具越过正式类型边界，仅修夹具；对应一次定向复验属于原16例，不另计新增。后端及前端RED、夹具前提失败和最终GREEN记录均保留。

主集成最终类型检查退出0（`e539b1`）；7777对语言词条检查通过、生产硬编码0（`b6595e`）。公开合同生成的原工具`a69ced`记录791项声明／373条路径，当前生成物含792个schema；新增六个协作DTO和三条路径，原路径／schema无变更。独立审查所发现的来源冲突清理与迟到读取边界已关闭。

`contracts-test.jsonl`和`contract-generation.log`均为0字节，未将其当作JSON测试结果或补造stdout。`types-final.log`为0字节属于tsc无诊断的正常输出，退出码依据原工具记录。本报告落盘后，主集成一次文档检查退出0（`67e42e`）：478份Markdown本地链接、373条API路径及3446项schema引用通过；只回填该次结果，不重复运行。

## 真实HTTP与App串联

Windows Electron35.7.5软件离屏环境按需加载真实App／Run／Workflow／云工作区及原样式；通过真实`/auth/session`、原浏览器cookie和ActorView访问真实Server、Service、Engine与SQLite。没有纯组件mock替代此链路。供应方部分复用既有受控HTTP/SSE夹具和真实固定SDK，原已知提交由领域夹具准备，不是真实厂商或新的正式Runner执行验收。

最终同一隔离环境完成13项不同检查：

1. 实际会话确立原owner及项目ActorView。
2. 首次有权元数据GET保持version0、无head／load，外部Attach为0。
3. 原云工作区实际挂载，共享React运行时身份唯一，业务写前仍为0 Attach。
4. 明确加载先提交HTTP200，再受控丢失浏览器响应；只创建一个load。
5. 读取本地已存记录不清除未知原操作，也不再次Attach。
6. 明确恢复发送逐字相同body／key，返回原load、原operation及loaded／unverified回执。
7. 本地50＋5条按sequence稳定追加，不访问供应方历史分页。
8. 正常刷新App恢复原存档及本地第一页，不再次Attach。
9. 页面可见“覆盖未核实”边界。
10. 正常空闲后窄屏读取按钮保留键盘焦点、2px可见轮廓及44px目标。
11. 原DAG状态、原因、版本、attempt和fence保持不变。
12. 只有initialize／session/load，无Prompt、Cancel或权限回答，未因加载再创建云任务。
13. 实际App无渲染异常或意外业务写入。

最终SessionAccess、Attach、initialize、session/load各1次；ACP POST共2次，平台history POST运输2次但load实体仅1个。55条记录按本地50＋5读取，刷新不增加外读。原运行仍为`running`，原节点为`waiting`、原因为`cloud.observation_unconfirmed`，attempt1／fence1不变；本次GUI不声称覆盖`waiting_input`。

45份冻结输入在运行前后及收尾核对一致，摘要为`f7f6af9e40482c5f49ff0d44a553e0d3c0ddf519498ce334a9a142acd3b8a7de`。Go实际夹具仅编译一次，二进制SHA为`18b1ef9ba016f7be70eef5d4f50297407be6f33d4263a5f583439bec192e085c`；后续未重编译。真实Web入口`apps/web/index.html`独立记录原SHA，未改源码；Vite按需加载，无完整Web构建或整仓源码副本。

## 宿主失败与同环境续验

首尝试在历史动作前因Vite优化代次混用React运行时失败，0检查、0历史操作。第二尝试实际挂载后被夹具过严的“所有entry缓存标识相同”门禁阻止，2项读取检查、0历史操作；Vite允许未变entry保留自身标识。三轮各由领域夹具准备一个受控已知任务，身份独立；前两轮私有快照及外部计数均证明Attach／历史POST为0，原始日志、截图和资料完整校验归档，不并入最终13项，也不把重建隔离夹具称为同一原任务恢复。首次历史POST以后，始终使用第三轮原环境与原load。

缓存宿主保留已发现16项依赖，根目录改为真实Web目录并保留敏感文件／private拒绝规则；共享React运行时身份与真实工作区挂载作为正确前置，合法entry标识差异只作记录。正式45份源码及Go夹具未因宿主问题改动，也未新增依赖或重跑已绿单元组。

第三次原`verify.cjs`在前9项功能检查后，键盘断言与窄屏焦点转移时序相遇，退出1；原窗口、API、数据库、profile及已确认load保持存活。正常idle＋两帧后同窗口键盘复核通过。首收尾截图辅助脚本又因假定844px而收到843px图片，退出1，原图与日志保留；最终仅明确视口和正常滚动，完成同环境剩余只读收尾，退出0。没有在history POST后再造夹具、换key、重发新load或修产品代码；不将这段历史写成一次全旅程退出0。

## 图像、快照与正常收尾

最终中文1366×960和英文390×844均为原始viewport PNG，主集成已分别目视确认呈现及边界通过。两图展示加载确认、覆盖未核实和原字面正文；不以图像推断供应方完整历史。早期图、原命令退出1、首捕获退出1及最终续验退出0分别保留。真实stdout／stderr已落盘；仅有工具输出的只读键盘复核未补造原始日志。

窗口关闭、原生window closed、before／will quit、Electron、Vite和Go host退出分别确认；最终六项关闭证据均通过，准确PID后验均不存在，未强制停止。helper原数据库与DataDir确为任务TMP下`t.TempDir`；HTTP关闭后、数据库仍开放时，用参数化`VACUUM INTO`导出到原先不存在的私有目标，再由正常helper清理原路径。最终两个原临时路径均已确认不存在。

私有一致快照保留2846720字节，SHA为`188cd4a6fc9765e0109d41a08d7957c8dc040dcfbdcf292b81ede27c55fb6a63`。关闭后只读审计得到head1／load1／entry55／submission1，快照SHA未改变。cookie、票据、fixture密钥、数据库、原body／key及浏览器状态仅留忽略缓存，未交付或提交Git。

安全产物位于`dist/quality/workbuddy-cloud-collab-164/`。11项安全文件manifest的SHA为`0574c564b93effd178308ad6db72b5eb2cec0bec615f5eb8c7f71ad4d6f9c3df`；安全回执、实际摘要、源码指纹与最终两图均有长度和SHA，私有材料与原始流不在交付清单。

## 验收边界

本阶段证明原会话明确加载、当前授权存档、原操作恢复、本地分页及代表App交互。没有真实WorkBuddy账号／供应方、Prompt回合、权限处理、主动取消／远端停止、完整历史覆盖、团队共享存档、模型调用、新Runner执行、物理移动设备、多机、其他数据库／系统、规模、安装包或生产验收。loaded仍是本次加载确认，原等待态不因此解除；完整人工协作与完整平台目标尚未完成。完整31项要求、P0–P7及58限定／34待办／92总账不变。
