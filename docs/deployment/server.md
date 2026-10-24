# 服务器部署与恢复

状态：本机可运行；团队/集群模板需按实际环境验收。

## 配置

| 环境变量 | 默认 | 说明 |
|---|---|---|
| SUMA_ADDR | 127.0.0.1:4310 | 非回环绑定要求设置首次所有权初始化凭证 |
| SUMA_DATA_DIR | .suma | 业务数据和文件目录，禁止提交Git |
| SUMA_DB_DRIVER | sqlite | sqlite / oceanbase / postgres |
| SUMA_DB_DSN | 数据目录/suma.db | 服务器数据库必须显式配置 |
| SUMA_INGRESS_MASTER_KEY | SQLite 自动持久化 | 入口密钥的 AES-256 主密钥；OceanBase/Postgres 必须显式提供同一份规范 base64 编码的 32 字节密钥 |
| SUMA_WEB_DIR | apps/web/dist | 静态浏览器产物 |
| SUMA_SETUP_TOKEN | 空 | 外部绑定时初始化请求的X-Suma-Setup-Token |
| SUMA_ALLOWED_ORIGINS | 本机5173开发服务 | 生产只填写真实站点，不设置通配符 |
| SUMA_WORKSPACE_ROOT | 空 | 明确允许编辑的代码根目录 |
| SUMA_KNOWLEDGE_URL/TOKEN | 空 | 内部检索服务地址与独立凭证 |

SQLite控制面使用单机进程，外部访问通过TLS反向代理。同源HttpOnly Cookie用于浏览器；Runner使用独立短期Bearer凭证。运行时、数据库和模型密钥通过环境或秘密管理提供。

SQLite 未设置 `SUMA_INGRESS_MASTER_KEY` 时，在 `SUMA_DATA_DIR/automation-ingress.key` 生成并复用私有的原始 32 字节文件；数据目录必须持久化。既存文件无效会阻止启动，不会覆盖或重新生成。OceanBase/Postgres 不自动生成节点私钥，所有连接同一业务库的控制面与恢复实例必须使用同一份主密钥。环境变量要求无空白、规范 base64 的 32 字节密钥；不要把其值写进 Git、配置样例或命令参数。

Docker 镜像可用 `SUMA_INGRESS_MASTER_KEY_FILE` 从挂载秘密文件载入该 base64 文本；Go 程序直接启动时仍只读取 `SUMA_INGRESS_MASTER_KEY`，应由服务管理器/秘密管理注入。Helm 通过已有 Secret 的 `secretKeyRef` 注入，参见[容器部署](containers.md)。这份加密主密钥与发送端的入口 HMAC 签名密钥不同；轮换单个入口签名密钥不会更换主密钥。

## OceanBase首验

锁定CE4.4.2 LTS BP3（010580d）。创建独立业务租户/账户及数据库；连接使用MySQL协议。业务迁移、锁语义、向量DDL和备份恢复必须在真实版本实跑。数据库兼容不能从驱动连接成功推导。

## 发布步骤

1. 构建Go控制面/Runner与浏览器静态文件，保存版本、摘要和依赖清单。
2. 在隔离目标执行迁移与恢复验证。
3. 配置TLS、身份、节点、配额、存储和模型能力。
4. 运行五条旅程及撤权/断网/重启场景。
5. 以实际测试报告批准版本。保留旧产物、备份和迁移记录。

## 备份

停止写入或使用数据库一致性备份接口，校验业务库、原件和索引配置。当前不提供“运行中直接复制SQLite文件”的备份按钮。恢复到独立目录/数据库，核对记录、摘要、权限与关键查询后再切换。

数据库备份必须配套安全备份入口加密主密钥，并限制独立访问。SQLite 自动生成的文件是原始 32 字节；环境/秘密文件是同一字节的规范 base64 文本，不能直接把原始文件当作 `*_FILE` 文本使用。升级、迁移至共享数据库或恢复时沿用原密钥；已有 SQLite 数据切换到显式密钥时应安全地编码既有原始文件，不生成新值。丢失或替换主密钥会使已保存的来源秘密无法解密，当前不提供主密钥重加密/轮换迁移工具。

生产回退必须处理新版本上线后的新增写入。仅切回旧数据库地址会丢失数据，不作为恢复方案。
