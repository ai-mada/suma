package main

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/ai-mada/suma/internal/commandlocale"
	rc "github.com/ai-mada/suma/internal/desktoprecoverycontract"
	"github.com/ai-mada/suma/internal/runner"
	"github.com/ai-mada/suma/internal/strictjson"
)

// This private offline entry runs before database/service initialization. The
// desktop supplies the original directory and proof key through stdin only;
// neither a transport client nor a normal Worker is constructed here.
func runDesktopResultRecovery(app *commandlocale.Command, input io.Reader) int {
	var request struct {
		Action         string    `json:"action"`
		StateDirectory string    `json:"state_directory"`
		ProjectID      string    `json:"project_id"`
		WorkerID       string    `json:"worker_id"`
		Plan           *rc.Plan  `json:"plan,omitempty"`
		Proof          *rc.Proof `json:"proof,omitempty"`
		ProofKey       string    `json:"proof_key,omitempty"`
	}
	raw, err := io.ReadAll(io.LimitReader(input, rc.InputLimit+1))
	invalid := func() int {
		return app.Failure("结果核对需要有效且有界的私有输入", "Result recovery needs valid bounded private input")
	}
	if err != nil || len(raw) > rc.InputLimit || strictjson.Decode(raw, &request, rc.InputLimit) != nil || !filepath.IsAbs(request.StateDirectory) || len(request.StateDirectory) > 4096 || strings.ContainsFunc(request.StateDirectory, unicode.IsControl) {
		return invalid()
	}
	identity := rc.Plan{Protocol: rc.PlanProtocol, ProjectID: request.ProjectID, WorkerID: request.WorkerID, SnapshotSHA256: strings.Repeat("0", 64), Items: []rc.Item{}}
	if rc.ValidatePlan(identity) != nil {
		return invalid()
	}
	var proof rc.Proof
	var key []byte
	if request.Proof != nil || request.ProofKey != "" {
		key, err = hex.DecodeString(request.ProofKey)
		if request.Proof == nil || err != nil || len(key) != 32 || hex.EncodeToString(key) != request.ProofKey {
			return invalid()
		}
		proof = *request.Proof
	}
	var result rc.Result
	switch request.Action {
	case "prepare":
		if request.Plan != nil || request.Proof != nil || request.ProofKey != "" {
			return invalid()
		}
		result = runner.PrepareDesktopResultRecovery(request.StateDirectory, request.ProjectID, request.WorkerID)
	case "apply", "status":
		if request.Plan == nil || rc.ValidatePlan(*request.Plan) != nil || request.Plan.ProjectID != request.ProjectID || request.Plan.WorkerID != request.WorkerID || request.Action == "apply" && request.Proof == nil {
			return invalid()
		}
		if request.Action == "apply" {
			result = runner.ApplyDesktopResultRecovery(request.StateDirectory, request.ProjectID, request.WorkerID, *request.Plan, proof, key, time.Now().UTC())
		} else {
			result = runner.InspectDesktopResultRecovery(request.StateDirectory, request.ProjectID, request.WorkerID, *request.Plan, proof, key, time.Now().UTC())
		}
	default:
		return invalid()
	}
	if json.NewEncoder(app.Out).Encode(result) != nil {
		return app.Failure("无法输出原结果核对状态", "Could not write original result-recovery status")
	}
	return 0
}
