package main

import (
	"context"
	"fmt"
	"github.com/ai-mada/suma/internal/buildinfo"
	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/repositoryprovider"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const version = buildinfo.Version

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.LookupEnv)) }
func run(args []string, out, stderr io.Writer, lookup func(string) (string, bool)) int {
	app := commandlocale.New("suma-repository-provider", "速码受管仓库提供方", "suma governed repository provider", out, stderr, lookup)
	config := app.String("config", "", "管理员固定仓库 JSON 配置", "administrator fixed repository JSON configuration")
	state := app.String("state", ".cache/repository-provider", "提供方持久状态目录", "provider durable state directory")
	addr := app.String("addr", "127.0.0.1:4351", "提供方回环监听地址", "provider loopback listener address")
	if stop, code := app.Parse(args, version); stop {
		return code
	}
	if *config == "" {
		return app.Failure("必须提供 --config", "--config is required")
	}
	raw, err := os.ReadFile(*config)
	if err != nil {
		return app.Failure("仓库提供方配置不可用", "repository provider configuration unavailable")
	}
	cfg, err := repositoryprovider.DecodeConfig(raw)
	if err != nil {
		return app.Failure("仓库提供方配置无效", "invalid repository provider configuration")
	}
	cfg.StateDir, err = filepath.Abs(*state)
	if err != nil {
		return 1
	}
	host, _, err := net.SplitHostPort(*addr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		return app.Failure("必须使用明确的回环监听地址", "an explicit loopback listener address is required")
	}
	provider, err := repositoryprovider.New(cfg)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer provider.Close()
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return app.Failure("仓库提供方监听器不可用", "repository provider listener unavailable")
	}
	server := &http.Server{Handler: provider.Handler(), ReadHeaderTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-ctx.Done():
	case err = <-done:
		if err != http.ErrServerClosed {
			fmt.Fprintln(stderr, app.Text("仓库提供方监听器已停止", "repository provider listener stopped"))
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
	return 0
}
