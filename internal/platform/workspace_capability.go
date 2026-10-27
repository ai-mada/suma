package platform

import (
	"context"
	"database/sql"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
	wcc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

const workspaceWriteTransportUnavailable = "Current worker has not declared remote workspace save support"

// The approved profile binds native tools. Transport versions are independent
// current inventory facts, not proof of a live handle or model authentication.
func supportsWorkspaceObservation(profile ToolProfile) bool {
	p, observation := profile.Profile, profile.Observation
	return profile.Available && observation != nil && observation.Available && tc.EqualProfile(p, observation.Profile) && p.Adapter == "suma" && p.Protocol == "suma-jsonl-v1" && !p.ReadOnly && p.WorkspaceToolsVersion == "attempt-workspace-v1" && observation.WorkspaceObservationProtocol == woc.Protocol
}

func supportsWorkspaceWrite(profile ToolProfile) bool {
	return supportsWorkspaceObservation(profile) && profile.Observation.WorkspaceWriteProtocol == wcc.Protocol
}

func (s *Service) requireWorkspaceWriteTransportTx(ctx context.Context, tx *sql.Tx, tenant, project string, basis workspaceObservationBasis) error {
	current, err := s.toolProfileObservationLockedTx(ctx, tx, tenant, project, basis.Profile)
	if err != nil {
		return err
	}
	if !supportsWorkspaceWrite(current) {
		return conflict(workspaceWriteTransportUnavailable)
	}
	return nil
}
