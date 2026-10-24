# 容器、Compose、Helm 与 CI

这些配置面向当前单控制面开发版本。控制面镜像同时服务已构建的浏览器页面和 Go API；Runner、知识索引是独立可选进程。当前没有用模板替代真实 OceanBase、集群故障恢复或生产验收。

## 镜像构建

在仓库根目录使用 Linux 容器与 BuildKit：

```sh
docker build --target control -t suma-control:local .
docker build --target runner -t suma-runner:local .
docker build --target knowledge -t suma-knowledge:local .
```

[Dockerfile](../../Dockerfile) 固定 Go 1.24.5 与 pnpm 11.25.0，Node 使用 24 系列；依赖读取仓库锁文件。Go、浏览器和知识服务先独立构建，最终镜像不复制宿主机的 node_modules、数据库、秘密文件、缓存或既有 dist。发布时应保存实际镜像摘要，并在目标环境锁定已验收摘要。

`control` 是默认目标，含控制面、CLI 和浏览器静态文件。`runner` 只带 Runner、Git 和基础系统工具；所需语言编译器、智能体 CLI 及额外工具应由管理员构建进专用 Runner 镜像，并列入本地命令许可。`knowledge` 带原生 LanceDB 生产依赖及已编译的服务。

三个目标使用 UID/GID `10001:10001`。Compose 使用只读根文件系统、独立数据卷、临时目录和最小容器权限。源码和工作区不会自动进入控制面容器。

## 本机 SQLite：浏览器与 API

基础文件为 [compose.yaml](../../deploy/compose.yaml)，只启动控制面。镜像内监听 `0.0.0.0:4310`，主机只发布 `127.0.0.1:4310`；SQLite 与文件数据保存在 `control_data` 卷的 `/var/lib/suma`。

先创建独立的初始化秘密文件，再启动：

```sh
mkdir -p deploy/secrets
if [ ! -e deploy/secrets/setup-token ]; then
  (umask 077; openssl rand -hex 32 > deploy/secrets/setup-token)
fi
# Linux bind-mounted secrets must be readable by container GID 10001:
sudo chgrp 10001 deploy/secrets/setup-token
chmod 0640 deploy/secrets/setup-token
docker compose -f deploy/compose.yaml up --build -d
```

已有文件时保留其内容；由现有秘密管理方式提供初始化令牌即可。Docker Desktop 的挂载权限与 Linux 主机可能不同，应检查最终 UID/GID 10001 是否可读，避免为了启动而公开秘密文件。打开本机页面，在首次初始化表单填写该令牌，创建真实管理员和项目。初始化接口验证 `X-Suma-Setup-Token`。

非秘密可选配置样例为 [compose.env.example](../../deploy/compose.env.example) 和 [control.env.example](../../deploy/control.env.example)。复制为 `deploy/.env`、`deploy/.env.control` 后，可在仓库根目录使用：

```sh
docker compose --env-file deploy/.env -f deploy/compose.yaml config --quiet
docker compose --env-file deploy/.env -f deploy/compose.yaml up --build -d
```

`deploy/.env` 用于 Compose 参数替换；`.env.control` 只注入控制面。配置中相对路径按第一个 Compose 文件所在目录 `deploy/` 解析。控制面固定端口、目录、数据库类型及秘密路径由 Compose 明确设置；外部数据库应使用下面的专用覆盖文件。

