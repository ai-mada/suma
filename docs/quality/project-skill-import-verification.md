# 本地技能文件导入验收

阶段 T-PROJECT-SKILL-IMPORT-283，版本 1.0.0。[产品交互](../product/project-knowledge.md#导入本地技能) · [架构](../architecture/project-knowledge.md#本地技能文件解析)

- 解析器 11 项定向测试通过，覆盖 BOM、CRLF、多行说明、重复键、别名、UTF-8 和字节限制。编辑器 3 项定向测试通过，覆盖显式保存、无效文件后重试以及关闭时丢弃迟到读取。
- Windows 上的隔离 Chrome 连到本机控制面和全新 SQLite 数据库。英文浅色 1366px 与中文深色 390px 均选取真实浏览器文件对象，预填技能并编辑正文。选取和编辑期间没有写请求。保存后两种布局各产生一次 HTTP 201，刷新和详情读取均返回 `kind=skill`、`state=draft`、`published_revision=0`，保存了编辑后的正文。中文窄屏另验证了错误文件后重新选取，以及无需滚动即可看到保存按钮。没有页面脚本错误。
- TypeScript、中英文词条、文档链接和 OpenAPI 引用通过。工作台引用仓库已锁定的 YAML 2.9.1，并在选择文件时加载解析器。离线重装在全仓供应链核验阶段遇到缺失的 `@ai-sdk/provider` 镜像元数据。运行验证使用工作区原有的 YAML 2.9.1，完整离线安装尚未通过。

```powershell
pnpm --filter @suma/web test src/skill-markdown-import.test.ts src/project-views/KnowledgeSkillImport.test.tsx --minWorkers 1 --maxWorkers 2
pnpm --filter @suma/web typecheck
pnpm --filter @suma/web check:i18n
go build -p 2 -o .cache/q283/control.exe ./cmd/suma-control
node scripts/check-docs.mjs
```

阶段覆盖本地单文件到项目草稿。远端仓库、ZIP 包、配套文件、跨端同步与生产部署仍待交付或验证。
