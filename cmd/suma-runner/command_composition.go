package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/signal"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

// This mode creates an explicitly selected new configuration. It never reports
// tools or starts a worker, even when the configuration is valid.
func runCommandComposition(app *commandlocale.Command, destination string, input io.Reader) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	report, err := runner.PrepareCommandComposition(ctx, destination, input)
	if err != nil {
		return writeCommandCompositionDiagnostic(app, err)
	}
	if json.NewEncoder(app.Out).Encode(report) != nil {
		// The durable file may already exist. An unconfirmed stdout receipt must
		// never trigger an overwrite, deletion or automatic retry here.
		return writeCommandCompositionDiagnostic(app, &runner.CommandCompositionError{Code: "output_unconfirmed"})
	}
	return 0
}

func writeCommandCompositionDiagnostic(app *commandlocale.Command, err error) int {
	failure := &runner.CommandCompositionError{Code: "configuration_invalid"}
	var classified *runner.CommandCompositionError
	if errors.As(err, &classified) {
		failure = classified
	}
	message := app.Text("请核对组合配置并保留原文件。", "Please check the combined configuration and preserve original files.")
	switch failure.Code {
	case "mode_conflict":
		message = app.Text("命令组合不能与身份、执行器或其他配置模式混用。", "Command composition cannot be combined with identity, worker or other configuration modes.")
	case "input_invalid":
		message = app.Text("请填写2至16个不同的已有绝对文件引用。", "Please provide 2 to 16 distinct existing absolute file references.")
	case "output_invalid":
		message = app.Text("请在已有本机目录中选择全新的绝对输出文件。", "Please select a new absolute output file in an existing local directory.")
	case "output_exists":
		message = app.Text("请保留已存在文件，选择全新输出文件。", "Please preserve the existing file and select a new output file.")
	case "source_unavailable":
		message = app.Text("请在目标机器核对该源文件、父目录及读取权限。", "Please check this source file, its parent directories and read permissions on the target machine.")
	case "source_invalid":
		message = app.Text("请核对该源配置的JSON、字段和大小；不会改写原件。", "Please check this source configuration's JSON, fields and size; the original is preserved.")
	case "duplicate_command":
		message = app.Text("请修正重名命令后重新明确组合；不会覆盖或自动改名。", "Please resolve duplicate command names before composing explicitly; no overwrite or automatic renaming is performed.")
	case "inputs_changed":
		message = app.Text("请核对已变化的源文件或目录，再明确组合。", "Please review changed source files or directories before composing again explicitly.")
	case "cancelled":
		message = app.Text("组合已取消，请核对目标文件后决定后续操作。", "Composition was cancelled; please inspect the destination before deciding the next action.")
	case "output_unconfirmed":
		message = app.Text("输出回执未确认，请核对原目标文件；不要覆盖或自动重试。", "The output receipt is unconfirmed; inspect the original destination without overwriting it or retrying automatically.")
	}
	// Raw errors may contain local paths, executable metadata or credentials.
	// Publish only a fixed code, source position and translated safe message.
	diagnostic := struct {
		Protocol    string `json:"protocol"`
		Code        string `json:"code"`
		SourceIndex int    `json:"source_index"`
		Message     string `json:"message"`
	}{"suma-command-composition-diagnostic-v1", failure.Code, failure.SourceIndex, message}
	_ = json.NewEncoder(app.Stderr).Encode(diagnostic)
	return 1
}
