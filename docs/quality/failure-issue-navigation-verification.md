# 问题详情到原失败运行

速码 1.0.0，`T-FAILURE-ISSUE-NAVIGATION-269`。关联需求为问题追溯、独立验证、智能分诊与中英文工作台。 [交互](../design/failure-diagnosis.md#入口与主路径) · [接口合同](../../contracts/failure-diagnosis.md#公共接口与dto-a先实现精确go类型)

Multica 将运行任务与问题通过持久标识关联。速码沿正式投影回执反查问题对应的原运行，沿既有运行和节点面板处理待核对与失败事实。 [Multica 固定参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/pkg/db/queries/qoder_issue_lifecycle.sql#L1-L10)

## 验证记录

- Windows、Go 1.26.10。2 项真实 SQLite 定向测试通过。覆盖编辑运行展示字段、手工问题、缺少或冲突的证据、重复来源、查询校验和 Runner 权限。生成并核对 1086 个模型及 491 条 API 路径，结构比对只增加该接口和响应模型。
- 10 项组件测试、TypeScript 与中英文词条检查通过。涵盖不提前请求、切换问题终止原请求、范围核对、原尝试选择与草稿保护。
- Chromium 使用真实组件和受控 API 响应，在中文 390 像素与英文 1280 像素验证键盘展开、原运行链接、普通问题空态和横向布局。两个语言各验证正式问题与普通问题，读取记录无写请求。证据在本地忽略目录 `.cache/q269/`。
- OceanBase 与 PostgreSQL 实机、海量回执下的查询耗时和真实模型调用仍待专项验证。本阶段反查在单个项目的故障回执范围内运行。
