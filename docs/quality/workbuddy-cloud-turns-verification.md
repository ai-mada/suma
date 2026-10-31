# 原云任务持续协作：验证与交付边界

T-WORKBUDDY-CLOUD-TURNS-169；suma 1.0.0；2026-10-31；负责人：主集成／后端／运行时／工作台。状态：源码、定向检查、七项实际业务及独立键盘／窄屏视觉验证通过；原失败记录保留。关联[合同](../../contracts/workbuddy-cloud-turns.md)、[架构与AI SOP](../architecture/workbuddy-cloud-turns.md)、[交互](../design/workbuddy-cloud-turns.md)和[机器摘要](workbuddy-cloud-turns-summary.json)。完整31／P0–P7／58限定／34待交付／92总账继续有效。

## 源码范围

原云节点新增独立“继续协作”：本人明确新输入、一轮真实SDK Prompt／Next、持久回复及工具状态、准确once权限选项、取消和失联核对。复用原task／submission／attempt，不重送最初Create目标，不增加DAG dispatcher。所有控制面副本可写正式inbox，由原lease runtime执行，不能靠内存sticky session冒充分布式控制。

新的6表在任何DDL前校验，正式head／原请求／permit摘要／输出／权限／命令／工具账本与当前授权、CAS及receipt配合。未知和已发送操作不重放；取消HTTP ACK、本回合RPC结束、远端进程停止、代码交付、独立测试和人工验收分别保留。权限细节不完整或被脱敏时不允许审批，always选项不执行。模型及价格未由平台控制，缺失usage不补零。

## 定向检查与修复

后端初始15个不同顶层（协议DTO2、领域13）、9子例分批通过；运行时初始9顶层／4子例通过。Root公共同源合同、错误双语、正式control退出排空各1顶层。Web模型6／组件8共14顶层通过，自动读与opaque选项只复验受影响原用例，未把重跑累加为新用例。

独立review发现Cancel ACK会把快照后新到的权限误记确认，已用领域子例和真实SDK／SSE竞态分别RED复现。修复私有ID／version快照、runtime实际已观察集合交集及准确ACK；新请求／版本变化／sent／unknown保留。只复验新增与原取消受影响项，独立review确认该P2关闭。后端新增0顶层／1子例，运行时新增1顶层，不覆盖原证据或重全套。

随后修复两个正式状态缺口：最后准确权限ACK且无其他待决／unknown才waiting_permission→running；活动回合截止或lease失效在View、原回执和Page统一overdue，停止无意义自动刷新并提供核对。分别只扩原用例并复验，原取消与unknown事实不被改写。

原后端receipt SHA`d7a289f9d52e701e17625649037ba2df610e940addb25a1685c5d9a891d68cde`；修订依次为取消范围`5d7bf0a92e4f453a33760f8dcdac0e5845caca8194ab7b5aef6d1f318d168a53`、权限状态`d88bc2864fb47c1e9f22ec2c2f08bc83c8020e94ea869107fab4b12252bc86c8`、失联投影`0d86d31cdbbbf37d75cb67497e8288e20dcc8896174a18208ca8b5797b2fc2ee`。最终8源摘要`9fdc70081502457f37502e5261fbee5406fd970ea7d8b9461bb7f7558316ac5d`。

原runtime receipt SHA`9571efbf38364d1b1a3937d32cec56e6d520efca4fec763c8d3cff459173c822`；取消范围修订`53e53eed4f30c704d75ba146ffea6f88a3fb5e4f26da97d3cb00c90021ec07ad`。原Web8文件receipt SHA`3d55ac86d96828d38f7c118a8fec76c87f3808e61b64e413727a5eb4930a0d6e`。后续原始选项名称与准确失联文案的局部修订另外记录，原14个不同测试与完整检查来源不混写。

Root完整TSC退出0、8016对中英／占位／复数／标记检查通过，生产硬编码0；其后展示小修只定向验证。813声明模型／814schemas／385路径，新增11DTO／5路径／6操作，原380路径／803schemas结构不变。共同successStatus先前仅支持200／201，新增异步202合同用例真实失败后修复为保留实际成功状态；只补验该1项，不重复计数。

最后一次`node scripts/check-docs.mjs`退出0：493份Markdown本地链接、385路径和3548个Schema引用检查通过。后续仅补写本检查结果，不重复执行。最终不同顶层共42项（Go28＋Web14）；两个fixture失败与独立视觉续验单列，不叠加为新自动化用例。

## 本地生命周期与原工作保护

