package platform

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
)

type schedulingToolProfile struct {
	profile           ToolProfile
	activationChecked bool
	activationRevoked bool
}

// decorateToolAdmissionTx reports configuration and activation facts only.
// A valid configuration still requires the worker and inventory checks at claim.
func (s *Service) decorateToolAdmissionTx(ctx context.Context, tx *sql.Tx, tenant, project string, out *dag.NodeAdmissionSnapshot, profiles map[string]schedulingToolProfile) error {
	binding := out.ToolBinding
	if binding == nil || out.NodeState != "ready" && out.NodeState != "pending" {
		return nil
	}
	fact, loaded := profiles[binding.ProfileID]
	if !loaded {
		profile, err := s.toolProfileTx(ctx, tx, Principal{TenantID: tenant}, project, binding.ProfileID)
		if err != nil {
			var api *APIError
			if !errors.As(err, &api) || api.Status != 404 {
				return err
			}
		} else if profile.ID == binding.ProfileID && profile.ProjectID == project {
			fact.profile = profile
		}
	}

	var code, message string
	switch {
	case fact.profile.ID == "":
		code, message = "tool_profile_missing", "找不到此运行固定的工具配置"
	case fact.profile.Profile.ProfileSHA256 != binding.ProfileSHA256 || fact.profile.Profile.CommandID != binding.CommandID:
		code, message = "tool_profile_changed", "当前工具配置与运行固定的指纹或命令不一致"
	case fact.profile.State == "paused":
		code, message = "tool_profile_paused", "固定工具配置已暂停，等待重新核对启用"
	case fact.profile.State == "revoked":
		code, message = "tool_profile_revoked", "固定工具配置已撤销，需核对运行配置"
	case fact.profile.State == "enabled":
		if !fact.activationChecked {
			err := s.toolActivationLockedTx(ctx, tx, tenant, project, fact.profile)
			if err != nil && !automationAuthorityFailure(err) {
				return err
			}
			fact.activationChecked = true
			fact.activationRevoked = err != nil
		}
		if fact.activationRevoked {
			code, message = "tool_activation_revoked", "固定工具配置的启用人已失去审批权限"
		}
	default:
		code, message = "tool_profile_changed", "当前工具配置的状态无法核验"
	}
	// Reuse the authorized read for repeated profiles within this queue page.
	// Each node still compares its own frozen fingerprint and command.
	if profiles != nil {
		profiles[binding.ProfileID] = fact
	}
	if code != "" {
		out.Blockers = append(out.Blockers, dag.SchedulingBlocker{Code: code, Message: message, Source: "tool", RunID: out.RunID, NodeID: out.NodeID})
		out.Status = "blocked"
	}
	return nil
}
