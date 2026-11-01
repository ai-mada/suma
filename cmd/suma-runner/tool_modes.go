package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/signal"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func toolModeHasAnyFlag(app *commandlocale.Command, names ...string) bool {
	for _, name := range names {
		if app.FlagProvided(name) {
			return true
		}
	}
	return false
}

func runToolPreparation(app *commandlocale.Command, file, adapterID string) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	result, err := runner.PrepareToolCommands(ctx, file, adapterID)
	if err != nil {
		return app.Failure("工具配置未准备；请检查工具类型及全新文件位置，不覆盖原配置或使用链接目录", "Tool configuration was not prepared; check the tool type and a new file location without overwriting existing files or using linked directories")
	}
	return outputToolPreparation(app, result)
}

func runACPToolPreparationStdin(app *commandlocale.Command, file string, input io.Reader) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	result, err := runner.PrepareACPToolCommands(ctx, file, input)
	if err != nil {
		return app.Failure("ACP 工具配置未准备；请核对五项输入，并使用工具包目录之外、不含链接的全新文件", "ACP tool configuration was not prepared; check the five input fields and a new file outside the package without linked directories")
	}
	return outputToolPreparation(app, result)
}

func outputToolPreparation(app *commandlocale.Command, result runner.ToolPreparation) int {
	if json.NewEncoder(app.Out).Encode(result) != nil {
		return app.Failure("工具配置结果无法输出；请先核对指定文件", "Tool preparation result could not be written; check the selected file before retrying")
	}
	if !result.Prepared {
		return 1
	}
	return 0
}

func runToolReport(ctx context.Context, app *commandlocale.Command, worker *runner.Worker) int {
	report, recovered, err := worker.ReportTools(ctx)
	if err != nil {
		return app.Failure("工具清单回执未确认；请保留原状态目录，使用相同项目与执行节点重新上报", "Tool inventory acknowledgement is unconfirmed; preserve the original state directory and retry with the same project and worker")
	}
	result := struct {
		Protocol    string           `json:"protocol"`
		ProjectID   string           `json:"project_id"`
		WorkerID    string           `json:"worker_id"`
		OperationID string           `json:"operation_id"`
		ObservedAt  string           `json:"observed_at"`
		Recovered   bool             `json:"recovered"`
		Items       []tc.Observation `json:"items"`
	}{"suma-runner-tool-report-v1", worker.Client.ProjectID, report.WorkerID, report.OperationID, report.ObservedAt, recovered, report.Items}
	if json.NewEncoder(app.Out).Encode(result) != nil {
		return app.Failure("清单已确认，但本地结果无法输出；请在原执行节点页面核对", "Inventory was acknowledged, but the local result could not be written; check the original worker page")
	}
	return 0
}
