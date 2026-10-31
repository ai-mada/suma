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

// This private, explicit entry precedes all service initialization. It uses the
// same reviewed local probe as the Runner CLI; no database, worker credential,
// arbitrary command line or development prompt is accepted here.
func runACPModelDiscovery(app *commandlocale.Command, input io.Reader) int {
	raw, err := io.ReadAll(io.LimitReader(input, runner.ACPModelDiscoveryInputLimit+1))
	if err != nil || len(raw) > runner.ACPModelDiscoveryInputLimit {
		return app.Failure("模型目录需要有效且有界的私有配置", "Model catalogue discovery needs a valid bounded private configuration")
	}
	configuration, err := runner.DecodeACPModelDiscoveryInput(raw)
	if err != nil {
		return app.Failure("模型目录的私有配置无效", "Invalid private model catalogue configuration")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	result, err := runner.DiscoverACPModels(ctx, configuration)
	if err != nil {
		return app.Failure("未能核实模型目录，请保留原观察并核对", "The model catalogue could not be verified; retain and reconcile the original observation")
	}
	if json.NewEncoder(app.Out).Encode(result) != nil {
		return app.Failure("无法输出模型目录观察", "Could not write the model catalogue observation")
	}
	return 0
}
