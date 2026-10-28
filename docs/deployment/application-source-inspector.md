# 应用源码检查部署与配置

状态：1.0.0，T-APPLICATION-123部署合同；负责人：平台部署、仓库和应用领域。运行验收与跨主机部署验收分别记录，本文不宣称未执行的环境已通过。

关联：[应用架构](../architecture/project-applications.md)、[操作与API](../development/project-applications.md)、[仓库提供方部署](repository-integration.md)、[解析器说明](../../packages/source-inspector/README.md)。

实现入口：`internal/platform/application_source_inspector.go`、`internal/repositoryprovider/source_inspection.go`；证据从[质量入口](../quality/verification.md)追溯。

## 进程与凭证

沿用`suma-control`、既有Runner和`suma-repository-provider`。开发、独立测试和仓库集成仍沿原Go DAG；源码检查是对准确提交的有界读取及本机可信解析，不增加调度服务或模型调用。

提供方配置新增可选`source_read_token_env`。启用时必须指向至少32字节的独立进程密钥，不能与回调、Runner或reconcile密钥重用；未配置时源码接口拒绝访问。普通Runner、reconcile、浏览器、项目API或Agent凭证不能替代这个源码读能力。

在既有提供方JSON中添加环境变量名：

```json
{
  "provider_id": "repository-local",
  "tenant_id": "organization-id",
  "project_id": "project-id",
  "control_url": "http://127.0.0.1:4324",
  "control_token_env": "SUMA_REPOSITORY_CALLBACK_TOKEN",
  "runner_token_env": "SUMA_REPOSITORY_RUNNER_TOKEN",
  "reconcile_token_env": "SUMA_REPOSITORY_RECONCILE_TOKEN",
  "source_read_token_env": "SUMA_REPOSITORY_SOURCE_READ_TOKEN",
  "git_executable": "C:/Program Files/Git/cmd/git.exe",
  "targets": {
    "main": {"bare_repository": "D:/suma-data/repositories/project.git", "ref": "refs/heads/main"}
  }
}
```

控制面`SUMA_REPOSITORY_PROVIDERS_JSON`的对应条目也配置同一源码读环境变量名：

```json
[
  {
    "id": "repository-local",
    "tenant_id": "organization-id",
    "project_id": "project-id",
    "target_aliases": ["main"],
    "origin": "http://127.0.0.1:4351",
    "token_env": "SUMA_REPOSITORY_CALLBACK_TOKEN",
    "reconcile_token_env": "SUMA_REPOSITORY_RECONCILE_TOKEN",
    "source_read_token_env": "SUMA_REPOSITORY_SOURCE_READ_TOKEN"
  }
]
```

只在所需进程的环境或受管服务密钥配置中提供实际值。它不是业务表单字段，不写入Git、命令行、日志或浏览器。轮换时更新两端并重启对应服务；凭证独立不表示当前用户自动获得源码或登记权限，控制面仍核验项目及来源授权。

控制面支持验证正常证书的HTTPS提供方地址，也支持明确配置的字面回环IP HTTP地址；禁用代理和重定向。提供方命令目前只监听回环地址，远程部署需要管理员管理的HTTPS入口及反向代理。协议允许跨主机，不等于该部署已完成TLS、运维或容量验收。

## 固定解析器

受信构建产生`packages/source-inspector/dist/source-inspector.bundle.mjs`，同时生成参与构建依赖的`THIRD_PARTY_NOTICES.txt`。分发时保留许可文件。直接SDK版本为`semver 7.8.5`、`yaml 2.9.1`、`minimatch 10.2.6`，构建工具为`esbuild 0.24.2`，依赖由根锁文件固定；不安装被检查项目的依赖。

控制面启动前设置以下两个进程环境配置，均为管理员管理的绝对文件路径：

```text
SUMA_SOURCE_INSPECTOR_NODE=C:/Program Files/nodejs/node.exe
SUMA_SOURCE_INSPECTOR_ENTRY=D:/suma-runtime/source-inspector.bundle.mjs
```

两者都未配置时检查功能不可用；只提供一个或文件不符合合同会拒绝该部署配置。加载时解析并固定真实Node和bundle文件及摘要，每次调用前重新核对；更换文件后需要明确重新加载服务，不能把运行中路径替换当作静默升级。

控制面通过stdin发送经校验的`{schema_version:1,sources:[...]}`，只接收一个`SourceDependencyReport`。子进程使用DataDir内专用空工作目录、15秒截止、128MiB堆上限和白名单系统环境，不转发`NODE_OPTIONS`、供应商密钥或业务凭据。参数为固定脚本入口，不接受业务传入的shell、URL或本机目录。此约束不称为完整OS隔离；管理员仍须保护Node、bundle、配置、数据目录和服务账户。

## 固定资源边界

| 对象 | 当前上限 |
| --- | --- |
| 应用代码范围 | 8个 |
| 提供方请求范围 | 16个，规范排序且不重叠 |
| 单来源树记录 | 10,000条 |
| 清单 | 每来源与控制面合计最多200个 |
| 单清单原字节 | 256KiB |
| 解析调用合计原字节 | 8MiB |
| 提供方／解析器输入传输 | 12MiB |
| 解析器输出 | 1MiB |
| Node调用 | 15秒，128MiB堆 |
| 应用依赖图／影响 | 1,000个应用、5,000条边，影响深度64 |

提供方对树、清单和不支持对象保留覆盖状态；解析调用超限拒绝，不把截断输出标成完整图。缩小真实代码范围、减少无关清单或分开登记版本；业务请求不能提升硬限额。

## 数据、日志与故障恢复

原清单通过有界Base64运输，逐文件校验blob定位、原字节长度及SHA256，再以严格UTF-8解析。应用版本只保存文件和解析元数据、观察与人工决定，不保存原清单正文／Base64。诊断不包含原URL、命令参数、源代码正文或环境变量。

检查超时或暂不可用时保留浏览器输入，再重新预览。登记响应丢失时保持相同请求、预期版本和幂等键恢复原操作；不要换键重复登记。程序摘要变化、来源或范围变化时获取新预览，不放宽校验。撤权或提供方身份变化须先恢复正确授权，不回退到Runner密钥。

备份保留控制面数据库、提供方状态库、完整bare仓库对象／receipt引用、原候选和测试制品，以及匹配的管理员配置和解析器构建摘要。只恢复应用目录而丢失原交付依据不能证明当前来源资格。恢复后先核对身份、准确提交、回执和程序摘要，再允许新的预览和登记。

## 桌面与容器资源链

桌面标准prepare为当前目标构建一次固定bundle，将目标Node、原始Node许可证、解析器依赖notice及摘要清单放入隔离的`source-inspector`资源目录。原生目标默认采用打包进程的Node并核验22+版本；跨系统或CPU目标必须显式提供绝对`--node-binary`和`--node-license`，只核验目标二进制格式，不将未实际执行的版本／安装标成已验。SDK及最终资源验证核对摘要；启动仅使用包内路径，去除继承的`SUMA_SOURCE_INSPECTOR_*`配置，不改变主机环境。详见[桌面打包](desktop-installation.md)。

根Dockerfile的control目标从Node24构建阶段复制Node、bundle和许可文件，设置固定入口及必要共享库。此处交付构建配置；实际镜像构建、多架构运行、签名和安装仍需独立环境验收，不沿用本机协议夹具结果。
