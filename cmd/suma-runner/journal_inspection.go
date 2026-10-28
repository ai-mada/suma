package main

import (
	"encoding/json"
	"fmt"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

func runJournalInspection(app *commandlocale.Command, state string) int {
	result := runner.InspectJournalState(state)
	if json.NewEncoder(app.Out).Encode(result) != nil {
		return app.Failure("本地执行日志诊断无法输出", "Local execution journal inspection could not be written")
	}
	var zh, en string
	switch result.Status {
	case "available":
		zh, en = "已读取本地执行日志快照；未检查服务器确认或执行器健康", "Local execution journal snapshot read; server acknowledgements and runner health were not checked"
	case "missing":
		zh, en = "本地执行日志不存在，未创建目录或状态", "Local execution journal is missing; no directory or state was created"
	case "invalid":
		zh, en = "本地执行日志内容无效，未输出部分数据", "Local execution journal is invalid; no partial data was reported"
	case "over_limit":
		zh, en = "本地执行日志超过 16 MiB 读取上限", "Local execution journal exceeds the 16 MiB read limit"
	default:
		zh, en = "本地执行日志无法安全读取", "Local execution journal cannot be read safely"
	}
	fmt.Fprintln(app.Stderr, app.Text(zh, en))
	if result.Status != "available" {
		return 1
	}
	return 0
}
