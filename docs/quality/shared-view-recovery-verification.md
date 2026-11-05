# 共享任务视图写入恢复验证

T-MULTICA-SHARED-VIEW-RECOVERY。Windows、Go 1.26.10、Vite 6.1.0、Chrome、隔离 SQLite。保留活动预览和用户数据库。

- 在 apps/web 运行 `node node_modules/vitest/vitest.mjs run src/SharedTaskViews.test.tsx src/TaskViews.test.tsx`，23 项共享视图和 6 项工作台定向检查通过，覆盖同标签页重挂载、写入前持久化、原回执与较新目录分离、账号隔离、当前撤权、权限降级、存储失败及版本冲突后的个人草稿。最后一次界面修订后复验 23 项共享视图检查。Web TypeScript 和双语检查通过，10325 对词条，生产硬编码 0。
- 使用仓库 Go 缓存构建隔离控制面，Vite 独立目录构建通过，保留既有包体积警告。服务端写入和回执合同未变，复用上一阶段的限定后端验证。
- Chrome 最终通过 9 项断言。受邀开发者创建共享视图，服务端提交后截断 201 响应。另一账号修改到修订 2 后，同标签页重载自动展开筛选区域并恢复核对入口，无自动写入。明确重试发送相同 JSON、操作 ID 和当前身份保护头，原修订 1 回执未覆盖新修订或当前筛选。恢复记录清除，数据库目录仍只有一条修订 2 的视图。
- 目视核查 1366px 英文浅色与 390px 英文深色截图。初次隔离执行无法启动 Chrome 的 GPU 子进程，获准宿主执行后保留 Chrome 沙箱并完成测试。首次通过后的目视核查发现空列表文案与待核对状态冲突，修正并按最终构建复验。
- 最终报告 `.cache/shared-view-recovery/browser-Cug4S9/result.json`，SHA256 `5b7135d5fe5fabb4ce7069f323b18146fe4e90b256f740925b0a5614b9b4d188`。报告、原始截图和摘要保留于任务忽略目录。临时构建、浏览器数据和数据库按摘要盘点后清理，共 327 个文件，见 `.cache/shared-view-recovery/scratch-cleanup.json`。
- 仍待验证关闭标签页后的恢复、跨设备写入队列、完整离线协作、物理多机及生产接入。
