package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

func runRemotePairing(app *commandlocale.Command, pair, control, project, identity, device string, secrets runner.RunnerSecretStore) int {
	if identity == "" {
		var err error
		identity, err = runner.DefaultRunnerIdentityPath(pair)
		if err != nil {
			return app.Failure("配对参数或本机配置目录无效", "Pairing options or the local configuration directory are invalid")
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	_, err := runner.PairRemoteRunner(ctx, runner.RunnerPairingOptions{PairingID: pair, ControlURL: control, ProjectID: project, IdentityPath: identity, DeviceName: device, Secrets: secrets, Observe: func(value runner.RunnerPairingResult) error { return json.NewEncoder(app.Out).Encode(value) }})
	if err != nil {
		return app.Failure("配对未确认完成；请保留原身份文件与系统凭据，在浏览器核对原待办后用同一命令重试", "Pairing is not confirmed; preserve the original identity file and system credential, review the original browser request and retry the same command")
	}
	return 0
}
