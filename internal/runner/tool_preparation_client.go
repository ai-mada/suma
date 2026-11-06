package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/ai-mada/suma/internal/strictjson"
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

func preparationReference(op tp.Operation) tp.Reference {
	return tp.Reference{PreparationID: op.ID, ClaimID: op.ClaimID, Fence: op.Fence}
}
func samePreparationSource(a, b tp.Operation) bool {
	return preparationReference(a) == preparationReference(b) && a.SourceSHA256 == b.SourceSHA256 && a.TenantID == b.TenantID && a.ProjectID == b.ProjectID && a.WorkerID == b.WorkerID && a.InstanceID == b.InstanceID
}

func (c *Client) claimToolPreparation(ctx context.Context, worker, nonce string) (tp.Claim, error) {
	var reply tp.Claim
	if !ii.Operation(nonce) {
		return reply, fmt.Errorf("invalid managed preparation claim")
	}
	payload, err := c.installationPayload(worker)
	if err != nil {
		return reply, err
	}
	payload["claim_id"] = nonce
	var raw json.RawMessage
	if err = c.post(ctx, "tool-preparations/claim", payload, &raw); err != nil {
		return reply, err
	}
	var fields map[string]json.RawMessage
	if strictjson.Decode(raw, &fields, 256<<10) != nil || len(fields) != 4 ||
		json.Unmarshal(fields["tenant_id"], &reply.TenantID) != nil ||
		json.Unmarshal(fields["project_id"], &reply.ProjectID) != nil ||
		json.Unmarshal(fields["worker_id"], &reply.WorkerID) != nil ||
		!ii.ID(reply.TenantID) || reply.ProjectID != c.ProjectID || reply.WorkerID != worker {
		return reply, fmt.Errorf("managed preparation canonical claim differs")
	}
	if c.Pool != nil && reply.TenantID != c.Pool.TenantID {
		return reply, fmt.Errorf("managed preparation tenant differs")
	}
	// Only the claim payload is nullable. Its explicit null is a confirmed
	// empty receipt, so the slot can finish this nonce and ask for new work.
	value, ok := fields["preparation"]
	if !ok {
		return reply, fmt.Errorf("incomplete managed preparation claim")
	}
	if string(value) != "null" {
		var op tp.Operation
		if strictjson.Decode(value, &op, 256<<10) != nil || tp.ValidateOperation(op) != nil ||
			op.TenantID != reply.TenantID || op.ProjectID != reply.ProjectID || op.WorkerID != worker || op.ClaimID != nonce || op.Fence < 1 {
			return reply, fmt.Errorf("managed preparation claim source differs")
		}
		reply.Preparation = &op
	}
	return reply, nil
}
func (c *Client) resultToolPreparation(ctx context.Context, source tp.Operation, request tp.ResultRequest) (tp.Operation, error) {
	var head tp.Operation
	if tp.ValidateResultRequest(request) != nil || request.Preparation != preparationReference(source) || request.SourceSHA256 != source.SourceSHA256 || request.ProjectID != c.ProjectID || request.WorkerID != source.WorkerID {
		return head, fmt.Errorf("invalid original managed preparation result")
	}
	payload, err := c.installationPayload(request.WorkerID)
	if err != nil {
		return head, err
	}
	payload["preparation"], payload["source_sha256"], payload["idempotency_key"], payload["outcome"], payload["failure_code"] = request.Preparation, request.SourceSHA256, request.IdempotencyKey, request.Outcome, request.FailureCode
	if request.Receipt != nil {
		payload["receipt"] = request.Receipt
	}
	if request.StopProof != nil {
		payload["stop_proof"] = request.StopProof
	}
	var raw json.RawMessage
	if err = c.post(ctx, "tool-preparations/results", payload, &raw); err != nil {
		return head, err
	}
	if strictjson.Decode(raw, &head, 256<<10) != nil || tp.ValidateOperation(head) != nil || !samePreparationSource(source, head) {
		return head, fmt.Errorf("managed preparation result acknowledgement source differs")
	}
	state, code := request.Outcome, request.FailureCode
	if state == "prepared" {
		state = "awaiting_inventory"
		code = "prepared"
	}
	if head.State != state || head.ResultCode != code || !reflect.DeepEqual(head.Receipt, request.Receipt) || !reflect.DeepEqual(head.StopProof, request.StopProof) {
		return head, fmt.Errorf("managed preparation result acknowledgement differs")
	}
	return head, nil
}
