# 项目个人通知偏好：限定验证

T-NOTIFICATION-PREFERENCES-167；速码1.0.0；2026-10-31。负责人：主集成代理。状态：个人项目讨论静音的后端、真实HTTP／SQLite业务与补齐正式样式的独立视觉限定核验完成。完整平台继续交付。依据[合同](../../contracts/project-notification-preferences.md)、[架构与SOP](../architecture/project-notification-preferences.md)、[交互规范](../design/project-notification-preferences.md)；机器记录见[摘要](project-notification-preferences-summary.json)。

## 源码与定向检查

| 范围 | 结果与证据边界 |
| --- | --- |
| Go领域 | 新8个顶层及3个schema子例、原受影响关注投影4个顶层通过，共12个不同顶层。覆盖本人viewer／用户API、撤权与机器拒绝、默认纯读、CAS／no-op／旧回执、回滚、human/API／最终assistant、提及保留、历史及异项目、两SQLite连接串行和写前schema拒绝。 |
| 同源合同及错误 | 新公开合同1项、两错误的精确来源／中英呈现1项通过；共14项不同Go顶层。生成799个声明模型、800个schema、377个路径，仅新增2路径／3DTO，既有375路径及797schema逐项对比均无改动。 |
| Web | 38项不同：model5、组件11、Inbox22；17新／21原。原9个组件RED→GREEN及两个后补组织/API迟到回执场景分别留证；原Inbox测试act警告仅修测试后重跑1项，其余21跳过，不重复计数。 |
| 类型与语言 | 一次完整TSC退出0，最终生产代码未再改动。之后仅新增两项测试，同文件Serena语义诊断为空且自身两项通过；不将该诊断称为第二次完整TSC。7875对中英词条、占位／复数／markup检查通过，生产硬编码0。 |

共52个不同自动化顶层检查，不把3子例、重复单例或以下8个实际检查再次加入该数。后端7源摘要`dd85ee447499d3d4e492874f6568ef9e63b1bed75cba1e59bf179f4ee28e2c7c`；最终9份Web作者清单摘要`d14fbd57b7edfafe8a67bf5274a9cb9be4e03c062cca9ce8df949dc1568fa3be`。主集成者独立核对各文件摘要，独立只读审阅未发现P1/P2。

## 真实本机纵向流程

使用新SQLite、虚构owner/viewer、正常公有handler准备成员／任务／关注。运行`TestNotificationPreferencesActual`和真实Server／HTTP，浏览器挂载真实NotificationInbox、新设置、API／ApiViewProvider与i18n，没有fetch mock；种子准备与owner HTTP发帖不冒充完整GUI。组件origin为独占回环端口，Go使用独立随机回环端口。

8项实际检查通过：默认GET返回v0/false/空时间且不新建偏好／回执／活动；普通消息先产生关注通知；viewer用GUI键盘开关并显式保存v1静音，UI独立GET；后续普通消息不产生关注通知，准确提及仍产生mention且无重复follow；GUI解除静音v2后新普通讨论恢复通知；旧v1原body/key重放返回原回执，另读仍v2/false且不增加新回执；历史／未读、任务／关注依据与英文390刷新保持。

最终SQL与界面共同证明：4条正式human消息、viewer follow2＋mention1／未读3、偏好单行v2/false、任务及关注v1；DAG Run／node／attempt／执行事件均0、供应方／模型／Go外发请求0。运行期只有7个POST：4消息、2次GUI保存、1次原回执核对。没有标已读／归档操作；不是靠收件箱读取时隐藏消息来模拟静音。

唯一Go宿主编译、唯一Vite SDK组件构建、浏览器驱动与外层最终均退出0。此前首次脚本在任何编译／准备数据前，因对空端口主动connect的2秒超时退出1，0phase／0seed／0业务请求、无活子进程。原两helper与44输入门禁封存；仅改为独占bind后释放自有socket，其他42输入逐项不变。新44输入门禁摘要`72dcede66cee96c11da2603f401b8847ecb413b04ba9cc5c46f5b41c7fff92eb`；没有重新编译、重新seed或重跑业务来覆盖早期失败。

首批两图的设置控件可读，但后续精确复核发现夹具仅加载style.css，遗漏正式main.tsx的experience-foundation.css及xyflow全局样式，Inbox间距变量不完整。因此旧图只保留为原业务旅程记录，不作最终视觉验收，没有在产品中补假样式掩盖夹具问题。

独立续验使用原一致快照的已校验副本、正常Server和原cookie当前认证，无Initialize、Engine、新seed或login；仅auth/session、偏好与收件箱三类正式GET各2次，加受保护私有状态GET1次，业务POST0。15张业务／来源／回执／执行表全列全行摘要前后相同，原快照及旧44输入、业务out／binary／原图与清单均不变。新49输入门禁`fd54feb886966328178f8b9c19ffa71c6143c2ee9dc59efed6bdf3d1f584edf9`单独绑定新视觉宿主和main同顺序三份全局CSS。

视觉域另作一次必要Go编译及一次组件SDK构建，两case／两图与各退出均0，未重复52项或原8业务。主集成者独立查看最终中文1366／英文390原图：当前v2/false、3条历史通知；空间令牌16／24、标题18px，桌面通知行纵16／横24px，窄屏各边16px、间距12px，无横溢出，实际focus成立。不外推完整focus-visible、导航、主题或设备矩阵。新11个观察进程正常退出、端口释放、无强停，12安全载荷及独立清单位于`dist/quality/notification-preferences-167/visual/`，摘要`ca4dedf510ba3811563112027ba73b62d67917d93252db9402f2762a83963d1d`。本阶段总计业务／视觉各1次宿主编译与各1次SDK构建，不能把两域叙为一轮从未失败的验收。

两窗口、Electron、代理和Go正常关闭；before/will-quit及退出0分别留证，11个已观察进程均不存在，组件端口与Go随机端口释放，无强停。关闭后留一份私有一致SQLite快照：2863104字节，摘要`0aec237e02768f6f41e50b17d47bb1fba36d0e0aa1da5546d5e5615a05814bcd`；原临时DB／DataDir已删除。14项安全载荷及清单逐项验摘要后导出`dist/quality/notification-preferences-167/`，清单摘要`b3509398682b6280815143052580695c04fb46719861578408d0be842a74cf56`。cookie、密码、正文、原始key、数据库、profile、二进制、bundle及私有日志不进入安全载荷或Git。

## 采用范围与剩余交付

参考Multica固定`e8ae72c9dfc1b4474b61a9087f51b33cbee6a048`的个人通知分组及批量读取思路，独立实现，不复制新版产品源码。此开关仅管理当前项目本人关注讨论；Multica的完整7组通知、系统提示／渠道等仍须后续交付，不标为全部通知偏好已完成。

OceanBase／PostgreSQL真实事务、物理多机、容量、安装包与生产仍待验；Cursor真实执行及其模型／预算／代码交付仍是必做项。完整31需求／P0–P7／58限定＋34待交付＝92项保持，不能以本阶段通过声明平台全部完成。
