# 多项目本机准备诊断与表单定位

版本1.0.0；阶段T-POOL-PREPARATION-DIAGNOSTICS-211，限定实现与验证通过。原31项／P0–P7目标保持进行中。关联：[原准备合同](pool-config-preparation.md)、[阶段待办](../docs/delivery/tool-management-next.md)。

## 业务边界

复用原Pool配置、允许列表解码、私有引用检查、根目录锚定和独占新文件写入。成功stdout的六字段协议、准备stdin的四字段／九字段行、派发和授权均不变。合法准备模式失败新增安全类型化诊断；CLI解析和模式互斥错误保持原行为。不读取凭据或环境值，不启动工具／Worker，不新增控制面接口、数据库或第二份健康状态。

## 唯一失败协议

stderr输出单行JSON，exit1。正常stdout可用且原准备函数失败时stdout为空；stdout在成功文件写入后发送失败可能已部分输出，必须使用output_unconfirmed引导核对原文件，不能声称未写入或自动换文件重试。

七键全部提供，闭形且不允许重复键：

```json
{
  "protocol": "suma-pool-config-diagnostic-v1",
  "code": "source_invalid",
  "scope_index": 2,
  "field": "commands_file",
  "input_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "output_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "message": "Fixed localized advice without private values"
}
```

| code | scope_index | field |
|---|---|---|
| input_invalid | 0 | input |
| output_invalid／output_unavailable／output_exists／output_unconfirmed | 0 | output |
| directory_unavailable | 1–256 | workspace／repository／worktrees |
| workspace_conflict | 1–256 | workspace |
| source_unavailable／source_invalid | 1–256 | commands_file／model_deployments_file／external_config_file |
| inputs_changed／configuration_invalid／cancelled | 0 | input |

scope_index为输入清单中从1开始的行，不重新排序。诊断只能使用固定code、字段、行号及固定双语指引；不保存或输出原错误链、路径、命令、环境、模型参数、凭据或输入正文。message由CLI固定字典按语言生成；浏览器不信任或渲染报告内message，使用自身语言目录。

input_sha256固定为SHA-256(bytes.TrimSpace(完整原始stdin字节))；读取错误或超过原64 KiB上限时为空，不计算部分数据或假称完整流摘要。不使用Go重新JSON编码，避免HTML／Unicode转义差异；实际生成的紧凑JSON与浏览器JSON.stringify相同，PowerShell附加CRLF由native TrimSpace去除。output_sha256是CLI收到的目标路径原字符串UTF-8摘要，不Abs／Clean／大小写折叠。摘要只作输入关联，不证明机器来源、当前文件版本、授权或执行成功。

## 表单交互与隔离

复制准备命令后，按需展开“准备失败？”，粘贴单条诊断并明确点击定位。最大4 KiB UTF-8，message最多1024 UTF-8字节；拒绝未知／缺失／null／重复键、未知协议、code／field／scope组合、非法摘要与不可用行号。严格复用原唯一JSON成员检查，不新增宽松解析或多摘要备用。

非空input摘要必须匹配当前四字段／九字段紧凑JSON，同时output摘要匹配当前目标原字符串；有行号的诊断必须完整匹配，才展开本行高级选项并聚焦准确字段。input摘要为空只允许global诊断，仍核对output摘要，明确标为仅目标关联，不能定位某个项目或声称当前完整输入已检查。global input显示静态指引，不伪造可编辑字段。

导入的是历史报告，界面明确不是实时检查。定位动作不改写任何配置值、不读取文件、不修复或覆盖本机文件。用户修改输入、项目、输出，或账户／组织／Pool／节点／API来源／授权观察变化时立即失效诊断；异步摘要回调绑定原revision／来源，不得复活旧报告。原T210项目切换清空七字段规则保持。

## 模块与验收

Runner拥有类型化诊断、原准备上下文和CLI双语错误；Web拥有闭形解析／唯一JSON词法复用、摘要核对、折叠报告与字段定位；控制代理拥有隔离生产Panel→真实CLI失败报告→表单定位→修正本机夹具后的同命令成功旅程及独立审查；主集成拥有合同、文档、统一检查和Git。

只运行新增及受影响原检查：错误脱敏／准确字段／未知写入，闭形／重复键／摘要／过期来源和定位。一次CLI构建、一次隔离UI构建、一次实际旅程：第二项目命令JSON错误返回安全报告，表单匹配并定位，本机明确修正后相同命令生成新配置。无厂商／模型／业务任务或重复原全套测试；真实文件I/O、合成公开元数据／剪贴板和正式授权分别记录。成功写入、库存、启用、派发与质量仍是独立事实。
