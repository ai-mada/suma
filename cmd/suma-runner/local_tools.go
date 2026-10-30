package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

func runInstalledToolDiscovery(app *commandlocale.Command, adapterID string) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	report, err := runner.DiscoverInstalledTools(ctx, adapterID)
	if err != nil {
		return app.Failure("无法安全检测本机工具；请检查工具类型和本机 PATH", "Local tool discovery could not complete safely; check the tool type and local PATH")
	}
	if json.NewEncoder(app.Out).Encode(report) != nil {
		return app.Failure("本机工具检测结果无法输出", "Local tool discovery results could not be written")
	}
	return 0
}
