package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// WorkspaceMetadata intentionally contains no host path or recursive inventory.
// A configured binding grants neither a live execution connection nor write
// authority. The directory reader applies its own current read checks.
type WorkspaceMetadata struct {
	ProjectID               string `json:"project_id"`
	Configured              bool   `json:"configured"`
	BindingAvailable        bool   `json:"binding_available"`
	BindingSHA256           string `json:"binding_sha256"`
	BindingPathSHA256       string `json:"binding_path_sha256"`
	BindingOperationID      string `json:"binding_operation_id"`
	DesktopBindingAvailable bool   `json:"desktop_binding_available"`
	Name                    string `json:"name,omitempty"`
	Reason                  string `json:"reason,omitempty"`
	ReasonMessageKey        string `json:"reason_message_key,omitempty"`
}

func desktopWorkspaceReasonKey(err error) string {
	var api *APIError
	if !errors.As(err, &api) {
		return ""
	}
	switch api.Code {
	case "desktop_workspace_unavailable":
		return "desktop_workspace.unavailable"
	case "desktop_workspace_invalid":
		return "desktop_workspace.invalid"
	case "desktop_workspace_grant_invalid":
		return "desktop_workspace.grant_invalid"
	case "desktop_workspace_grant_expired":
		return "desktop_workspace.grant_expired"
	case "desktop_workspace_binding_changed":
		return "desktop_workspace.binding_changed"
	case "desktop_workspace_operation_conflict":
		return "desktop_workspace.operation_conflict"
	case "desktop_workspace_root_invalid":
		return "desktop_workspace.root_invalid"
	case "desktop_workspace_root_changed":
		return "desktop_workspace.root_changed"
	case "desktop_workspace_root_unavailable":
		return "desktop_workspace.root_unavailable"
	}
	return ""
}

func (s *Service) workspaceBindingMetadata(project string, state workspaceBindingState) WorkspaceMetadata {
	out := WorkspaceMetadata{ProjectID: project, BindingAvailable: strings.TrimSpace(os.Getenv("SUMA_WORKSPACE_ROOT")) != "", DesktopBindingAvailable: s.desktopWorkspaceAvailable(), BindingSHA256: state.sha256}
	if state.root != "" {
		out.BindingPathSHA256 = digest(state.root)
		if state.proof != nil {
			out.BindingOperationID = state.proof.OperationID
		}
	}
	return out
}

func (s *Service) GetWorkspaceMetadata(ctx context.Context, actor Actor, project string) (WorkspaceMetadata, error) {
	out := s.workspaceBindingMetadata(project, workspaceBindingState{})
	err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		state, err := s.workspaceBindingStateTx(ctx, auth.tx, auth.currentPrincipal.TenantID, project)
		if err != nil {
			return err
		}
		out = s.workspaceBindingMetadata(project, state)
		// A broken native proof is a binding fact, never healthy configuration.
		// Keep its CAS and ACK fields so an authorized user can explicitly choose
		// a replacement; authorization failures still propagate from this guard.
		if err = s.validateWorkspaceBindingRoot(ctx, auth.currentPrincipal.TenantID, project, state); err != nil {
			out.Reason = err.Error()
			out.ReasonMessageKey = desktopWorkspaceReasonKey(err)
			return ctx.Err()
		}
		out.Configured, out.Name = true, filepath.Base(state.root)
		return nil
	})
	return out, err
}
