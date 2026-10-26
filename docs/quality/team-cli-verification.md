# 团队多工具执行验证

状态：T-P4-021，本地功能、实际进程与最终中英界面／预览前向更新已验证。完整平台仍实施中。关联[PRD](../product/team-cli-runtime.md)、[执行合同](../../contracts/team-cli-runtime.md)、[架构与 AI SOP](../integrations/team-cli-runtime.md)。

## 实现与实际检查

- Harness/dsh 与 CodeBuddy 进入原 Go DAG、项目／共享池授权、持久进度、隔离 Git 工作树及独立测试路径，没有第二派发中心。DSH 使用实际 Multica profile v1，CodeBuddy 使用固定 headless stream-json。模型文本或 exit 0 不能替代唯一整体结果及实际进程停止。
- 原生进程夹具四场景：DSH 正常结果→EOF、无结果退出、同请求取消与 CodeBuddy 完成；真正启动子进程、修改私有工作树、生成候选，并核对原工作区保持。首轮 1 顶层／4 子例通过；无结果时序断言调整后仅复验该子例；DSH probe stderr 分离后仅复验正常子例。回执分别为 `045604180Z`、`045703718Z`、`050709337Z`，不累计重叠检查。
- Node 入口配置与真实只读 version/help/probe：2 顶层／10 通过事件，无失败或跳过（`053832595Z`）。绝对 .js/.mjs/.cjs 入口、非链接普通文件与大小约束；Node eval、相对／缺失路径和权限参数覆盖拒绝。摘要绑定 Node 可执行字节、入口字节与完整命令配置，不声称覆盖全部导入模块及厂商私有配置。
- 新 Node 实际执行组：1 顶层／5 子例、6 通过事件，无失败或跳过（`053952182Z`）。真实 Node 执行 Harness .js/.mjs 与 CodeBuddy .cjs，三个独立候选的父提交／内容／clean 均核对；两个 adapter 在库存后入口被改动，执行在启动前拒绝，无执行 marker 或候选。原脏工作区与 HEAD 保持。
- decoder、固定参数、环境隔离、任务派发与本地化仅做相关定向；Runner 平台凭据不传厂商进程。Go 1.26.10、既有标准库／SDK，不增加依赖。原生 Agent 的固定 bundle 沿用 020，不重复构建。
- 前端工具合同 24 项、CLI catalog 新 11 项及已有相关 2 项通过，类型／3229 成对中英文词条检查与隔离构建通过。基线实际中英界面已检查折叠、选择保留、复制反馈和键盘操作。最终 Node 界面实际核对空／相对路径禁止复制、绝对 mjs 预览、语言切换原 JSON 不变、切工具无过期反馈、原生参数恢复及 Codex 隐藏额外字段，刷新仍默认折叠，1366 浅色无横向溢出／告警；记录见机器摘要；复制 Promise 反馈不当作操作系统剪贴板读回验证。

所有原始命令、退出、日志、构建清单、截图及数据库仅在忽略目录 `.cache/e2e/team-cli-stage-021/` 和 `.cache/mcp019-deps/`。Go 回执前缀均为 `20261026T`；早期 decoder RED／中间并行编译失败保留，最终相应定向 GREEN，不隐去原失败。

## 交付边界

主预览已更新至 021 runtime-revision2：control 229668、holder 223492，地址仍为 `http://pool.localhost:4324/`。最终浏览器后只读核对：147 表结构、143 表完整内容保持，另 4 个协调表同主键且只推进 sequence；18 authority、0 Grant、全部既有 DAG 记录与 failed 1／succeeded 2 保持，integrity 为 ok。原账号通过临时页直接读取原订单项目和新 Node 入口，未登录／签发凭据；用户原标签页未操作。旧 021 control 与 holder 均正常退出 0，隔离 UI 的 control／holder 也均退出 0。

新正常实例启动写入边界在 Popen 前持久记录；备份与旧程序／网页保留，未恢复数据库。当前长期 holder 仅按本轮 `primary-upgrade-revision2/stop.request` 协议停止。最终事实与路径见[精简摘要](team-cli-summary.json)。

未重复全 Go、全浏览器矩阵或未变业务旅程。当前只有 Windows 本机与代表性界面证据；不能扩大为 macOS/Linux、全部尺寸、完整沙箱或分布式容量验收。新协议采用本地确定性进程夹具，当前 PATH 缺少 dsh／CodeBuddy；没有调用这些厂商模型、自动安装 profile、验证 WorkBuddy 客户端或恢复厂商会话。CodeBuddy 与 WorkBuddy 分别记录。

已完成的是本页列明的适配、进程及配置体验。多团队步骤交接、专家组智能决策、受管目标分支合并、知识／记忆／技能自动萃取发布、SSO／A2A、分布式数据库及规模、跨平台安装、SaaS 与完整 P0–P7 均继续实施，不能以工具目录或协议夹具宣称完整平台验收。
