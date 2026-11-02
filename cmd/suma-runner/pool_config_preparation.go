package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
		return writePoolConfigDiagnostic(app, destination, err)
	}
	if json.NewEncoder(app.Out).Encode(report) != nil {
		// A failing stdout writer may already have sent a prefix. Preserve that
		// uncertainty; never infer the new file was absent or retry preparation.
		return writePoolConfigDiagnostic(app, destination, &runner.PoolConfigPreparationError{Code: "output_unconfirmed", Field: "output"})
	}
	return 0
}

func writePoolConfigDiagnostic(app *commandlocale.Command, destination string, err error) int {
	failure := &runner.PoolConfigPreparationError{Code: "configuration_invalid", Field: "input"}
	var classified *runner.PoolConfigPreparationError
	if errors.As(err, &classified) {
		failure = classified
	}
	outputSHA := sha256.Sum256([]byte(destination))
	diagnostic := runner.PoolConfigDiagnostic{Protocol: runner.PoolConfigDiagnosticProtocol, Code: failure.Code, ScopeIndex: failure.ScopeIndex, Field: failure.Field, InputSHA256: failure.InputSHA256, OutputSHA256: hex.EncodeToString(outputSHA[:]), Message: poolConfigDiagnosticMessage(app, failure)}
	_ = json.NewEncoder(app.Stderr).Encode(diagnostic)
	return 1
}

// Only the fixed code and field labels enter display text. Do not include the
// error string, file path, source JSON or any executable/provider metadata.
func poolConfigDiagnosticMessage(app *commandlocale.Command, failure *runner.PoolConfigPreparationError) string {
	message := app.Text("请核对本机配置组合与大小，并保留原文件。", "Please check the combined local configuration and its size; preserve original files.")
	switch failure.Code {
	case "input_invalid":
		message = app.Text("请核对完整私有输入的字段、格式与大小。", "Please check the complete private input fields, format and size.")
	case "output_invalid":
		message = app.Text("请选择已有目录中的全新绝对输出文件。", "Please choose a new absolute output file in an existing directory.")
	case "output_unavailable":
		message = app.Text("请核对输出父目录是否存在且可写，并保留原文件。", "Please check that the output parent directory exists and is writable; preserve original files.")
	case "output_exists":
		message = app.Text("请保留原文件并选择全新输出文件。", "Please preserve the existing file and choose a new output file.")
	case "output_unconfirmed":
		message = app.Text("请核对原目标文件和输出，不要覆盖或自动重试。", "Please inspect the original target file and output; do not overwrite it or retry automatically.")
	case "directory_unavailable":
		message = app.Text("请在目标机器核对现有目录和访问条件。", "Please check the existing directory and access conditions on the target machine.")
	case "workspace_conflict":
		message = app.Text("请为每个项目选择独立工作区。", "Please choose a separate workspace for each project.")
	case "source_unavailable":
		message = app.Text("请在目标机器核对已有配置文件及访问条件。", "Please check the existing configuration file and access conditions on the target machine.")
	case "source_invalid":
		message = app.Text("请修复该本机配置文件的格式与允许列表。", "Please correct the format and allowlist in this local configuration file.")
	case "inputs_changed":
		message = app.Text("请保留原文件，核对已变化的输入或目录后再明确准备。", "Please preserve original files and review changed inputs or directories before preparing again explicitly.")
	case "cancelled":
		message = app.Text("已取消准备，请核对目标文件后决定后续操作。", "Preparation was cancelled; please inspect the target file before deciding what to do next.")
	}
	if failure.ScopeIndex > 0 {
		label := app.Text("配置", "configuration")
		switch failure.Field {
		case "workspace":
			label = app.Text("工作区", "workspace")
		case "repository":
			label = app.Text("仓库", "repository")
		case "worktrees":
			label = app.Text("工作树根目录", "worktree root")
		case "commands_file":
			label = app.Text("命令配置文件", "commands file")
		case "model_deployments_file":
			label = app.Text("模型部署配置文件", "model deployments file")
		case "external_config_file":
			label = app.Text("外部配置文件", "external configuration file")
		}
		return fmt.Sprintf(app.Text("第 %d 行 · %s：", "Scope %d · %s: "), failure.ScopeIndex, label) + message
	}
	return message
}
