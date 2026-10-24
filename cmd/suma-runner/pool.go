package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/ai-mada/suma/internal/runner"
)

func runPool(control, path, state string, allow, describe bool) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("pool local configuration unavailable")
	}
	config, err := runner.DecodePoolConfig(raw)
	if err != nil {
		return err
	}
	if describe {
		items, err := runner.DescribePoolTools(context.Background(), config, allow)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"schema_version": 1, "pool_id": config.PoolID, "runner_id": config.RunnerID, "execution_enabled": allow, "scopes": items})
	}
	token := os.Getenv("SUMA_POOL_RUNNER_TOKEN")
	if token == "" {
		return fmt.Errorf("pool mode requires SUMA_POOL_RUNNER_TOKEN")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client := &runner.Client{BaseURL: control, Token: token, Pool: &runner.PoolContext{PoolID: config.PoolID, RunnerID: config.RunnerID}}
	worker := &runner.PoolWorker{Client: client, Config: config, StateDir: filepath.Join(state, "pool"), AllowExec: allow, Diagnostic: func(d runner.PoolDiagnostic) { _ = json.NewEncoder(os.Stderr).Encode(d) }}
	err = worker.Run(ctx)
	if ctx.Err() != nil {
		return nil
	}
	return err
}