控制进程退出用10秒有界helper正常关闭本地连接，不自动发远端Cancel；runtime注册与关闭在同mutex内，封住关闭后的新连接。新增late-permit实际SQLite聚焦用例证明零外发／known-not-sent留痕。操作系统信号及安装包全矩阵不由这项helper检查推定。

`cmd/suma-control/main.go`原有未提交工作只增加1行必要defer，删除此行可精确重建任务前文件SHA。提交时仅将该行应用到HEAD版本的index；原用户改动不整体暂存。其余原20项按既存SHA保护。阶段仅提交必要源码、合同和文档；缓存、原日志、私有DB／prompt／凭据、二进制与图片保持忽略。

## 隔离实际旅程

首次Go宿主编译0、隔离生产组件构建0；夹具统计查询误用表内不存在的`started_at`列，准备阶段退出1，0 GUI检查／0新Turn／0新Prompt。原任务建立涉及受控Create，不能称没有任何副作用；该临时库已由正常测试清理，未声称恢复它。原17项证据、10份helper原字节与首个二进制／UI输出保留。仅修查询为`ORDER BY id`及版本化输出域；新96输入门禁核签后第二次只重编译Go，按SHA复用原UI四个输出，无二次前端构建。

第二次使用独立新准备身份，由正式任务领域／原claim-Create-result建立一个原任务／Run／submission。两套正常Server及独立SQLite连接共用持久业务，Prompt发primary，权限和取消HTTP发secondary；primary原lease runtime通过真实ACP SDK及受控本机HTTP／SSE执行。七项业务检查全部通过：原来源与状态；首Prompt提交后截断202；同body/key恢复；有界自动GET显示真实回复和准确权限；secondary权限提交后丢响应并准确恢复；第二明确输入；secondary取消只结束准确原RPC。共6个GUI POST运输、45个GET、2 Turn／2 Prompt／1 permission reply／1 Cancel，原任务／Run／submission各1。T1`end_turn`／partial用量，T2`cancelled`／unreported；原DAG仍running／node waiting／attempt1／fence1，task仍in_progress，候选／独立测试报告／人工验收0。

原驱动第八项在英文390刷新、终态GET恢复、CSS令牌／无横溢出及0新增POST断言之后，用`HTMLElement.focus()`却要求键盘`:focus-visible`轮廓，错误断言失败。原outer／Node退出1、completed=false保留；不将局部通过改写为第八项通过。页面错误、console错误、外部请求均0。finally中Go宿主、代理和Electron均正常退出0，9个观察进程已退出、3端口释放、未强停，2928640字节私有一致快照保留，SHA`815d897c742082cbea5910f99ca6ebd926e6d0164262cdd13cc32fe543a5bf81`。这是同机两逻辑服务与受控厂商协议，不是物理多机或真实商业模型验收。

Root已检查中文1366×960原图：真实样式、实时回复、权限卡标题和原工作区边界可见；它是滚动局部视口，权限卡下半部未入图，不能称完整页面或完整权限弹层截图。英文键盘／窄屏只读续验另记，不重跑这七项业务，也不覆盖失败出口。图片、数据库、prompt／凭据、原日志和构建输出均保持忽略，最终紧凑证据写入本文及机器摘要。

独立视觉续验使用同一原快照持久DTO及原生产JS／三CSS，Session／run／node／connection启动envelope明确为夹具组合，不能当作原认证API复验。以真实Tab进入refresh，`:focus-visible=true`／solid 2px；390×844下scroll=client373，令牌16／24／18，8个只读GET／0 POST／0其他写入、0页面／console／外部请求。1个视觉用例、1张英文局部图及命令退出0；native／Node／只读代理均正常退出0，6个观察进程已退出、端口释放，同一快照摘要不变。无新Go编译、前端构建、SDK或业务执行。Root已独立查看英文图及原中文图；第八原业务检查依旧partial／failed，未借此改写。

英文图SHA`e39725cacd86391f62c8c5e8a969803538f912bb2df1419f8be949ddfcc31f81`，中文图SHA`1b6f75ac326eead90a267d2a19a15081bebfb26f57f4617f8694707390810d38`。私有原件与安全导出保存在忽略的`dist/quality/workbuddy-cloud-turns-169/`，简明报告与摘要入Git；完整App、全设备／主题、其他数据库及真实供应方仍需独立验收。

## 继续交付范围

真实WorkBuddy及模型／费用、未知副作用的供应方核对能力、可信云候选／代码基线／同提交独立测试／验收、完整CLI／专家团队、其他数据库、物理多机、安装／OS信号／规模和生产仍待分别实现或验收。本阶段输入与权限是本人原云任务上的明确授权，不能伪装Agent已获通用云权限、云OS沙箱或全部平台功能完成。
