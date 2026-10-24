# Native display language / 原生显示语言

The runtime supports `zh-CN` and `en-US` for its own diagnostics. `RuntimeConfig.locale` selects a display projection for one run, captured before asynchronous work. It never changes the model system/user messages, provider request body, tool authorization, canonical tool IDs, invocation operation IDs, usage facts or model/tool output.

原生运行时的自有诊断支持 `zh-CN` 和 `en-US`。`RuntimeConfig.locale` 在每次运行入口固定显示语言，不使用可变的进程全局语言。模型消息、提供方请求正文、工具权限、工具标识、调用操作标识、用量事实及模型／工具输出保持原样。

For SDK calls, an omitted or `undefined` locale uses `zh-CN`. Explicit empty, invalid or unsupported locale values reject with `NativeLocaleError` (`code: invalid_locale`, `display.message_key: native.config.invalid_locale`) before any event, model/control request, catalog filesystem access or probe process. The error uses a fixed message without echoing the invalid value. Supported aliases are normalized without mutating caller objects.

SDK 未传语言或传 `undefined` 时使用 `zh-CN`；显式空值、无效或未支持的语言在任何事件、模型／控制服务请求、目录文件访问或探测进程之前，以 `NativeLocaleError` 拒绝（`code: invalid_locale`、`display.message_key: native.config.invalid_locale`）。固定错误信息不回显无效值，已支持别名的规范化不修改调用方对象。

An error event retains its original `type`, `code`, `message`, `request_id` and other protocol facts. Optional `display` contains `message_key` (`native.*`), declared `message_params`, normalized `locale` and `localized_message`. The original English `message` remains run evidence. CLI `invalid_json` and `busy` events retain their previous absence of `message`; their human explanation exists only in `display`. Unknown provider/OS text is never matched or split into translation fragments.

错误事件保留原有 `type`、`code`、`message`、`request_id` 等协议事实；可选的 `display` 提供稳定 `native.*` 键、已声明类型的参数、规范语言及翻译文本。原始英文 `message` 是运行证据，不能随显示语言修改。CLI 的 `invalid_json` 和 `busy` 继续不含原先缺失的 `message`，人类说明只在 `display` 内。外部提供方或操作系统原文不会经过近似匹配或句子切分。

Both native CLIs accept `--language VALUE` or `--language=VALUE`. Selection order is explicit flag, `SUMA_LANGUAGE`, the first nonempty `LC_ALL` / `LC_MESSAGES` / `LANG`, then `zh-CN`. BCP 47 normalization uses Node `Intl.Locale`; supported aliases and underscores are accepted, Traditional Chinese and non-Latin English scripts are not labeled as supported translations. Environment encodings/modifiers are stripped; `C`/`POSIX` stay neutral. Invalid, empty or repeated explicit language flags exit 2 before any model/control request or adapter probe.

两个原生 CLI 均支持 `--language VALUE` 或 `--language=VALUE`，优先级为显式参数、`SUMA_LANGUAGE`、第一个非空的 `LC_ALL`／`LC_MESSAGES`／`LANG`、`zh-CN`。Node `Intl.Locale` 解析已支持的 BCP 47 别名与下划线写法，繁体中文及非拉丁文字英文不会冒充已有翻译。环境编码及修饰后缀被移除，`C`／`POSIX` 保持中性。无效、空或重复语言参数在任何模型／控制服务请求及适配器探测之前以 2 退出。

`--help` and `--version` are read-only. Runtime version bytes remain `suma-agent-runtime 0.1.0`. Help is human text only in explicit help mode. Running mode writes JSONL to stdout without human banners; usage errors go to stderr. The adapter catalog keeps its original `reason`, version, installation, verified operations and `execution_enabled:false`; translated context is additive `display`. A foreign OS failure keeps its original reason and gets generic translated context only.

`--help` 与 `--version` 为只读，运行时版本标识仍是 `suma-agent-runtime 0.1.0`。只有显式帮助模式在标准输出提供人类说明；运行模式的标准输出仅含 JSONL，用法错误写入标准错误。适配器目录保留原始 `reason`、版本、安装及已验证能力事实与 `execution_enabled:false`，翻译只增加 `display`。外部操作系统失败保留原始原因，仅附通用已翻译上下文。

This is an SDK/CLI display contract. The existing Go Runner stream parser selects public fields and does **not** propagate `display` into DAG/HTTP/UI events. This package alone does not claim downstream UI integration, production model access, adapter execution support, full platform localization, or P0–P7 completion. Tests use loopback HTTP fixtures and local child processes, without vendor accounts or paid models.

此处只实现 SDK／CLI 显示合同。现有 Go Runner 流解析器选择公开字段，**不会**把 `display` 传到 DAG／HTTP／UI 事件。本包不声明下游界面已接入、真实生产模型已验收、适配器已支持执行、全平台国际化或 P0–P7 已完成。测试仅使用本机回环 HTTP 夹具及本地子进程，不调用厂商账户或付费模型。
