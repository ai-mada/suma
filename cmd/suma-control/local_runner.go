package main

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

func runDesktopManagedRunner(app *commandlocale.Command, input io.Reader) int {
	return runDesktopManagedRunnerWith(app, input, runner.RunDesktopManagedRunner)
}

func runDesktopManagedRunnerWith(app *commandlocale.Command, input io.Reader, run func(context.Context, runner.DesktopManagedRunnerInput) error) int {
	reader := bufio.NewReader(io.LimitReader(input, runner.DesktopManagedInputLimit+1))
	line, err := reader.ReadBytes('\n')
	if err != nil || len(line) > runner.DesktopManagedInputLimit {
		return app.Failure("本机执行节点需要有效的私有启动配置", "Local execution needs a valid private startup configuration")
	}
	configuration, err := runner.DecodeDesktopManagedRunnerInput(line)
	if err != nil {
		return app.Failure("本机执行节点的私有配置无效", "Invalid private local Runner configuration")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	// Windows cannot deliver Unix SIGTERM to a Go child. Closing the owned stdin
	// cancels the same execution context on every platform, allowing the original
	// Worker to preserve unknown results and stop contained processes before exit.
	go func() { _, _ = reader.ReadByte(); cancel() }()
	if err = run(ctx, configuration); err != nil && ctx.Err() == nil {
		return app.Failure("本机执行节点未完成启动或已中断，请保留原连接并核对", "The local Runner did not start or was interrupted; preserve and reconcile its original connection")
	}
	return 0
}
