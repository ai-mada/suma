package localization

// Pairing diagnostics describe platform-owned authorization/configuration
// facts. The remote device's self-reported name remains original content.
var runnerPairingCatalog = func() []Definition {
	rows := [][3]string{
		{"invalid", "Invalid Runner pairing request", "执行节点配对请求无效"},
		{"changed", "Runner pairing changed; refresh the original request", "执行节点配对依据已变化，请刷新原请求后核对"},
		{"expired", "Runner pairing expired; create a new pairing", "执行节点配对已过期，请明确创建新的配对"},
		{"unavailable", "A safe operator-controlled URL is required for Runner pairing", "执行节点配对需要管理员配置安全且可达的控制服务地址"},
		{"limit", "This project already has eight active Runner pairings", "当前项目已有8个有效配对待办，请先处理或等待其过期"},
		{"rejected", "Runner pairing was rejected", "执行节点配对已被拒绝"},
	}
	items := make([]Definition, 0, len(rows))
	for _, row := range rows {
		items = append(items, Definition{Key: "runner_pairing." + row[0], SourceTemplate: row[1], Texts: map[Locale]string{ZhCN: row[2], EnUS: row[1]}})
	}
	return items
}()
