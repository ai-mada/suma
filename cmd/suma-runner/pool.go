package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

func runPool(app *commandlocale.Command, control, path, state string, allow, describe, installedDiscovery, maintenanceOnly, toolPreparation bool) error {
	config, _, err := runner.ReadPoolConfigFile(path)
	if err != nil {
		return fmt.Errorf("%s", app.Text("资源池本地配置不可用", "pool local configuration unavailable"))
	}
	if describe {
		items, err := runner.DescribePoolTools(context.Background(), config, allow)
		if err != nil {
			return err
		}
		return json.NewEncoder(app.Out).Encode(map[string]any{"schema_version": 1, "pool_id": config.PoolID, "runner_id": config.RunnerID, "execution_enabled": allow, "scopes": items})
	}
	token := app.Getenv("SUMA_POOL_RUNNER_TOKEN")
	if token == "" {
		return fmt.Errorf("%s", app.Text("资源池模式需要 SUMA_POOL_RUNNER_TOKEN", "pool mode requires SUMA_POOL_RUNNER_TOKEN"))
	}
	return runConfiguredPool(app, control, config, state, allow, installedDiscovery, maintenanceOnly, toolPreparation, token)
}

func runPoolIdentity(app *commandlocale.Command, control, path, identityPath, state string, allow, installedDiscovery, maintenanceOnly, toolPreparation bool, secrets runner.RunnerSecretStore) (runErr error) {
	config, _, err := runner.ReadPoolConfigFile(path)
	if err != nil {
		return err
	}
	identity, err := runner.LoadPoolRunnerIdentity(identityPath, control, config.PoolID, config.RunnerID, secrets)
	if err != nil {
		return err
	}
	defer func() {
		if err := identity.Close(); err != nil {
			_ = json.NewEncoder(app.Stderr).Encode(runner.PoolDiagnostic{Code: "pool_shutdown_identity_lock"})
			runErr = errors.Join(runErr, runner.ErrPoolShutdownUnconfirmed)
		}
	}()
	if config.Slots > identity.Identity().MaxSlots {
		return fmt.Errorf("local Pool slots exceed the paired identity")
	}
	if !app.FlagProvided("state") {
		state = identity.Path() + ".journal"
	}
	return runConfiguredPool(app, control, config, state, allow, installedDiscovery, maintenanceOnly, toolPreparation, identity.Token())
}

// Both explicit credential sources converge on the original Pool Worker.
// Selecting a system-backed identity never consults an environment fallback.
func runConfiguredPool(app *commandlocale.Command, control string, config runner.PoolLocalConfig, state string, allow, installedDiscovery, maintenanceOnly, toolPreparation bool, token string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client := &runner.Client{BaseURL: control, Token: token, Pool: &runner.PoolContext{PoolID: config.PoolID, RunnerID: config.RunnerID}}
	worker := &runner.PoolWorker{Client: client, Config: config, StateDir: filepath.Join(state, "pool"), AllowExec: allow, AllowInstalledToolDiscovery: installedDiscovery, AllowToolPreparation: toolPreparation, MaintenanceOnly: maintenanceOnly, Diagnostic: func(d runner.PoolDiagnostic) { _ = json.NewEncoder(app.Stderr).Encode(d) }}
	return worker.Run(ctx)
}

func reportUnconfirmedPoolShutdown(app *commandlocale.Command) int {
	return app.Failure(
		"资源池执行器的停止尚未确认。请保留原状态目录，按日志提示核对原执行。",
		"Pool Worker shutdown is unconfirmed. Keep the original state directory and use the diagnostics to check the original executions.",
	)
}
