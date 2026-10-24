package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/store"
)

var version = "dev"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.LookupEnv)) }

func run(args []string, out, stderr io.Writer, lookup func(string) (string, bool)) int {
	app := commandlocale.New("suma-control", "速码控制面服务", "suma control service", out, stderr, lookup)
	addr := app.String("addr", app.Env("SUMA_ADDR", "127.0.0.1:4310"), "监听地址", "listen address")
	if stop, code := app.Parse(args, version); stop {
		return code
	}
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	data := app.Env("SUMA_DATA_DIR", ".suma")
	if err := os.MkdirAll(data, 0700); err != nil {
		logger.Error(app.Text("数据目录不可用", "data directory"), "event", "control.data_directory", "error", err)
		return 1
	}
	driver := app.Env("SUMA_DB_DRIVER", "sqlite")
	dsn := app.Env("SUMA_DB_DSN", filepath.Join(data, "suma.db"))
	if store.Dialect(driver) != "sqlite" && app.Getenv("SUMA_DB_DSN") == "" {
		logger.Error(app.Text("服务端数据库需要 SUMA_DB_DSN", "SUMA_DB_DSN required for server databases"), "event", "control.database_dsn_required")
		return 1
	}
	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		logger.Error(app.Text("监听地址无效", "invalid address"), "event", "control.address_invalid")
		return 1
	}
	ip := net.ParseIP(host)
	if (ip == nil || !ip.IsLoopback()) && app.Getenv("SUMA_SETUP_TOKEN") == "" {
		logger.Error(app.Text("外部监听需要 SUMA_SETUP_TOKEN 确立首次所有权；请配置 TLS 反向代理", "external binding requires SUMA_SETUP_TOKEN for first-run ownership; configure TLS reverse proxy"), "event", "control.external_binding_guard")
		return 1
	}
	db, err := store.Open(driver, dsn)
	if err != nil {
		logger.Error(app.Text("打开数据库失败", "open database"), "event", "control.database_open", "error", err)
		return 1
	}
	defer db.Close()
	service := &platform.Service{DB: db, Dialect: store.Dialect(driver), DataDir: data}
	service.AutomationIngressKey, err = platform.LoadAutomationIngressKey(data, store.Dialect(driver), app.Getenv)
	if err != nil {
		logger.Error(app.Text("自动化接入密钥配置失败", "automation ingress key configuration"), "event", "control.automation_key_configuration", "error", err)
		return 1
	}
	service.ReleaseProviders, err = platform.LoadReleaseProviders(app.Getenv("SUMA_RELEASE_PROVIDERS_JSON"), app.Getenv)
	if err != nil {
		logger.Error(app.Text("发布提供方配置失败", "release provider configuration"), "event", "control.release_provider_configuration", "error", err)
		return 1
	}
	if err = service.Migrate(context.Background()); err != nil {
		logger.Error(app.Text("业务数据库结构迁移失败", "migrate business schema"), "event", "control.business_schema_migrate", "error", err)
		return 1
	}
	engine, err := dag.New(db, store.Dialect(driver))
	if err != nil {
		logger.Error(app.Text("执行数据库结构迁移失败", "migrate execution schema"), "event", "control.execution_schema_migrate", "error", err)
		return 1
	}
	server := &platform.Server{Service: service, Engine: engine, WebDir: app.Env("SUMA_WEB_DIR", "apps/web/dist"), AllowedOrigins: strings.FieldsFunc(app.Env("SUMA_ALLOWED_ORIGINS", "http://127.0.0.1:5173,http://localhost:5173"), func(r rune) bool { return r == ',' }), SetupToken: app.Getenv("SUMA_SETUP_TOKEN")}
	httpServer := &http.Server{Addr: *addr, Handler: server.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if e := service.TickAutomations(ctx, engine); e != nil && ctx.Err() == nil {
					logger.Error(app.Text("自动化调度失败", "automation tick"), "event", "control.automation_tick", "error", e)
				}
				if e := engine.Tick(ctx); e != nil && ctx.Err() == nil {
					logger.Error(app.Text("执行调度失败", "scheduler tick"), "event", "control.scheduler_tick", "error", e)
				}
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if e := service.ProjectIndexes(ctx); e != nil && ctx.Err() == nil {
					logger.Error(app.Text("索引投影待处理", "index projection pending"), "event", "control.index_projection", "error", e)
				}
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if e := service.ProjectFailures(ctx); e != nil && ctx.Err() == nil {
					logger.Error(app.Text("故障投影待处理", "failure projection pending"), "event", "control.failure_projection", "error", e)
				}
			}
		}
	}()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpServer.Shutdown(shutdown)
	}()
	logger.Info(app.Text("速码控制面启动", "suma control service started"), "event", "control.started", "address", *addr, "database", store.Dialect(driver))
	if err = httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error(app.Text("控制面服务失败", "serve"), "event", "control.serve", "error", err)
		return 1
	}
	return 0
}
