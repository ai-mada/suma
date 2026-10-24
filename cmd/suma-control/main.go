package main

import (
	"context"
	"flag"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/store"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func main() {
	addr := flag.String("addr", env("SUMA_ADDR", "127.0.0.1:4310"), "listen address")
	flag.Parse()
	data := env("SUMA_DATA_DIR", ".suma")
	if err := os.MkdirAll(data, 0700); err != nil {
		slog.Error("data directory", "error", err)
		os.Exit(1)
	}
	driver := env("SUMA_DB_DRIVER", "sqlite")
	dsn := env("SUMA_DB_DSN", filepath.Join(data, "suma.db"))
	if store.Dialect(driver) != "sqlite" && os.Getenv("SUMA_DB_DSN") == "" {
		slog.Error("SUMA_DB_DSN required for server databases")
		os.Exit(1)
	}
	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		slog.Error("invalid address")
		os.Exit(1)
	}
	ip := net.ParseIP(host)
	if (ip == nil || !ip.IsLoopback()) && os.Getenv("SUMA_SETUP_TOKEN") == "" {
		slog.Error("external binding requires SUMA_SETUP_TOKEN for first-run ownership; configure TLS reverse proxy")
		os.Exit(1)
	}
	db, err := store.Open(driver, dsn)
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	service := &platform.Service{DB: db, Dialect: store.Dialect(driver), DataDir: data}
	service.ReleaseProviders, err = platform.LoadReleaseProviders(os.Getenv("SUMA_RELEASE_PROVIDERS_JSON"), os.Getenv)
	if err != nil {
		slog.Error("release provider configuration", "error", err)
		os.Exit(1)
	}
	if err = service.Migrate(context.Background()); err != nil {
		slog.Error("migrate business schema", "error", err)
		os.Exit(1)
	}
	engine, err := dag.New(db, store.Dialect(driver))
	if err != nil {
		slog.Error("migrate execution schema", "error", err)
		os.Exit(1)
	}
	server := &platform.Server{Service: service, Engine: engine, WebDir: env("SUMA_WEB_DIR", "apps/web/dist"), AllowedOrigins: strings.FieldsFunc(env("SUMA_ALLOWED_ORIGINS", "http://127.0.0.1:5173,http://localhost:5173"), func(r rune) bool { return r == ',' }), SetupToken: os.Getenv("SUMA_SETUP_TOKEN")}
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
					slog.Error("automation tick", "error", e)
				}
				if e := engine.Tick(ctx); e != nil && ctx.Err() == nil {
					slog.Error("scheduler tick", "error", e)
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
					slog.Error("index projection pending", "error", e)
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
					slog.Error("failure projection pending", "error", e)
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
	slog.Info("速码控制面启动", "address", *addr, "database", store.Dialect(driver))
	if err = httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("serve", "error", err)
		os.Exit(1)
	}
}
