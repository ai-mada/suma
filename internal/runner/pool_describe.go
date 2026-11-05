package runner

import (
	"context"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type PoolToolDescription struct {
	TenantID  string           `json:"tenant_id"`
	ProjectID string           `json:"project_id"`
	Items     []tc.Observation `json:"items"`
}

// DescribePoolTools uses the same resolved per-project configuration as actual
// execution, including the native agent's fixed project environment. It never
// authenticates, calls a model or exposes configured paths/environment values.
func DescribePoolTools(ctx context.Context, config PoolLocalConfig, allow bool) ([]PoolToolDescription, error) {
	resolved, err := resolvedPoolConfig(config)
	if err != nil {
		return nil, err
	}
	out := make([]PoolToolDescription, 0, len(resolved.Scopes))
	for _, scope := range resolved.Scopes {
		items, err := describeTools(ctx, ExecConfig{AllowExec: allow, Workspace: scope.Workspace, Repository: scope.Repository, BaseCommit: scope.BaseCommit, WorktreeRoot: scope.Worktrees, Commands: scope.Commands, ModelDeployments: scope.ModelDeployments}, true)
		if err != nil {
			return nil, err
		}
		out = append(out, PoolToolDescription{TenantID: scope.TenantID, ProjectID: scope.ProjectID, Items: items})
	}
	return out, nil
}
