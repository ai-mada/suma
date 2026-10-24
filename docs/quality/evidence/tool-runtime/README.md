# Runtime 与 Native 公开实测证据

状态：T-P4-004 域内证据归档，2026-10-24。范围以[专项报告](../../tool-runtime-verification.md)为准；完整平台与正式交付由[总验证入口](../../verification.md)和[当前运行包](../../../deployment/tool-runtime-bundles.md)分别记录。本目录归档时没有重跑测试或修改生产源码。

| 切面 | 原始证据 | 适用范围 |
|---|---|---|
| Runner 最终域内回归 | [111个通过事件](runtime-suite-final.jsonl)、[原始收据](runtime-verification.json)、[39文件快照](runtime-source-start.json)、[退出码](runtime-exit.json)、[vet输出](runtime-vet-final.txt) | Windows本机111 pass、0 fail、0 skip，含子测试和helper；当时39源文件一致，原生CLI当时5项 |
| 早期原生CLI回归 | [5项测试](native-cli-final.txt)、[类型](native-typecheck-final.txt)、[原失败](native-cli-red.txt) | 独立Agent凭证及version/help；不代表后续别名源码已由这5项验收 |
| 后续TS别名补充 | [原始补充收据](native-alias-verification.json)、[RED](native-alias-red.txt)、[GREEN](native-alias-green.txt)、[类型](native-alias-typecheck.txt)、[构建](native-alias-build.txt)、[version](native-alias-cli-version.txt)、[help](native-alias-cli-help.txt) | 两个TS文件变化，原生8项通过；33个Go文件保持原字节，未覆盖旧收据 |
| 原生CLI→真实控制面 | [三用例收据](native-live-scope.json)、[原始事件](native-live-scope.stdout.jsonl)、[复现辅助程序](native-live-scope.py) | 正确Agent读取真实3个Task ID；Runner凭证及外项目参数实际403且模型fixture零请求；模型为本机HTTP fixture |
| 会话、终态与preset | [顺序／身份RED](protocol-order-and-preset-red.jsonl)、[GREEN](protocol-order-and-preset-green.jsonl)、[缺计数RED](codex-required-usage-red.jsonl)、[GREEN](codex-required-usage-green.txt) | 拒绝外来／缺失身份、缺开始、终态后活动、prompt/resume/cwd/config覆盖及缺失用量被补零 |
| 本机配置与进程 | [库存503恢复](inventory-outage-green.jsonl)、[隔离配置RED](isolation-inventory-red.txt)、[入口字节RED](native-entry-drift-red.txt)、[GREEN](native-entry-drift-green.txt)、[严格配置RED](strict-command-config-red.txt) | 库存故障保持真实进程与心跳；同路径／同版本入口漂移；明确拒绝非法配置。严格配置RED最初是新解码API尚不存在的编译失败 |
| 凭证与身份修复 | [Runner环境RED](agent-environment-red.txt)、[有界会话RED](native-session-id-red-confirmed.txt)、[引号凭据RED](quoted-credential-red.txt) | 单测用固定假凭据；真实系统秘密未归档，最终修复包含在111项切面 |
| 历史构建／夹具 | [最初API未接线](runtime-red.txt)、[库存API未接线](inventory-red.txt)、[旧夹具回归失败](runtime-suite-attempt1.jsonl)、[旧只读探测](runtime-describe-tools.json)、[执行禁用探测](runtime-describe-disabled.json) | 作为历史失败／探测保留；不能替代最终111项或作为当前工具注册manifest |

[归档映射](archive-map.json)逐项绑定35份原件与副本摘要；[源码切面核对](source-cut-verification.json)解释原39文件、后续两份TS变化、当前33份Go及原生构建的关系。[敏感检查](sensitivity-review.json)对照当前私有上下文及高危模式，没有发现真实凭证；原单测中的静态fixture bearer字面量保持原字节并单独标识。

[控制程序来源说明](control-provenance.json)必须与原生三用例一起阅读：这些用例发生在`1e61a39a…`候选控制程序，不是在最终`c06d3553…`交付字节。原生原收据没有控制进程摘要；候选身份来自主集成运行记录。相同116项运行包输入不能将原测试改称在最终字节上执行。[完整摘要清单](files-sha256.json)覆盖本目录除清单自身以外的文件。

没有归档私有context、真实令牌、命令配置、数据库、外部登录文件、进程stdout/stderr日志或二进制。辅助程序读取调用者提供的本机私有上下文，仓库不提供任何凭证；模型端始终是明确标注的loopback fixture，没有真实模型／厂商验收结论。
