# 构建环境与复现

状态：本机已验证，2026-10-23。负责人：平台集成。关联：P0、P7；证据见[验证报告](../quality/verification.md)。

使用 Go1.24.5、Node22.16.0、pnpm11.25.0 的 Windows amd64 环境完成本轮构建。依赖以 go.sum 和 pnpm-lock.yaml 锁定。CI 另定义 Linux/Windows 检查，未将未运行的 CI 作为通过证据。

## 工作区依赖

pnpm11 的 CI 与交互环境对全局虚拟存储的默认值不同。曾出现“安装成功，运行脚本又请求重装”的问题；以 `verifyDepsBeforeRun:error` 取得的明确原因是 enableGlobalVirtualStore 发生变化。仓库已显式设置为 false，保留工作区内 node_modules，脚本检查不匹配时直接报错。没有关闭依赖供应链检查。

项目 storeDir 位于 pnpm-workspace.yaml，使用 `.cache/pnpm-store`。pnpm11 不再从 .npmrc 读取该普通配置；.npmrc 只保留注册表设置。已有安装可以继续引用其既有内容存储；新安装按仓库配置建立缓存，不删除机器原有的共享存储。[pnpm11 官方说明](https://github.com/pnpm/pnpm.io/blob/main/blog/releases/11.0.md)

```powershell
pnpm install --frozen-lockfile
pnpm typecheck
pnpm test
pnpm build
pnpm check:docs
```

## Go 与测试目录

测试和构建缓存使用仓库忽略的 .cache，环境变量仅对当前进程生效：

```powershell
New-Item -ItemType Directory -Force .cache/go-build,.cache/ct | Out-Null
$env:GOCACHE=(Resolve-Path .cache/go-build).Path
$env:GOTMPDIR=(Resolve-Path .cache/ct).Path
$env:TEMP=$env:GOTMPDIR
$env:TMP=$env:TEMP
go test ./... -count=1
go vet ./...
```

Windows 的真实 Git worktree 测试会在 Go 临时目录内继续创建 `.cache/runner/worktrees/...`。014 本机复现到 Git for Windows 内部 `'$GIT_DIR' too big`；同失败项在较短的仓库忽略目录 `.cache/ct` 下通过。上方仅缩短当前测试进程的临时路径，不改用户／机器环境，也不在盘符根散落目录；更长工作区仍需按项目目录策略选择足够短的受控任务目录。该结果不代表所有平台安装与 Git 长路径场景已验收。

容器入口脚本测试需要 POSIX sh；Windows 可通过 SUMA_TEST_SH 指定已有 Git for Windows 的 sh.exe。该设置仅用于测试，不为 Runner 开启 shell。实际执行子进程采用明确命令许可、参数数组和显式环境；Go 测试命令还需要在许可环境中给出 GOCACHE/TEMP 等目录，不能依赖宿主机凭证或完整环境隐式继承。

正式部署前仍需执行目标平台安装、升级、备份恢复及真实数据库矩阵。本页不代表 OceanBase、容器或 Helm 已运行成功。
