package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

func runPoolStateInspection(app *commandlocale.Command, state string) int {
	result := runner.InspectPoolState(filepath.Join(state, "pool"))
	if json.NewEncoder(app.Out).Encode(result) != nil {
		return app.Failure("无法输出资源池本地诊断", "Could not write the local Pool inspection")
	}
	if result.Status == "available" && result.Complete {
		fmt.Fprintln(app.Stderr, app.Text("已读取资源池本地快照。按槽位核对原执行，实时状态请查看控制面。", "Local Pool snapshot read. Inspect the original execution by slot. See the control plane for live status."))
		return 0
	}
	switch result.Status {
	case "available":
		return app.Failure("只读取到部分资源池记录。请查看槽位日志和领取状态。", "Only part of the local Pool state was read. Check the slot journal and claim states.")
	case "missing":
		return app.Failure("未找到资源池状态。请使用执行器原有的 --state 目录。", "Pool state is missing. Use the original Runner --state directory.")
	case "over_limit":
		return app.Failure("资源池状态超出读取上限。请按槽位查看原日志。", "Pool state exceeds the read limit. Inspect the original journal by slot.")
	case "invalid":
		return app.Failure("资源池本地状态格式无效。请保留原目录并核对。", "Local Pool state is invalid. Keep the original directory for reconciliation.")
	default:
		return app.Failure("无法安全读取资源池本地状态。", "Could not safely read local Pool state.")
	}
}
