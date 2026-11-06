# 命令行技能导入验收

阶段 T-CLI-SKILL-IMPORT-284，版本 1.0.0。 [操作说明](../product/project-knowledge.md#命令行预览与保存) · [读取与写入合同](../../contracts/project-knowledge.md#从-skillmd-创建草稿)

- Windows、Go 1.26.10。10 个定向顶层测试通过，覆盖真实 SQLite/HTTP、已有项目知识读写、常见 YAML 1.2 元数据、有界文件读取与错误恢复。Windows 当前进程缺少创建符号链接权限，对应子用例跳过。UNC 和不支持的路径独立覆盖。
- 使用真实授权的本机服务验证零请求预览、项目不匹配的本地拒绝、显式提交和原请求重放。两次合法 HTTP 写请求返回同一草稿 ID，数据库只有一份草稿及一次修订，`published_revision=0`。
- 构建真实 CLI，在不可用的控制面地址验证离线生成 JSON。中文说明、项目范围和版本字段正确，额外权限字段未进入草稿。中英文帮助均通过。Go YAML v3.0.6 已使用官方模块代理和校验数据库核对，锁定依赖后完成离线构建。 [第三方声明](../delivery/third-party.md)

```powershell
go test -p 2 ./internal/cli -run '^(TestKnowledgeSkill|TestSkillMarkdown|TestReadSkillMarkdown|TestProjectContext)' -count=1 -v
go build -p 2 -o .cache/q284/suma.exe ./cmd/suma
.cache/q284/suma.exe --language en-US knowledge import-skill --help
.cache/q284/suma.exe --language zh-CN knowledge import-skill --help
node scripts/check-docs.mjs
```

验证覆盖 Windows 的本地单文件与 SQLite。远端技能包及附属文件仍待接入。其他操作系统的实际运行和生产部署仍待验证。
