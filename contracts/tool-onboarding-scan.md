# 本机工具扫描与登记预检（1.0.0）

状态：T-TOOL-ONBOARDING-176 实施合同。负责人：主集成代理；桌面和 Web 按文件边界并行开发。

## 本机批量扫描

沿用 `sumaDesktop.discoverInstalledTools` 与 `suma:discover-installed-tools`。单项请求、响应保持当前精确合同。新增的批量请求为：

```ts
type LocalToolsScanRequest = {
  action: 'scan'; project_id: string; user_id: string; tenant_id: string;
  locale: 'zh-CN' | 'en-US'; force?: boolean;
};
type LocalToolsScanResult = {
  status: 'observed';
  report: { protocol: 'suma-installed-tools-v1'; items: InstalledToolObservation[] };
  scan: {
    id: string; project_id: string; user_id: string; tenant_id: string;
    observed_at: string; expires_at: string; cached: boolean;
  };
} | { status: 'failed'; code: string };
```

对象为闭合类型，ID 为 32 位小写十六进制；时间为 UTC ISO 字符串。批量请求不能携带 `adapter_id`。结果精确包含现有七个接入项各一次，按目录顺序返回：`codex-jsonl`、`codebuddy-cli`、`dsh-multica`、`claude-code-cli`、`cursor-cli`、`qoder-acp`、`codex-app-server`。两个 Codex 项代表不同协议，不能计作两种已安装产品。

- 复用 Go 已有的 `--discover-installed-tools` 全目录并行探测，不增加派发器、后台服务、数据库或 CLI 别名。
- 每行复用现有校验并移除命令参数；不向渲染进程授予启动权限。未知、缺失或重复接入项拒绝整个结果。
- 桌面仅保留最后一个身份／组织／项目范围的批量结果，单调时间有效期 60 秒；`force: true` 忽略完成缓存，同范围正在运行的请求合并为一次进程。
- 同范围单项请求可等待进行中的批量结果并投影为原单项响应。已完成批量缓存不能替代单项原有的即时核对。
- 每次返回（包括缓存、合并请求）都检查当前窗口、实际会话及项目权限。不同范围不共享结果；关闭窗口清除缓存并停止本次探测。
- 扫描只读取安装事实，不安装、登录、连接、登记、启用、探测模型或创建任务。安装不等于健康、已连接或可派发。

## 简洁界面

桌面工具入口在当前身份和项目有效时自动检查一次，可手动重新检查。每项显示安装事实、协议及具体下一步，选择后进入现有接入流程，不自动替换配置或丢弃草稿。浏览器模式展示现有桌面／执行节点接入指引。异步结果绑定当前范围；原连接未知／忙碌状态仍限制切换。完整支持中文和英文。

## 高级登记预检

采用探测清单时保留原调用者、项目、执行节点及单调有效期。开始新的登记前，过期清单引导重新读取原 GET；名称草稿保留，不静默更换配置。身份变化后不能沿用旧清单。

原登记请求结果未知时，按原请求和操作 ID 核对；新的清单过期不能阻断原请求恢复。手工配置是独立有效入口，允许保存暂停且未验证的配置，不把执行节点暂时离线作为永久禁止手工登记的理由。导入失败不能自动清除已有来源约束。平台原有权限、登记和启用校验仍是正式依据。

## 验收与边界

新增关键行为定向验证；不重复未变 Go、全套历史测试或全部浏览器矩阵。复用上一阶段的控制面二进制，做一次真实批量探测。根集成统一一次类型与中英文词条检查。记录组件验证、真实 CLI 观察和未完成的真实 GUI／跨平台验收，分别披露。
