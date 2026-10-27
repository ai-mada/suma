# 执行器传输能力与当前预览交付验证

状态：T-P4-039／1.0.0，2026-10-27，限定本地功能阶段通过。负责人：主集成与两个并行领域代理。来源：[完整实施计划](../delivery/implementation-plan.md)、[合同](../../contracts/workspace-transport-capabilities.md)。[机器摘要](workspace-capability-summary.json)记录准确证据路径与摘要；原始数据库、凭据、截图、二进制和日志仅留忽略缓存。

## 实现与检查

- Control：未声明read的旧执行器不再列目标；只有read时不能保存；write撤回阻止新许可与正文重发，原metadata及准确已发生效果仍能恢复。2个新回归先红后绿0.816秒。最终 `go test ./internal/platform -run 'TestWorkspaceCapability|TestWorkspaceWrite|TestWorkspaceObservation' -count=1` 通过26.684秒，包含两个现有实际出站联合用例，无GUI等待、模型或厂商调用。
- 共享合同和Runner：`go test ./internal/toolcontract ./internal/runner -run TestWorkspaceTransport -count=1` 通过，两项共享声明验证与一组真实本机Node版本/帮助探测；禁用、入口漂移、不可用、普通CLI和只读用途不会广告broker。先前编译/断言失败日志保留，不删除红阶段。
- Web：`vitest run src/tool-model.test.ts --maxWorkers 1 --no-file-parallelism` 27项通过；真实原生公开库存形状曾被旧导入器拒绝，现已修复。`vitest run src/ToolFacts-consumers.test.tsx -t 'remote workspace transport'` 新显示用例通过，其他8项有意未选。最终直接 `tsc --noEmit -p tsconfig.json`、4568对语言键检查通过。错误语言domain位置和生成类型cast的中间失败已纠正，不将早期组合命令的最后exit0当成tsc通过。
- 生成合同 `-check`、更改的合同与NLS包、受影响Go vet通过。Windows五程序有038和039两次必要源码构建；最终039前端仅构建一次，40.87秒。构建中最后一行纯类型cast修正经TypeScript转译证明JS完全相同，没有重建相同UI。Monaco大型lazy chunk警告仍存在，未假称性能目标已通过。

## 主预览实际交付

原035 Control与持有进程的原生句柄均不存在，4324端口实际拒绝连接，故先做WAL感知一致备份，再在隔离克隆演练038迁移。原186张表完整列、行和schema准确保持；只新增 `suma_workspace_observations`、`suma_workspace_writes` 两张空表。实际主数据重复核对迁移结果；没有复制夹具数据库覆盖主库。原入口密钥另存配套备份。

038初次核验因Python默认GBK读取UTF-8网页失败；已启动服务由持有句柄正常停止exit0，修正明确UTF-8后前向恢复。切换039时所有包含DDL的迁移源码与演练038准确相同，沿用迁移证据，不重复克隆整套旅程。039切换前再次一致备份。已退出旧进程的image查询发生短暂失败，原持有会话exit0、准确正常停止回执及随后原生句柄缺失共同证明终结，才恢复切换；没有以观察超时推断停机。

最终Control和Web为039同切面，全部136个本地Web资产固定摘要，HTTP入口及直接模块字节匹配。启动时188张表所有完整事实和schema保持。既有会话实际读取后，184张非锁表完整事实保持；4张锁表34个身份保持，仅sequence增加（credential30行、governance2行、identity1行各+36；全局guard1行+405，包含正常后台协调）。18authority／0Grant／18session、1failed／2succeeded和原已验收任务v3保持。

实际GUI核对原项目、详情Escape关闭、038工作区来源刷新保持及准确空执行/0保存历史；最终039工具详情展开后显示最近read/write均未声明、当前Pool凭据过期。中英文切换保留展开和原工具名称，相关console错误和警告为0。当前单个浏览器视口的检查不是设备/主题完整矩阵。没有新登录、凭据、运行、模型调用、正式测试验收或Runner启动；两个旧enabled工具配置不据此宣称在线。

## 交付与后续

本地预览地址：`http://pool.localhost:4324/`。当前可执行程序、原生bundle与网页保持精确映射；未改管理员工作区绑定，未将任意目录开放。合同、架构/AI SOP及英文说明见[接入说明](../integrations/workspace-transport-capabilities.md)、[English](../en/workspace-transport-capabilities.md)。只提交必要源码、文档与本摘要；不push。

独立物理机器/TLS、远程二进制证明、完整IDE/PTY/LSP/DAP、外部CLI/专家团队模型选择、完整费率与模型授权、安装/移动端/渠道、SSO/SaaS/分布式数据库与规模等仍继续实施。完整P0–P7及需求1–22保持活动状态。
