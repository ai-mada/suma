package platform

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/ai-mada/suma/internal/dag"
	nwc "github.com/ai-mada/suma/internal/nativeworkspacecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	wo "github.com/ai-mada/suma/internal/workspaceoperationcontract"
)

// A host request is a distinct, closed envelope. An SDK ToolEvent or message
// cannot claim host provenance by adding fields to its activity payload.
type HostOperationProgressRequest struct {
	ProjectID     string         `json:"project_id"`
	Assignment    dag.Assignment `json:"assignment"`
	OperationID   string         `json:"operation_id"`
	HostOperation wo.Event       `json:"host_operation"`
}

type PoolHostOperationProgressRequest struct {
	PoolRunnerAssignmentRequest
	OperationID   string   `json:"operation_id"`
	HostOperation wo.Event `json:"host_operation"`
}

func validateHostOperationProfile(n dag.Node, profile ToolProfile, a dag.Assignment, event wo.Event) error {
	id, sha, bound, err := dag.ToolBinding(n)
	if err != nil || !bound || profile.ID != id || profile.WorkerID != a.WorkerID || profile.Profile.ProfileSHA256 != sha || event.ProfileSHA256 != sha || len(n.Command) != 1 || profile.Profile.CommandID != n.Command[0] || profile.Profile.ReadOnly || profile.Profile.WorkspaceToolsVersion != nwc.Version {
		return forbidden()
	}
	switch profile.Profile.Adapter {
	case "suma", "octop", "opencode":
	default:
		return forbidden()
	}
	if tc.ValidateProfile(profile.Profile, true) != nil {
		return forbidden()
	}
	return nil
}

func (s *Service) validateHostOperationProgressTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, a dag.Assignment, event wo.Event) error {
	if err := assertRunnerCommand(tx, auth); err != nil {
		return err
	}
	if a.TenantID != auth.target.tenantID || a.ProjectID != auth.target.projectID || wo.ValidateBoundEvent(dag.WorkspaceOperationIdentity(a.TenantID, a.ProjectID, a), event) != nil {
		return forbidden()
	}
	n, profile, err := s.toolBoundNodeTx(ctx, tx, auth.currentPrincipal, auth.target.projectID, a.RunID, a.NodeID)
	if err != nil {
		return err
	}
	return validateHostOperationProfile(n, profile, a, event)
}

func (s *Service) ValidatePoolHostOperationProgressTx(ctx context.Context, tx *sql.Tx, proof *poolMachineProof, a dag.Assignment, event wo.Event) error {
	if err := proof.assertAssignment(tx, a); err != nil {
		return err
	}
	if wo.ValidateBoundEvent(dag.WorkspaceOperationIdentity(a.TenantID, a.ProjectID, a), event) != nil {
		return forbidden()
	}
	n, profile, err := s.poolBoundToolNodeTx(ctx, tx, dag.PoolScope{TenantID: a.TenantID, ProjectID: a.ProjectID}, a.RunID, a.NodeID)
	if err != nil {
		return err
	}
	return validateHostOperationProfile(n, profile, a, event)
}

func decodeHostOperationProgressEnvelope(body map[string]any) (HostOperationProgressRequest, error) {
	var request HostOperationProgressRequest
	if len(body) != 4 {
		return request, invalid("Invalid Go workspace operation progress")
	}
	for _, key := range []string{"project_id", "assignment", "operation_id", "host_operation"} {
		if _, found := body[key]; !found {
			return request, invalid("Invalid Go workspace operation progress")
		}
	}
	if err := strictAutomation(body, &request); err != nil {
		return request, invalid("Invalid Go workspace operation progress")
	}
	if request.OperationID != request.HostOperation.EventID || request.ProjectID != request.Assignment.ProjectID {
		return request, invalid("Invalid Go workspace operation progress")
	}
	return request, nil
}

// Keep the nested raw JSON until the dedicated closed event decoder has checked
// duplicates and Unicode. Re-encoding a generic map would erase that evidence.
func preserveHostOperationJSON(raw []byte, body map[string]any) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if event, found := fields["host_operation"]; found {
		if err := RejectDuplicateReleaseJSON(raw); err != nil {
			return err
		}
		body["host_operation"] = event
	}
	return nil
}
