# 语言与消息合同

状态：已实现基础合同；负责人：Runtime / Native 集成代理；关联阶段：T-P4-006。

## 语言选择

正式支持 `zh-CN`、`en-US`，默认 `zh-CN`。`zh`、`zh-Hans`、简体地区标签归一为 `zh-CN`；`en` 及拉丁文字的英文地区标签归一为 `en-US`。`zh-Hant`、`zh-TW`、`zh-HK` 不冒充已支持的繁体翻译。输入使用 BCP 47 解析，接受本地环境常见的下划线写法；拒绝控制字符和超长值。

纯函数 `Resolve(Preferences)` 的顺序是：显式手动选择 → 已保存的用户偏好 → 组织默认 → 请求 `Accept-Language` → 默认语言。只有实际存在的设置才能参与选择，不能生成虚假的账户或组织配置。本轮 HTTP 适配只使用 `X-Suma-Language` 和 `Accept-Language`；用户及组织语言持久设置尚未实现。

`Accept-Language` 最多 8,192 字节、64 个范围；质量值为 0–1 且最多三位小数。非法、重复范围或控制字符使协商失败并交由默认选择处理；明确 `q=0` 不被通配符重新启用。地区别名归一为已支持目录后，最具体范围优先，再按质量和出现顺序选择。此接口是应用偏好选择器，不发出 HTTP 406。

CLI 顺序为 `--language` → `SUMA_LANGUAGE` → 第一个非空的 `LC_ALL` / `LC_MESSAGES` / `LANG` → `zh-CN`。显式参数不支持或为空时以退出码 2 拒绝；不支持的环境候选可以回退。POSIX 的 `C` / `POSIX` 保持中性。CLI 不改机器、用户环境或服务端设置，业务请求发送规范化的 `Accept-Language`。

桌面主进程在 Electron ready 之后读取 `app.getLocale()`，本进程 `SUMA_LANGUAGE` 可以覆盖。渲染界面另行管理自己的语言偏好；主进程不能假设网页的本地保存已同步到本机或账户。

## 稳定机器字段与消息

`internal/localization` 提供 `Normalize`、`Resolve`、`NegotiateAcceptLanguage`、`EnvironmentLocale`、`Format`、`WithSource`、`Catalog`、`LookupSource` 和 `LookupCode`。没有可变的进程全局当前语言；同一服务可以并发处理不同语言。

消息输出为 `message_key`、可选 `message_params`、`locale`、`message`、`localized`、可选 `fallback`。HTTP 的 `code`、状态、权限、租户／项目标识、资源单位、哈希、版本、幂等标识和请求正文保持稳定。语言不参与业务授权、操作摘要或持久证据。

目录采用 `named-dot-v1` 纯文本模板，只允许 `{{.name}}`，不允许函数、条件、HTML 或任意 Go 模板行为。参数必须与声明逐项一致：

| 类型 | 约束 |
| --- | --- |
| `text` | 非空有效 UTF-8、最多 512 字节，无控制字符 |
| `identifier` | 1–100 个 ASCII 字母、数字及 `_ . : -` |
| `integer` | 明确的非负整数，最大 `2^53-1`；不接受浮点、空值、小数字面量或字符串猜测 |

已知零值完整保留；缺失、额外或非法参数拒绝，并且错误不回显参数值。客户端使用 `Intl` 和明确时区格式化数字与日期，本轮服务端文本中的整数为稳定十进制文本。

未知合法消息键保留原键，以 `unknown_key` 标记安全回退，不输出未经声明的参数。`WithSource` 仅用于原本已经公开的旧诊断：目录缺失或参数无效时保留原文，`localized=false`、`fallback=source_untranslated`。先用 `LookupSource` 匹配维护过的完整原文；只有原文为空时才允许 `LookupCode` 通用映射。禁止用通用 `invalid_input` 替代具体校验原因，禁止前后缀或模糊匹配。

CLI 自有诊断使用独立的稳定 `cli.*` 键和精确本地目录；远端错误保留服务端消息及字段，不重新翻译。终端内的业务状态及用户内容保持原样。Webhook 语言头不改变签名正文、投递标识或恢复语义。

## 覆盖与渐进接入

本轮基础目录覆盖公共鉴权／不存在／服务／来源错误及共享执行池的准入、授权、路由、凭证和具体诊断。CLI 帮助、只读 TUI、自有校验／网络／签名投递诊断已接入；桌面启动、就绪、异常退出和停止提示已接入。桌面原始操作系统异常保留为诊断，不能标成已经翻译。

旧业务模块的所有消息、Runner CLI、本地配置错误、原生智能体所有人类提示、邮件／通知、账户与组织持久偏好仍需分批接入。原始外部工具日志、模型输出、用户文件、终端内容和历史持久证据不翻译。该基础交付不等于全站双语覆盖完成。

实现与验证入口：`internal/localization`、`internal/cli`、`apps/desktop/src/localization.cjs` 及各自测试；HTTP 投影属于控制面集成，不能通过修改存储对象来生成翻译。
