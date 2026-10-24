# 完整需求链接点击盒证据

本目录保存 T-P4-004 最后一次需求点击区域修复的独立原始记录。前代 189、194、197 及 CGLjC2wJ 切面不覆盖。说明见[专项报告](../../tool-ui-hitbox-verification.md)。

- [RED](ui-requirement-hitbox-red.txt)与[GREEN](ui-requirement-hitbox-green.txt)：新增 1 项 DOM 结构回归从失败到通过；原 3 项实际 React 消费者导航继续通过。
- [web198](ui-web-hitbox-final.txt)、[类型](ui-hitbox-typecheck-final.txt)、[格式](ui-hitbox-format-check.txt)、[标准空白](ui-hitbox-whitespace-check.txt)：最终应用检查。
- [源码快照摘要](ui-hitbox-source-before.json)与[最终收据](ui-hitbox-verification.json)：22 份 UI／测试加 2 份依赖输入的不可混同切面。
- [文档检查](ui-hitbox-docs-check.txt)、[归档来源与字节摘要](archive-map.json)、[归档核对](archive-verification.json)：文件完整性和共享文档检查器范围。

结构回归没有伪造 JSDOM 布局或命中坐标。实际鼠标／键盘、浏览器返回、后端和生产构建的验证，由主集成分别记录。本目录不包含凭据、活动数据库或用户私密执行内容。
