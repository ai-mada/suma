package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
)

func probeNativeConflictRepair(ctx context.Context, executable string, spec CommandSpec, secrets []string) error {
	if spec.NativeWorkspace == nil || !spec.NativeWorkspace.ConflictRepair || spec.Adapter != "suma" {
		return fmt.Errorf("native conflict repair capability is not enabled")
	}
	text, err := probeCommand(ctx, executable, append(append([]string{}, spec.Args...), "--conflict-repair-probe"), spec, secrets)
	if err != nil {
		return fmt.Errorf("native runtime conflict repair capability probe failed")
	}
	fields, err := decodeToolObject([]byte(text))
	if err != nil || len(fields) != 5 {
		return fmt.Errorf("native runtime conflict repair capability is invalid")
	}
	var result struct {
		Protocol       string   `json:"protocol"`
		SourceLimit    int      `json:"source_limit"`
		WorkspaceTools []string `json:"workspace_tools"`
		MaxSteps       int      `json:"max_steps"`
		MaxTokens      int      `json:"max_tokens"`
	}
	if json.Unmarshal([]byte(text), &result) != nil || result.Protocol != ccc.CandidateConflictRepairProtocol || result.SourceLimit != 16 || result.MaxSteps != 128 || result.MaxTokens != 131072 {
		return fmt.Errorf("native runtime does not support the fixed conflict repair protocol")
	}
	expected := []string{"workspace.check", "workspace.edit", "workspace.list", "workspace.merge_continue", "workspace.merge_status", "workspace.read", "workspace.remove", "workspace.write"}
	sort.Strings(result.WorkspaceTools)
	if strings.Join(result.WorkspaceTools, ",") != strings.Join(expected, ",") {
		return fmt.Errorf("native runtime conflict repair tools do not match")
	}
	return nil
}
