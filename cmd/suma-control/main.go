package main

import (
	"context"
	"github.com/ai-mada/suma/internal/buildinfo"
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

const version = buildinfo.Version

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.LookupEnv)) }

func run(args []string, out, stderr io.Writer, lookup func(string) (string, bool)) int {
	app := commandlocale.New("suma-control", "速码控制面服务", "suma control service", out, stderr, lookup)
	addr := app.String("addr", app.Env("SUMA_ADDR", "127.0.0.1:4310"), "监听地址", "listen address")
	maintenance := app.Bool("maintenance", false, "升级核验模式：暂停业务命令及后台调度和投影", "upgrade verification: pause business commands and background scheduling/projection")
	discoverInstalled := app.Bool("discover-installed-tools", false, "只读检测本机已安装工具，不启动控制服务或读取业务凭据", "detect installed local tools without starting the service or reading business credentials")
	localTool := app.String("local-tool", "", "本机检测类型，例如 codex-jsonl、claude-code-cli、qoder-acp；空值检测全部", "local detection entry, for example codex-jsonl, claude-code-cli or qoder-acp; empty selects all")
	managedRunner := app.Bool("desktop-managed-runner", false, "由桌面私有管道管理项目执行节点，不启动控制服务", "run the project worker through the private desktop pipe, without starting the control service")
	describeACP := app.Bool("describe-acp-models-stdin", false, "明确读取私有原生工具的模型目录，不发送开发目标", "explicitly read a private native tool model catalogue without sending a development goal")
	if stop, code := app.Parse(args, version); stop {
		return code
	}
	if *describeACP {
		if *maintenance || *discoverInstalled || *localTool != "" || *managedRunner {
			return app.Failure("模型目录读取不能与服务或其他工具模式混用", "Model catalogue discovery cannot be combined with service or other tool modes")
		}
		return runACPModelDiscovery(app, os.Stdin)
	}
	if *managedRunner {
		if *maintenance || *discoverInstalled || *localTool != "" {
			return app.Failure("本机托管执行模式不能与服务或检测参数混用", "Managed local execution cannot be combined with service or detection options")
		}
		return runDesktopManagedRunner(app, os.Stdin)
	}
	if *discoverInstalled {
		if *maintenance {
			return app.Failure("本机检测不能与服务维护模式混用", "Local detection cannot be combined with service maintenance")
		}
		return runInstalledToolDiscovery(app, *localTool)
	}
	if *localTool != "" {
		return app.Failure("--local-tool 只能与 --discover-installed-tools 使用", "--local-tool requires --discover-installed-tools")
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
	desktopKey, err := desktopWorkspaceKey(app.Getenv("SUMA_DESKTOP_WORKSPACE_KEY"), driver, ip)
	if err != nil {
		logger.Error(app.Text("桌面目录授权配置无效", "invalid desktop workspace configuration"), "event", "control.desktop_workspace_configuration", "error", err)
		return 1
	}
	publicURL, publicURLSource, err := platform.ResolveMCPPublicURL(app.Getenv("SUMA_PUBLIC_URL"), *addr)
	if err != nil {
		logger.Error(app.Text("公开服务地址配置无效", "invalid public service URL configuration"), "event", "control.public_url_invalid")
		return 1
	}
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
	service := &platform.Service{DB: db, Dialect: store.Dialect(driver), DataDir: data, DesktopWorkspaceKey: desktopKey}
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
	service.RepositoryProviders, err = platform.LoadRepositoryProviders(app.Getenv("SUMA_REPOSITORY_PROVIDERS_JSON"), app.Getenv)
	if err != nil {
		logger.Error(app.Text("代码集成提供方配置失败", "repository integration provider configuration"), "event", "control.repository_provider_configuration", "error", err)
		return 1
	}
	service.ApplicationSourceInspector, err = platform.LoadApplicationSourceInspector(app.Getenv)
	if err != nil {
		logger.Error(app.Text("源码解析器配置失败", "source inspector configuration"), "event", "control.source_inspector_configuration", "error", err)
		return 1
	}
	if err = service.Initialize(context.Background()); err != nil {
		logger.Error(app.Text("业务数据库结构迁移失败", "migrate business schema"), "event", "control.business_schema_migrate", "error", err)
		return 1
	}
	workBuddy, err := configureWorkBuddyConnections(service, publicURL, app.Getenv)
	if err != nil {
		logger.Error(app.Text("WorkBuddy连接配置失败", "WorkBuddy connection configuration failed"), "event", "control.workbuddy_configuration", "error", err)
		return 1
	}
	engine, err := dag.New(db, store.Dialect(driver))
	if err != nil {
		logger.Error(app.Text("执行数据库结构迁移失败", "migrate execution schema"), "event", "control.execution_schema_migrate", "error", err)
		return 1
	}
	engine.AttemptAccounting = service
	engine.AttemptReconciliation = service
	server := &platform.Server{BuildVersion: version, Service: service, Engine: engine, WebDir: app.Env("SUMA_WEB_DIR", "apps/web/dist"), AllowedOrigins: strings.FieldsFunc(app.Env("SUMA_ALLOWED_ORIGINS", "http://127.0.0.1:5173,http://localhost:5173"), func(r rune) bool { return r == ',' }), SetupToken: app.Getenv("SUMA_SETUP_TOKEN"), PublicURL: publicURL, PublicURLSource: publicURLSource, Maintenance: *maintenance}
	server.WorkBuddy = workBuddy
	defer closeWorkBuddyCloudTurns(workBuddy, app.Text, logger)
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
				if *maintenance {
					continue
				}
				if e := service.TickAutomations(ctx, engine); e != nil && ctx.Err() == nil {
					logger.Error(app.Text("自动化调度失败", "automation tick"), "event", "control.automation_tick", "error", e)
				}
				if e := engine.Tick(ctx); e != nil && ctx.Err() == nil {
					logger.Error(app.Text("执行调度失败", "scheduler tick"), "event", "control.scheduler_tick", "error", e)
				}
				if _, e := service.ExpireWorkspaceObservations(ctx); e != nil && ctx.Err() == nil {
					logger.Error(app.Text("工作区观察到期待处理", "workspace observation expiry pending"), "event", "control.workspace_observation_expiry", "error", e)
				}
				if _, e := service.ExpireWorkspaceWrites(ctx); e != nil && ctx.Err() == nil {
					logger.Error(app.Text("工作区保存核对待处理", "workspace save reconciliation pending"), "event", "control.workspace_write_expiry", "error", e)
				}
			}
		}
	}()
	// Business dependency advancement has its own bounded worker. Slow project
	// checks cannot hold the DAG lease/ready tick or create a browser dispatch loop.
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if *maintenance {
					continue
				}
				tickCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				err := service.TickExpertTeamPlans(tickCtx, engine)
				cancel()
				if err != nil && ctx.Err() == nil {
					logger.Error(app.Text("专家团队计划推进失败", "expert team plan advancement"), "event", "control.expert_team_tick", "error", err)
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
				if *maintenance {
					continue
				}
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
				if *maintenance {
					continue
				}
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
