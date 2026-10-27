# 当前身份与授权实施基线

状态：1.0.0初始化版本。身份、用途权限、当前证明、事务协调与撤销是初始结构；不再保留将旧会话转换为可信凭证的迁移计划。

权威入口为[当前授权合同](../../contracts/current-credential-authority.md)、[当前授权架构](../architecture/current-credential-authority.md)、[完整实施计划](implementation-plan.md)与[初始化设计](../architecture/initial-version.md)。源码使用Service.Initialize及各领域Initialize方法，新库建立完整发行证明；无证明凭证拒绝，未声明已有领域schema拒绝，不自动迁移或清空。

阶段复核与旧故障证据保留在[身份专项验证](../quality/current-authority-verification.md)及Git，不能将历史设计缺口或局部通过当作当前全平台完成。SSO、真实分布式存储与生产规模等剩余工作仍见完整计划。
