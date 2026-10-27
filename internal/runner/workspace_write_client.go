package runner

import (
	"context"
	"encoding/json"
	"fmt"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
	wwc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

type workspaceWriteClaimReply struct {
	Lease *wwc.Lease `json:"lease"`
}

func (r *workspaceWriteClaimReply) UnmarshalJSON(raw []byte) error {
	type plain workspaceWriteClaimReply
	var result plain
	if wwc.Decode(raw, &result) != nil {
		return fmt.Errorf("invalid workspace write lease fields")
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	if _, ok := fields["lease"]; !ok {
		return fmt.Errorf("workspace write lease field is absent")
	}
	*r = workspaceWriteClaimReply(result)
	return nil
}

type workspaceWriteReceiptReply wwc.Receipt

func (r *workspaceWriteReceiptReply) UnmarshalJSON(raw []byte) error {
	type plain workspaceWriteReceiptReply
	var result plain
	if wwc.Decode(raw, &result) != nil {
		return fmt.Errorf("invalid workspace write receipt fields")
	}
	*r = workspaceWriteReceiptReply(result)
	return nil
}
func (c *Client) writeScopeMatches(project string, scope wwc.Scope) bool {
	if project != c.ProjectID || woc.ValidateScope(scope) != nil {
		return false
	}
	if c.Pool == nil {
		return true
	}
	a := c.Pool.Assignment
	return a != nil && writeAssignmentMatchesScope(*a, scope)
}
func (c *Client) ClaimWorkspaceWrite(ctx context.Context, request wwc.ClaimRequest) (*wwc.Lease, error) {
	if !c.writeScopeMatches(request.ProjectID, request.Scope) || request.WorkerID != request.Scope.WorkerID || !woc.ValidDigest(request.WorkspacePolicySHA256) || !nativeOperationID(request.OperationID) {
		return nil, fmt.Errorf("workspace write claim scope is invalid")
	}
	var response workspaceWriteClaimReply
	if err := c.post(ctx, "workspace-writes/claim", request, &response); err != nil {
		return nil, err
	}
	if lease := response.Lease; lease != nil {
		if validateWorkspaceWriteLease(*lease, request.ProjectID, request.Scope, request.WorkspacePolicySHA256) != nil {
			return nil, fmt.Errorf("workspace write lease differs from its exact claim")
		}
	}
	return response.Lease, nil
}
func (c *Client) CompleteWorkspaceWrite(ctx context.Context, result wwc.ResultRequest) (wwc.Receipt, error) {
	if !c.writeScopeMatches(result.ProjectID, result.Scope) || result.WorkerID != result.Scope.WorkerID || result.CommandID == "" || result.ClaimID == "" || result.ExecutionFence != 1 || !nativeOperationID(result.OperationID) || !woc.ValidDigest(result.WorkspacePolicySHA256) || !woc.ValidDigest(result.RequestSHA256) || result.ResultSHA256 != wwc.ResultDigest(result.Outcome, result.Effect, result.ErrorCode) {
		return wwc.Receipt{}, fmt.Errorf("workspace write result scope is invalid")
	}
	var response workspaceWriteReceiptReply
	if err := c.post(ctx, "workspace-writes/result", result, &response); err != nil {
		return wwc.Receipt{}, err
	}
	receipt := wwc.Receipt(response)
	if !receipt.Acknowledged || receipt.CommandID != result.CommandID || receipt.ClaimID != result.ClaimID || receipt.ResultSHA256 != result.ResultSHA256 {
		return wwc.Receipt{}, fmt.Errorf("workspace write acknowledgement differs from its exact result")
	}
	return receipt, nil
}
