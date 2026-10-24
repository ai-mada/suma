# 已安装 Codex 只读探测补充

T-P4-004，2026-10-24。本补充使用**本阶段初次`20261024T045219750Z-715ba2c8`交付Runner**，SHA为`12c8681ec9d7e1ba325252e264e5d7aaa7d73d9c053cc3d6c3466292b44f56cb`；不把它称为后续post-review阶段最终运行包，也没有为后续包重复CLI探测。

- [非秘密describe原文](installed-codex-describe.json)：通过`--describe-tools`探测一个合法只读Codex profile。
- [原始探测回执](installed-codex-verification.original.json)：保留当时原字节；其“delivered Runner”含义由本补充明确限定为初次045219切面。
- [切面与来源补充](installed-codex-probe.json)：版本、指纹、安装路径、输入限制、归档映射及旧41文件未变化。
- [复现辅助程序](installed-codex-probe.py)：只定位已安装的官方App native exe，不执行npm shell shim，不安装软件，不读取privatecontext或使用控制面token。
- [完整摘要](files-sha256.json)：覆盖本目录除自身外的文件。

实测版本为`codex-cli 0.164.0-alpha.11.1`；可用含义限于本地安装、`--version`和`exec --help`匹配。配置的task preset仅用于校验，未运行`exec --json`任务。没有调用模型、登录、任务或控制面API，没有验证模型认证／健康，也没有OS级网络捕获。没有归档命令配置、原始进程stderr、登录文件或二进制。

本目录独立于[既有Runtime档案](../tool-runtime/README.md)，原41份文件与原摘要清单保持不变。完整验收范围见[专项报告](../../tool-runtime-verification.md)。
