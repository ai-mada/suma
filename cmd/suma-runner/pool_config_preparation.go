package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/signal"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

func runPoolConfigPreparation(app *commandlocale.Command, destination string, input io.Reader) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	report, err := runner.PreparePoolLocalConfig(ctx, destination, input)
	if err != nil {
		return app.Failure("资源池配置未准备；请核对私有输入、现有目录和本机引用文件，并使用全新输出文件", "Pool configuration was not prepared; check the private input, existing directories and local references, and use a new output file")
	}
	if json.NewEncoder(app.Out).Encode(report) != nil {
		return app.Failure("资源池配置输出未确认；请核对所选本机文件，不要覆盖原文件", "Pool configuration output is unconfirmed; inspect the selected local file without overwriting it")
	}
	return 0
}
