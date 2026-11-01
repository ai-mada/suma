package platform

import (
	"context"

	"github.com/ai-mada/suma/internal/toolcontract"
)

// MatchToolProfiles reads the whole exact configuration set. Inventory freshness
// is a projection on its items, never a prerequisite for locating saved facts.
func (s *Service) MatchToolProfiles(ctx context.Context, actor Actor, project string, req ToolProfileMatchRequest) (ToolProfileMatchList, error) {
	out := ToolProfileMatchList{ProjectID: project, WorkerID: req.WorkerID, Profile: toolcontract.CloneProfile(req.Profile), Items: []ToolProfile{}}
	if req.ProjectID != project || !toolcontract.ValidCommandID(req.WorkerID) {
		return out, invalid("工具名称、执行节点或项目无效")
	}
	if err := toolcontract.ValidateProfile(req.Profile, true); err != nil {
		return out, invalidDiagnostic("validation.tool_profile_detail", "固定工具档案无效："+err.Error(), map[string]any{"detail": err.Error()})
	}
	err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		items, err := s.toolProfileExactMatchesTx(auth, project, req.WorkerID, req.Profile)
		if err != nil {
			return err
		}
		for i := range items {
			items[i], err = s.toolProfileObservationTx(auth.ctx, auth.tx, auth.currentPrincipal, project, items[i])
			if err != nil {
				return err
			}
		}
		out.Items, out.Total = items, len(items)
		return nil
	})
	return out, err
}

// Both callers hold the original current authorization/organization locks. This
// shared predicate keeps a read's identity and Create's race guard identical.
func (s *Service) toolProfileExactMatchesTx(auth *authorizedCommand, project, worker string, profile toolcontract.Profile) ([]ToolProfile, error) {
	out := []ToolProfile{}
	if err := assertProjectRead(auth, project); err != nil {
		return nil, err
	}
	rows, err := auth.tx.QueryContext(auth.ctx, s.q(`SELECT `+toolProfileColumns+` FROM suma_tool_profiles WHERE tenant_id=? AND project_id=? AND worker_id=? ORDER BY id LIMIT 1001`), auth.currentPrincipal.TenantID, project, worker)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
		if count > 1000 {
			return nil, conflict("项目工具档案已达 1000 项限制")
		}
		item, err := scanToolProfile(rows)
		if err != nil {
			return nil, err
		}
		if toolcontract.EqualProfile(item.Profile, profile) {
			out = append(out, item)
		}
	}
	return out, rows.Err()
}

func toolProfileMatchExists() error {
	return &diagnosticAPIError{cause: &APIError{409, "tool_profile_match_exists", "已存在相同工具档案，请读取并选择已有配置", nil}, key: "execution.tool_profile.match_exists"}
}
