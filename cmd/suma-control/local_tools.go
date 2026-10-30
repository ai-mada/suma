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

// The desktop uses the already attested control binary as a read-only probe.
// This entry must run before directory creation, database/configuration reads,
// credential initialization or any control-plane background worker.
func runInstalledToolDiscovery(app *commandlocale.Command, adapterID string) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	result, err := runner.DiscoverInstalledTools(ctx, adapterID)
	if err != nil {
		return app.Failure("本机工具检测失败，请检查工具类型后重试", "Local tool detection failed; check the selected tool and try again")
	}
	if err = json.NewEncoder(app.Out).Encode(result); err != nil {
		return app.Failure("无法输出工具检测结果", "Could not write tool detection results")
	}
	return 0
}
