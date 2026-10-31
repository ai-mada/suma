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

func runACPModelDiscoveryStdin(app *commandlocale.Command, input io.Reader) int {
	raw, err := io.ReadAll(io.LimitReader(input, runner.ACPModelDiscoveryInputLimit+1))
	if err != nil || len(raw) > runner.ACPModelDiscoveryInputLimit {
		return app.Failure("模型探测输入无效", "Invalid private model discovery input")
	}
	configuration, err := runner.DecodeACPModelDiscoveryInput(raw)
	if err != nil {
		return app.Failure("模型探测输入无效", "Invalid private model discovery input")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	result, err := runner.DiscoverACPModels(ctx, configuration)
	if err != nil {
		return app.Failure("工具安装或模型探测依据已变化，请重新检测", "The reviewed installation or discovery basis changed; detect the tool again")
	}
	if json.NewEncoder(app.Out).Encode(result) != nil {
		return app.Failure("模型探测结果无法输出", "Model discovery results could not be written")
	}
	return 0
}
