# Multica Claude 协作采用记录

状态：限定采用与验证通过；T-CLAUDE-SDK-192；suma1.0.0。负责人：主集成代理。关联[执行合同](../../contracts/claude-sdk.md)、[PRD](../product/claude-sdk.md)与[限定验证](../quality/claude-sdk-verification.md)。

本轮读取本机Multica固定提交`5a2cc891ac338eed1919aa11a1eef5957cd82cec`。其Claude实现采用Go stream-json原生控制协议，而非官方TypeScript SDK；相关自动许可与禁用提问的策略不用于速码。源码、许可与路径哈希保留在忽略的本阶段来源清单，未复制其Claude执行代码。

速码采用团队统一任务、工具接入及执行交互的产品思路，复用自身已实现的CLI、Runner、工作树、权限、问答、Journal和独立质量门。新增原生协议交给[官方SDK0.3.314](https://github.com/anthropics/claude-agent-sdk-typescript/releases/tag/v0.3.314)，该SDK对应CLI2.1.314；回调与用户输入参照[官方规范](https://code.claude.com/docs/en/agent-sdk/user-input)。准确版本最终以已校验包原件、类型、闭包和本轮证据为准，在线文档不能自动扩大已审支持。

SDK作为精确依赖使用，按包原件记录其许可与商业条款链接，不把它标成MIT／Apache源码。Multica许可及附加条件单独记录，不声称所有参考实现都可无限制复制或用于SaaS。本阶段没有复制商业部分、登录厂商账户或调用商业模型。

支持边界由原合同及真实验证决定：SDK初始化确认是配置事实；本地回调写入不是供应方ACK；同提交独测和人工验收不由SDK success替代。全部原31／P0–P7及多项目、多团队、分布式、记忆知识、发布运维和生产目标继续。