Compose 将获准的秘密文件挂入 `/run/secrets`；镜像的 [入口脚本](../../deploy/entrypoint.sh) 将对应 `*_FILE` 读取到应用需要的环境变量中，不打印秘密内容。`*_FILE` 是镜像启动脚本提供的能力，Go 程序本身仍读取原有环境变量。环境文件、秘密和数据库不进入构建上下文或 Git。参见 [Docker Compose secrets 官方说明](https://docs.docker.com/compose/how-tos/use-secrets/)。

停止服务使用 `docker compose -f deploy/compose.yaml down`，保留数据卷。备份与恢复应遵循[服务器部署与恢复](server.md)，不要把运行中的 SQLite 文件复制当成一致性备份。

## 可选 Runner：明确工作区与本地许可

[Runner 覆盖文件](../../deploy/compose.runner.yaml) 需要四项明确配置：实际项目 ID、唯一稳定的 worker ID、已有主机工作区绝对路径、已有本地命令许可 JSON 绝对路径。另准备独立项目范围 `runner` 凭证文件 `deploy/secrets/runner-token`，并配置容器读取权限。

```sh
export SUMA_RUNNER_PROJECT=ACTUAL_PROJECT_ID
export SUMA_RUNNER_ID=UNIQUE_STABLE_WORKER_ID
export SUMA_RUNNER_WORKSPACE=/absolute/path/to/authorized/project
export SUMA_RUNNER_ALLOWLIST=/absolute/path/to/commands.json
docker compose -f deploy/compose.yaml -f deploy/compose.runner.yaml up --build -d
```

Windows Docker Desktop 可在 `deploy/.env` 使用 `D:/codes/project` 形式的绝对路径。工作区挂到 `/workspace`，必须让容器 UID/GID 10001 有完成已授权任务所需的权限。许可文件只读挂载，缺少路径不会自动创建主机目录。此覆盖文件显式启用 `--allow-exec`，执行范围由管理员提供的许可文件决定。

[commands.example.json](../../deploy/runner/commands.example.json) 只许可固定参数的 Git 状态读取。工作流节点的 `command` 只填写一个许可条目名，例如 `["workspace-status"]`；远端不能追加命令参数。要编译、测试或运行 Agent，先提供包含工具的专用镜像，再审核相应本地许可。不要挂载主机 SSH 密钥目录、整个主机文件系统或 Docker socket。

Runner 使用独立 `runner_state` 持久卷保存操作日志；停止信号设为 SIGINT，对应当前 Runner 的取消处理。`.env.runner` 是独立可选文件，样例见 [runner.env.example](../../deploy/runner.env.example)。传给子进程的额外环境必须由许可中的 `env`/`env_from` 明确声明。

## 可选知识索引

准备单独的 `deploy/secrets/knowledge-token`，该文件只授权给控制面和知识服务：

```sh
docker compose -f deploy/compose.yaml -f deploy/compose.knowledge.yaml up --build -d
```

[知识服务覆盖文件](../../deploy/compose.knowledge.yaml) 将控制面连接到内部 `http://knowledge:4312`，知识服务不发布宿主机端口，索引目录使用独立卷。独立 `.env.knowledge` 的非秘密样例见 [knowledge.env.example](../../deploy/knowledge.env.example)。默认为本机 LanceDB；向量嵌入需要真实 HTTPS 服务与固定模型、维度、版本，不会从配置存在推断验证成功。

知识服务的 `SUMA_KNOWLEDGE_HOST` 默认 `127.0.0.1`，容器设为 `0.0.0.0`。额外秘密如嵌入密钥或 OceanBase 检索连接串，需要自行显式挂载并设置 `SUMA_EMBEDDING_API_KEY_FILE`、`SUMA_KNOWLEDGE_OB_URL_FILE`，或由目标环境的秘密管理直接注入；基础覆盖文件不自动获取任何账户。

## 外部 OceanBase

[OceanBase 覆盖文件](../../deploy/compose.oceanbase.yaml) 仅替换控制面数据库驱动和秘密引用，不创建数据库集群：

```sh
docker compose -f deploy/compose.yaml -f deploy/compose.oceanbase.yaml up --build -d
```

将完整 MySQL DSN 安全写入 `deploy/secrets/database-dsn`，例如驱动所需的 `user:password@tcp(host:2881)/suma?parseTime=true&tls=true` 形式；密码和真实主机由管理员提供，文件权限与其他秘密一致。使用真实租户/账户、最低权限、已配置的 TLS 与备份。DSN 内容不要出现在 Compose 参数、镜像层、日志或提交中。

知识服务的 OceanBase 后端是另一项独立配置，不会随控制面的数据库驱动自动切换。实际版本的 DDL、锁、迁移、索引和恢复均需单独实跑；当前模板没有提供这些验证证明。

## Helm：单控制面与持久卷

[Helm chart](../../deploy/helm/suma/Chart.yaml) 创建一个控制面 Pod、ClusterIP Service 和 PVC；更新策略为 Recreate，避免 SQLite 在升级时出现两个写入控制面。即使连接外部 OceanBase，也保持一个控制面实例。没有 HPA、多副本或分布式成熟度声明。

先将已构建并验收的镜像加载到目标节点或推入你授权的镜像仓库，再配置实际镜像标签。已有秘密通过引用注入，不在 values 中填写值：

```sh
kubectl create secret generic suma-control-secrets --from-file=setup-token=deploy/secrets/setup-token
helm lint deploy/helm/suma
helm template suma deploy/helm/suma
helm upgrade --install suma deploy/helm/suma \
  --set image.repository=YOUR_REGISTRY/suma-control \
  --set image.tag=YOUR_VERIFIED_TAG
```

OceanBase 模式额外在已有秘密中准备 `database-dsn`，并设置 `database.driver=oceanbase`、`database.existingSecret` 和 `database.dsnKey`。可以通过 `persistence.existingClaim` 复用已有卷；默认 PVC 为 10Gi、ReadWriteOnce，卸载时保留。存储权限、卷扩容和恢复以实际 CSI/集群为准。

就绪探针访问 `/healthz` 检查数据库连接；存活探针检查 HTTP 监听端口，启动探针为迁移预留时间。这些探针不代表完整业务旅程通过。TLS Ingress/反向代理由目标环境提供，生产默认 `secureCookies=true`。只在本机端口转发临时验收时，才显式选择 `secureCookies=false` 以使用 HTTP 浏览器会话。

## CI 与本次验证边界

[CI 工作流](../../.github/workflows/ci.yaml) 定义 Windows/Linux Go 全量测试、静态检查与命令构建，Linux TypeScript 检查、包测试、浏览器构建和文档检查；部署检查包括 Compose 组合校验、Helm lint/render、入口脚本测试、三个镜像的构建及控制面页面/健康响应检查。镜像不会自动发布，不使用部署凭证。

本次本机已实际执行 Compose 基础与覆盖组合校验、秘密合并与知识端口检查、入口脚本两个测试、知识服务 TypeScript 检查，以及真实知识进程默认回环和 `0.0.0.0` 两种监听配置的健康检查。当前 Docker 客户端存在，但会话访问 Docker 引擎被拒绝，因此**没有实际构建镜像或启动容器**。Helm 未安装，官方校验工具下载遇到 TLS 失败，因此**没有本机 Helm lint/render 或 Kubernetes 安装结果**。CI 文件只定义未来检查，尚不构成 CI 已运行的证据。
