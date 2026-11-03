# Multica参考与工具接入采用记录

T226重新锁定 [Multica 2115a513](https://github.com/multica-ai/multica/tree/2115a513a375bfe2910b76725b617504faee4b25)：核验实际 daemon 探测源码与 LICENSE，保留安装发现／协议资格分离；许可证仍含 Part I 附加条件，本阶段未复制产品代码或素材。新增 ZCode 发现记入待实现，不能宣称平台已具备。真实 Codex 接入和诊断见[当前报告](../quality/real-cli-onboarding-verification.md)。

速码1.0.0 · T-MANAGED-TOOL-PREPARATION-224 · 负责人：主集成代理。

本轮通过官方远端读取主分支提交 `24b54bc09cebc4234af27ddafabd78b9c49626ad`，再按该提交通过GitHub连接器取得并验证四份Git blob；不是把旧本地HEAD当作最新。本地 `D:/codes/github/multica` 保持 `e8b6a97643ccc35efbcf2a4675c8d088a288ed6e`，未修改；四份对应文件仅CRLF差异、规范化后内容一致。

| 参考来源 | 采用方式与速码实现 |
|---|---|
| [agents_probe.go](https://github.com/multica-ai/multica/blob/24b54bc09cebc4234af27ddafabd78b9c49626ad/server/internal/daemon/agents_probe.go) | 借鉴分层探测及明确失败；速码沿原私有扫描回执核对准确安装、版本和完整依赖，拒绝静默换同名程序 |
| [agents_refresh.go](https://github.com/multica-ai/multica/blob/24b54bc09cebc4234af27ddafabd78b9c49626ad/server/internal/daemon/agents_refresh.go) | 参考本机维护与能力刷新边界；速码沿单维护owner、持久Journal、不可变命令代次及原库存ACK，独立保持业务执行资格 |
| [commands.ts](https://github.com/multica-ai/multica/blob/24b54bc09cebc4234af27ddafabd78b9c49626ad/packages/core/runtimes/commands.ts) | 参考明确运行方式；速码保留固定adapter、能力声明和准确项目／机器范围，不把目录存在当作运行成功 |
| [LICENSE](https://github.com/multica-ai/multica/blob/24b54bc09cebc4234af27ddafabd78b9c49626ad/LICENSE) | 包含额外托管／商业嵌入、标识与署名条件，不能简写为纯Apache-2.0。此阶段为原创合同与实现，未复制产品／前端代码 |

完整来源清单SHA256 `3dfd48c000449f006530e80104db51b4b3a45f2a9c5ca1d2f8b9942bc4247738`；只读取得记录SHA256 `a8a3ec1dd734a3cfcb3bc0968496d13f0f0ebc513cd5096cbb993b00b19c01e6`。原始文件留在忽略目录供审计，不提交参考源码、依赖或临时下载。不运行参考仓库脚本，不以参考设计代替速码验收。[当前实现与证据](../quality/managed-tool-preparation-verification.md)仍明确真实厂商、其他工具、分布式和生产的剩余范围。

## T225当前接入扩展

沿上述已锁定24b54参考和现有速码实现，当前六类自动工厂共用维护／来源／库存合同；新增四CLI复用速码原固定builder、版本／帮助／协议检查和rooted安装树核，不复制Multica产品代码或运行其脚本。两SDK与Codex App Server自动资源／provider接入继续，未把本地其他参考文件称为新核实的远端内容。[本次证据](../quality/managed-cli-preparation-verification.md)将自有metadata程序与真实厂商分开。
