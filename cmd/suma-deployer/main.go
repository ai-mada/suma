package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/deployer"
)

var version = "1.0.0"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.LookupEnv)) }

func run(args []string, out, stderr io.Writer, lookup func(string) (string, bool)) int {
	app := commandlocale.New("suma-deployer", "速码本地静态发布提供方", "suma local static deployment provider", out, stderr, lookup)
	config := app.String("config", "", "管理员本地静态提供方 JSON 配置", "administrator local static-provider JSON configuration")
	if stop, code := app.Parse(args, version); stop {
		return code
	}
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	if *config == "" {
		return app.Failure("必须提供 --config", "--config is required")
	}
	file, err := os.Open(*config)
	if err != nil {
		return app.Failure("提供方配置不可用", "provider configuration unavailable")
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var cfg deployer.Config
	err = decoder.Decode(&cfg)
	_ = file.Close()
	if err != nil {
		return app.Failure("提供方配置无效", "invalid provider configuration")
	}
	if cfg.ManagementAddress == "" {
		cfg.ManagementAddress = "127.0.0.1:4350"
	}
	host, _, err := net.SplitHostPort(cfg.ManagementAddress)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		return app.Failure("management_address 必须是明确的回环地址", "management_address must be an explicit loopback address")
	}
	listener, err := net.Listen("tcp", cfg.ManagementAddress)
	if err != nil {
		return app.Failure("提供方管理监听器不可用", "provider management listener unavailable")
	}
	provider, err := deployer.New(cfg)
	if err != nil {
		_ = listener.Close()
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer provider.Close()
	server := &http.Server{Handler: provider.Handler(), ReadHeaderTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	logger.Info(app.Text("本地静态提供方管理服务开始监听", "local static provider management listening"), "event", "deployer.listening", "address", listener.Addr())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-ctx.Done():
	case err = <-done:
		if err != http.ErrServerClosed {
			logger.Error(app.Text("提供方管理监听器已停止", "provider management listener stopped"), "event", "deployer.listener_stopped")
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
	return 0
}
