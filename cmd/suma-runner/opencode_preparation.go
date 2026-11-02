package main

import (
	"context"
	"io"
	"os"
	"os/signal"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

func runOpenCodeToolPreparationStdin(app *commandlocale.Command, file string, input io.Reader) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	result, err := runner.PrepareOpenCodeToolCommands(ctx, file, input)
	if err != nil {
		return app.Failure("OpenCode SDK 配置未准备；请核对九项私有输入、已审核资源和工具包之外的全新文件", "OpenCode SDK configuration was not prepared; check the nine private fields, reviewed assets and a new file outside the tool package")
	}
	return outputToolPreparation(app, result)
}
