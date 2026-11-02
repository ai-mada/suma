# Qoder 准备卡与原接入交接

版本1.0.0 · T-QODER-TOOL-PREPARATION-221。状态：实现，实际验证见[阶段报告](../quality/qoder-tool-preparation-verification.md)。[领域合同](../../contracts/qoder-tool-preparation.md)和[用户步骤](../product/qoder-tool-preparation.md)使用相同术语。

## 信息与操作

模型和可选凭据名称在前，本机信息为一个清晰分组，避免多层向导。Shell和新文件路径均须明确填写，不从浏览器系统推断远程机器。复制命令、采用路径是两个明确操作；命令可预览，私有五键和手工固定ACP JSON按需展开。

所有输入、按钮与折叠标题至少44px；窄屏单列，长命令和路径可换行，标签关联输入，焦点保持可见。未完成、锁定、复制中、复制失败及复制完成分别使用中英文文案；复制完成只说明复制，不显示“已连接”或“已生成”。新语言继续沿统一词条目录扩展。

## 分层和生命周期

- 纯模型复用现有文件路径、输出JSON、凭据名称和Shell字面量校验，生成严格五键私有输入及三键输出草稿。
- 卡片绑定当前API caller、账号／组织、字段revision、锁定与卸载代次；旧剪贴板回应不能影响新输入，不自动采用路径。
- 父指南校验当前source、准确工具、epoch、navigationNonce及selectionNonce。普通只采用三字段，共享保本机引用，不上传文件或写Pool配置。
- 原Go ACP准备器明确区分Cursor包与Qoder原生文件，复用严格JSON解码、原CommandSpec、摘要pin及独占writer；准备不启动版本／ACP／模型进程。

专用表单和组合器的同级React key按组件命名，保留原sourceKey和单调epoch，切换项目／账号时可靠卸载旧表单。恢复不能靠放宽nonce、抹去旧失败或自动重填认证来完成。

## 验证范围

聚焦私有输入闭形、Shell引用、原生文件／来源变化、新建拒覆盖、旧回应隔离和普通／共享采用；复用已通过的未变化检查，不重复运行完整平台或供应商套件。真实机器执行、原生宿主退出和代表中英文界面分别登记，夹具不冒充Qoder厂商。
