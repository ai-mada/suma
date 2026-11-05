package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

var errDesktopRunnerStopRequested = errors.New("desktop requested Runner stop")

type desktopRunnerShutdownReceipt struct {
	Type     string `json:"type"`
	WorkerID string `json:"worker_id"`
	PID      int    `json:"pid"`
}

func runDesktopManagedRunner(app *commandlocale.Command, input io.Reader) int {
	return runDesktopManagedRunnerWith(app, input, runner.RunDesktopManagedRunner)
}

func runDesktopManagedRunnerWith(app *commandlocale.Command, input io.Reader, run func(context.Context, runner.DesktopManagedRunnerInput) error) int {
	reader := bufio.NewReaderSize(input, runner.DesktopManagedInputLimit+1)
	line, err := reader.ReadSlice('\n')
	if err != nil || len(line) > runner.DesktopManagedInputLimit {
		return app.Failure("本机执行节点需要有效的私有启动配置", "Local execution needs a valid private startup configuration")
	}
	configuration, err := runner.DecodeDesktopManagedRunnerInput(line)
	if err != nil {
		return app.Failure("本机执行节点的私有配置无效", "Invalid private local Runner configuration")
	}
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	ctx, cancel := context.WithCancelCause(signalCtx)
	defer cancel(nil)
	// The private pipe supplies a portable stop request. Parent loss (EOF) also
	// cancels work, but only an explicit request can receive a stop receipt.
	go func() {
		frame, err := reader.ReadSlice('\n')
		if err == nil && string(frame) == "stop\n" {
			cancel(errDesktopRunnerStopRequested)
			return
		}
		cancel(nil)
	}()
	if err = run(ctx, configuration); err != nil {
		return app.Failure("本机执行节点未完成启动或已中断，请保留原连接并核对", "The local Runner did not start or was interrupted; preserve and reconcile its original connection")
	}
	if context.Cause(ctx) == errDesktopRunnerStopRequested {
		receipt := desktopRunnerShutdownReceipt{Type: "suma.local_runner.shutdown", WorkerID: configuration.WorkerID, PID: os.Getpid()}
		if err := json.NewEncoder(app.Out).Encode(receipt); err != nil {
			return app.Failure("无法写入停止回执，请核对原连接", "Could not write the stop receipt; check the original connection")
		}
	}
	return 0
}
