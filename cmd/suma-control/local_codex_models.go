package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

// This explicit private mode runs before database/service initialization. The
// desktop owns the installation and configuration; stdin carries no goal,
// repository, arbitrary process arguments, or platform worker credential.
func runCodexModelDiscovery(app *commandlocale.Command, input io.Reader) int {
	raw, err := io.ReadAll(io.LimitReader(input, runner.CodexModelDiscoveryInputLimit+1))
	if err != nil || len(raw) > runner.CodexModelDiscoveryInputLimit {
		return app.Failure("Codex目录读取需要有效且有界的私有配置", "Codex catalogue discovery needs valid bounded private configuration")
	}
	configuration, err := runner.DecodeCodexModelDiscoveryInput(raw)
	if err != nil {
		return app.Failure("Codex目录的私有配置无效", "Invalid private Codex catalogue configuration")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	result, err := runner.DiscoverCodexModels(ctx, configuration)
	if err != nil {
		return app.Failure("未能核实Codex目录，请核对原观察", "The Codex catalogue could not be verified; reconcile the original observation")
	}
	if json.NewEncoder(app.Out).Encode(result) != nil {
		return app.Failure("无法输出Codex目录观察", "Could not write the Codex catalogue observation")
	}
	return 0
}
