package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

func strictWorkspaceObservationJSON(raw []byte, out any) error {
	if woc.Decode(raw, out) != nil {
		return fmt.Errorf("invalid workspace observation response fields")
	}
	return nil
}

type workspaceObservationClaimReply struct {
	Lease *woc.Lease `json:"lease"`
}

func (r *workspaceObservationClaimReply) UnmarshalJSON(raw []byte) error {
	type plain workspaceObservationClaimReply
	var result plain
	if err := strictWorkspaceObservationJSON(raw, &result); err != nil {
		return err
	}
	var present map[string]json.RawMessage
	_ = json.Unmarshal(raw, &present)
	if _, ok := present["lease"]; !ok {
		return fmt.Errorf("workspace observation lease field is absent")
	}
	*r = workspaceObservationClaimReply(result)
	return nil
}

type workspaceObservationReceiptReply woc.Receipt

func (r *workspaceObservationReceiptReply) UnmarshalJSON(raw []byte) error {
	type plain workspaceObservationReceiptReply
	var result plain
	if err := strictWorkspaceObservationJSON(raw, &result); err != nil {
		return err
	}
	*r = workspaceObservationReceiptReply(result)
	return nil
}

func (c *Client) ClaimWorkspaceObservation(ctx context.Context, request woc.ClaimRequest) (*woc.Lease, error) {
	if request.ProjectID != c.ProjectID || request.WorkerID != request.Scope.WorkerID || request.OperationID == "" || woc.ValidateScope(request.Scope) != nil || !woc.ValidDigest(request.WorkspacePolicySHA256) {
		return nil, fmt.Errorf("workspace observation claim scope is invalid")
	}
	if c.Pool != nil {
		a := c.Pool.Assignment
		if a == nil || a.RunID != request.Scope.RunID || a.NodeID != request.Scope.NodeID || a.Attempt != request.Scope.Attempt || a.Fence != request.Scope.Fence || a.WorkerID != request.WorkerID {
			return nil, fmt.Errorf("workspace observation claim differs from the fixed Pool assignment")
		}
	}
	var response workspaceObservationClaimReply
	if err := c.post(ctx, "workspace-observations/claim", request, &response); err != nil {
		return nil, err
	}
	if lease := response.Lease; lease != nil {
		if validObservationLease(*lease, request.ProjectID, request.Scope, request.WorkspacePolicySHA256, time.Now()) != nil || lease.Observation.Result != nil || lease.Observation.ErrorCode != "" || lease.Observation.ObservedAt != "" {
			return nil, fmt.Errorf("workspace observation lease differs from its exact claim")
		}
	}
	return response.Lease, nil
}
func (c *Client) CompleteWorkspaceObservation(ctx context.Context, result woc.ResultRequest) (woc.Receipt, error) {
	var response workspaceObservationReceiptReply
	if result.ProjectID != c.ProjectID || result.WorkerID != result.Scope.WorkerID || result.RequestID == "" || result.LeaseFence < 1 || woc.ValidateScope(result.Scope) != nil || result.ResultSHA256 != woc.ResultDigest(result.ObservationResult, result.ErrorCode) {
		return woc.Receipt{}, fmt.Errorf("workspace observation result scope is invalid")
	}
	if c.Pool != nil {
		a := c.Pool.Assignment
		if a == nil || a.RunID != result.Scope.RunID || a.NodeID != result.Scope.NodeID || a.Attempt != result.Scope.Attempt || a.Fence != result.Scope.Fence || a.WorkerID != result.WorkerID {
			return woc.Receipt{}, fmt.Errorf("workspace observation result differs from the fixed Pool assignment")
		}
	}
	if err := c.post(ctx, "workspace-observations/result", result, &response); err != nil {
		return woc.Receipt{}, err
	}
	receipt := woc.Receipt(response)
	if !receipt.Acknowledged || receipt.RequestID != result.RequestID || receipt.LeaseFence != result.LeaseFence || receipt.ResultSHA256 != result.ResultSHA256 {
		return woc.Receipt{}, fmt.Errorf("workspace observation acknowledgement differs from its exact result")
	}
	return receipt, nil
}
