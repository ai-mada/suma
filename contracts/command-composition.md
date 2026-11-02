# 已有命令配置安全组合

版本1.0.0；T-COMMAND-COMPOSITION-215；负责人：主集成／Runner／工作台。完整31项／P0–P7继续。

## 合同

新增本机离线模式 `suma-runner --compose-commands DEST`，从UTF-8标准输入读取闭形JSON `{"files":["绝对源文件1","绝对源文件2"]}`。2–16项，输入≤64KiB；每源≤2MiB，总读取≤2MiB；准确源普通文件、已有不含链接父目录，按原本机路径规范检查。目标必须为绝对路径，使用原新文件writer／独占创建；不覆盖、不创建父目录，不读取凭据、启动进程、连接控制面、探测或领取任务。输入含任何其他模式／参数即拒绝，language除外。未知参数等公共解析错误仍由原CLI解析器统一处理。

各源严格原JSON对象解码、拒绝重复键／未知字段／空对象；允许一个源的native_workspace.checks引用另一个源的命令，只在合成后调用原DecodeCommands／ValidateToolConfiguration。重名一律拒绝，不做覆盖／去重／自动重命名／自动增加检查或执行能力。合成后≤128项／≤2MiB；原NativeWorkspace协议限制不变，Codex／Claude／ACP不强加该策略。源、父目录在写前复核；取消和变化拒绝。原始源全部保持。

成功JSON协议 `suma-command-composition-v1`：source_count、command_count、configuration_sha256（包括最终LF字节）；不回显路径／命令／环境内容。进入该模式后的错误结构协议 `suma-command-composition-diagnostic-v1`：code、source_index（1起；非源错误0）、message（CLI按language中英文），固定原因不透传原路径、命令ID或外部error。原因包括mode_conflict、input_invalid、output_invalid、output_exists、source_unavailable、source_invalid、duplicate_command、configuration_invalid、inputs_changed、cancelled、output_unconfirmed。

## 简洁交互

原ToolSetupGuide按需展开组合面板；选择目标Shell、填写已有文件引用（两行起、增删≤16）、全新JSON输出（以.json结尾，与原连接消费者一致），实时说明字段问题、生成准确命令并复制。只处理文件引用，不上传源JSON、不请求网络、不替用户执行。来源／账户／组织／项目／节点变化后清除该面板草稿，旧异步复制不显示到新来源；disabled时不得复制或编辑。双语、键盘、焦点／44像素触达和窄屏。组合面板可明确将输出路径及Shell采用到原Runner指南，保留原机器、身份、工作区及工具选择；只采用路径，不证明已创建文件。原同一有序configurationRequest通道只应用一次，手工后改不覆盖，工具／账户／项目／节点变化不复活旧意图。后续沿原Runner指南检测、登记、启用；准备回执不冒充库存或派发许可。原开发内部checks不等于不同Worker同提交独测。

## 验收

必须验证跨源引用成功、重复ID／重复JSON键／悬空引用拒绝、源不变和目标不覆盖、模式互斥先于stdin／凭据访问；真实编译CLI执行一组本机文件组合与拒绝覆盖，界面代表CN／EN。复用原质量合同，不重复执行真实模型或全平台测试。
