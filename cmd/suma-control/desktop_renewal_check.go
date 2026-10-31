package main

import (
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
	"github.com/ai-mada/suma/internal/strictjson"
)

// The desktop alone supplies this state directory. Inspection precedes service
// initialization and must neither open a database nor resume the Runner.
func runDesktopRunnerRenewalCheck(app *commandlocale.Command, input io.Reader) int {
	const limit = 16 << 10
	var request struct {
		StateDirectory string `json:"state_directory"`
	}
	raw, err := io.ReadAll(io.LimitReader(input, limit+1))
	if err != nil || len(raw) > limit || strictjson.Decode(raw, &request, limit) != nil || !filepath.IsAbs(request.StateDirectory) || len(request.StateDirectory) > 4096 || strings.ContainsFunc(request.StateDirectory, unicode.IsControl) {
		return app.Failure("续期检查需要有效且有界的私有状态目录", "Renewal inspection needs a valid bounded private state directory")
	}
	result := runner.InspectDesktopRunnerRenewalState(request.StateDirectory)
	if json.NewEncoder(app.Out).Encode(result) != nil {
		return app.Failure("无法输出原执行状态核对结果", "Could not write the original execution-state inspection")
	}
	return 0
}
