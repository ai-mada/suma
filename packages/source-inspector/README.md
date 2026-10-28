# 固定源码依赖观察器

版本1.0.0。公开输入／输出类型以 `internal/repositoryintegrationcontract/source_inspection.go` 和 `source_dependency.go` 为唯一合同；此包不拥有仓库、应用或审批权限。

可信宿主通过stdin提供 `{schema_version:1,sources:ProviderSourceInspection[]}`，解析器验证固定快照、文件摘要、Base64及UTF-8后，输出一个 `SourceDependencyReport`。进程不读取项目目录、环境变量或URL，不执行清单脚本；这不等于完整操作系统隔离。源码内容不写日志、报告或临时文件。失败只输出固定错误码。

当前范围为npm／pnpm工作区的package.json声明和pnpm-workspace.yaml。支持dependencies、devDependencies、peerDependencies、optionalDependencies，以及明确的workspace版本、别名和相对引用；纯相对file／link仅匹配输入中已经存在的包，不读取链接目标。清单以外、缺失目标、非标准工作区模式及不支持的声明保留诊断。HTTP／Git等不支持的定位信息不回显原文，只保留原字节摘要和 `range_redacted`。

工作区模式当前只支持有界的普通路径、`*`、`**`和排除项；复杂扩展模式明确返回unsupported。外部锁文件只保留原文件依据，不解析已安装版本或供应链漏洞；运行时与其他语言的依赖均未覆盖。人工校正、正式采用、版本生成、任务和发布必须回到控制面。

总输入12MiB、合计原字节8MiB、报告1MiB；每来源固定文件／树／单文件上限沿仓库合同。超限失败，不生成截断的完整图。

工作区依赖由主集成代理统一更新锁文件。随后使用 `pnpm --filter @suma/source-inspector build` 生成完全打包的 `dist/source-inspector.bundle.mjs`及参与构建的依赖原许可 `dist/THIRD_PARTY_NOTICES.txt`，分发时一并保留；运行时只依赖Node22及其内置模块。主进程固定Node与bundle摘要并在每次使用前复核。单个行为用例使用 `pnpm --filter @suma/source-inspector test`，无需模型、网络或源码依赖安装。
