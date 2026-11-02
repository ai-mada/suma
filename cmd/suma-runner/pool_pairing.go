package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

func runPoolPairing(app *commandlocale.Command, pair, control, pool, identity, device string, secrets runner.RunnerSecretStore) int {
	if identity == "" {
		var err error
		identity, err = runner.DefaultPoolRunnerIdentityPath(pair)
		if err != nil {
			return app.Failure("资源池配对参数或本机配置目录无效", "Pool pairing options or local configuration directory are invalid")
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	_, err := runner.PairPoolRunner(ctx, runner.PoolRunnerPairingOptions{PairingID: pair, ControlURL: control, PoolID: pool, IdentityPath: identity, DeviceName: device, Secrets: secrets, Observe: func(value runner.PoolRunnerPairingResult) error { return json.NewEncoder(app.Out).Encode(value) }})
	if err != nil {
		return app.Failure("资源池配对未确认；请保留原身份与系统凭据，核对原待办后重试同一命令", "Pool pairing is unconfirmed; preserve original identity and system credential, review the original request and retry the same command")
	}
	return 0
}
