package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

func runPoolConnectionCheck(app *commandlocale.Command, control, path, identityPath string, secrets runner.RunnerSecretStore) int {
	failure := func() int {
		return app.Failure("资源池连接核对未确认；请保留原配置与身份，核对配对、当前授权及节点容量。", "Pool connection check is unconfirmed; preserve the configuration and identity, and review pairing, current grants and node capacity.")
	}
	config, fileSHA, err := runner.ReadPoolConfigFile(path)
	if err != nil {
		return failure()
	}
	identity, err := runner.LoadPoolRunnerIdentity(identityPath, control, config.PoolID, config.RunnerID, secrets)
	if err != nil {
		return failure()
	}
	defer identity.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	client := &runner.Client{BaseURL: control, Token: identity.Token(), Pool: &runner.PoolContext{PoolID: config.PoolID, RunnerID: config.RunnerID}}
	report, err := runner.CheckPoolConnection(ctx, client, config, identity.Identity(), fileSHA)
	if err != nil {
		return failure()
	}
	if json.NewEncoder(app.Out).Encode(report) != nil {
		return failure()
	}
	return 0
}
