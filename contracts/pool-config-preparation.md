# Pool 多项目本地配置准备

状态：T-POOL-CONFIG-PREPARATION-209 限定实现与验证通过；版本1.0.0。完整31项／P0–P7继续。关联：[原Pool执行](execution-pools.md)、[机器配对](pool-runner-pairing.md)、[后续计划](../docs/delivery/tool-management-next.md)。

## 一个配置权威

原 `PoolLocalConfig`、`DecodePoolConfig`、PoolSession、双边Grant、工具库存、配置启用及派发保持唯一权威。本阶段只将目标机器已有的私有允许列表和项目路径组合为一个全新本地配置，不新增数据库、API、凭据、派发器或Worker。配置生成不代表授权、连接、安装、健康或执行成功。

Pool拥有者可以读取已授权的Pool／节点和Grant元数据，不因此获得消费项目的仓库、命令、模型或其他业务数据。表单只填写目标机器路径和明确的租户／项目标识；不调用消费项目接口，不要求将命令、环境值或凭据粘贴到浏览器。表单草稿只在当前组件内存中，账户／组织／Pool／节点／API来源变化即丢弃，不持久化到浏览器存储或控制面。

## 固定私有输入

新CLI：`suma-runner --prepare-pool-config NEWFILE`。只接受私有标准输入，除语言／帮助／版本信息外不与其他模式或执行选项混用。目标为已存在目录中的全新绝对文件；沿原根目录锚定、O_EXCL和0600新文件防护，不覆盖、不创建父目录、不跟随链接／别名。先拒绝已有输出，再读取输入。

输入上限64 KiB，闭形且拒绝重复键、null、缺失、未知字段、错误类型和尾随内容：

```json
{
  "pool_id": "registered-pool",
  "runner_id": "registered-runner",
  "slots": 1,
  "scopes": [{
    "tenant_id": "approved-tenant",
    "project_id": "approved-project",
    "workspace": "/existing/workspace",
    "commands_file": "/existing/private/commands.json",
    "repository": "",
    "base_commit": "",
    "worktrees": "",
    "model_deployments_file": "",
    "external_config_file": ""
  }]
}
```

外层4键，每行9键全部明确提供；可选值使用空字符串。1–128槽位、1–256不重复租户／项目范围，沿原标识规则。`commands_file`空值生成空命令表，便于先准备目录，不自动开启发现。其余引用非空时必须是目标操作系统绝对路径；workspace／repository／worktrees是现有目录。repository和base_commit同时提供或同时为空，提交固定为40／64位十六进制，不运行Git来猜基线。

在目标机器读取已有commands／model_deployments／external文件，复用原解码器及最终 `DecodePoolConfig`。读取普通文件，拒绝链接与路径别名；输入原件不改写，不读环境变量值、系统凭据库或网络。累计实际引用文件不超过2 MiB，最终配置含换行不超过原2 MiB上限；超限拒绝，不截断。最终 `schema_version=1`，不添加重复control_url或其他授权来源；省略原默认轮询参数。

成功只输出6键：`protocol=suma-pool-config-preparation-v1`、`pool_id`、`runner_id`、`scope_count`、`slots`、`configuration_sha256`（最终写入字节SHA-256）。报告和错误不回显私有命令、模型、路径、环境值或完整输入。未运行任何程序、探针、模型、HTTP、Worker或Grant写入。静态通过不能替代真实运行时目录／基线、当前机器身份、Grant与工具资格检查。

## 简洁工作台

原Pool节点行增加“准备项目配置”，绑定准确当前Pool及有效节点。槽位默认1且不超过当前节点上限；Shell、新文件路径与项目行直接编辑，仓库、固定基线、worktrees及额外私有文件按需展开。可以从当前已加载、准确owner／Pool的approved Grant公开名称和范围中选择项目，自动填入标识；明确只代表已加载项，保留手工标识入口，不调用消费项目API或把历史Grant当作未来授权。支持添加／删除项目行，键盘焦点保持在剩余行或添加入口，390宽度可完整操作；中文／英文所有状态成对维护。

表单校验绝对路径的目标Shell格式、唯一范围、基线、槽位和64 KiB输入边界。浏览器不声称文件存在；复制的明确准备命令将JSON通过经过验证的Shell引用传至标准输入。没有自动执行、自动启动、自动工具发现或默默添加allow-exec。只生成准备命令；后续检查和连接按原指南明确执行。配置变化需新文件及新的运行状态目录，不能删除旧Journal或绕过未决执行核对。

## T210：项目目标切换

本行实际tenant_id／project_id组合变化时，重建本行私有配置：清空workspace、commands_file、repository、base_commit、worktrees、model_deployments_file、external_config_file，保留新的范围组合并立即失效旧准备命令。不得把项目A的本机执行输入静默带到项目B。准确同一组合的重选、仅切换到手工输入而组合未变时保留本行输入；其他行不受影响。适用于授权选择器和手工标识修改，不新增凭据或配置权威。界面提示需要重新选择目标机器文件。仅复验受影响交互，原Go／CLI／静态文件写入证据继续保留。

## 分工与有限验收

Runner代理拥有新准备模块、CLI及main最小互斥接线；Web代理拥有Panel／纯模型／局部样式／双语及PoolWorkspace入口；控制代理拥有独立浏览器→真实CLI集成夹具并独立审查；主代理拥有合同、集成、文档、证据与Git。无共享文件跨写。

定向验证闭形、限额、引用安全、拒绝覆盖、模式互斥、Shell注入与scope切换。一次真实编译CLI接生产表单生成的PowerShell命令，写入隔离双项目配置并核对同字节摘要及再次拒绝覆盖；中英代表视图覆盖窄屏与键盘。沿用不变的Pool调度、配对、库存及独立测试证据，不重复厂商／模型／全量测试。Windows实际、合成视图、其他OS／物理多机／生产分别记录。
