package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ai-mada/suma/internal/strictjson"
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
)

func (c *Client) installationPayload(worker string) (map[string]any, error) {
	if !ii.ID(c.ProjectID) || !ii.ID(worker) {
		return nil, fmt.Errorf("invalid installation machine scope")
	}
	if c.Pool != nil && (c.Pool.Assignment != nil || c.Pool.ProjectID != c.ProjectID || c.Pool.TenantID == "") {
		return nil, fmt.Errorf("installation scan requires exact project machine scope")
	}
	p := map[string]any{"project_id": c.ProjectID}
	if c.Pool == nil {
		p["worker_id"] = worker
	}
	return p, nil
}
func (c *Client) claimInstallationScan(ctx context.Context, worker, claim string) (*ii.Scan, error) {
	if ii.ValidateClaimRequest(ii.ClaimRequest{ProjectID: c.ProjectID, WorkerID: worker, ClaimID: claim}) != nil {
		return nil, fmt.Errorf("invalid installation claim")
	}
	p, err := c.installationPayload(worker)
	if err != nil {
		return nil, err
	}
	p["claim_id"] = claim
	var body json.RawMessage
	if err = c.post(ctx, "tool-installation-scans/claim", p, &body); err != nil {
		return nil, err
	}
	if len(body) > 256<<10 {
		return nil, fmt.Errorf("installation claim receipt exceeds its bound")
	}
	if _, err = decodeToolObject(body); err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil {
		return nil, fmt.Errorf("invalid installation claim receipt")
	}
	value, ok := raw["scan"]
	if !ok || len(raw) != 1 {
		return nil, fmt.Errorf("missing installation claim receipt")
	}
	if string(value) == "null" {
		return nil, nil
	}
	var scan ii.Scan
	if strictjson.Decode(value, &scan, 256<<10) != nil || ii.ValidateScan(scan) != nil || scan.ProjectID != c.ProjectID || scan.WorkerID != worker || scan.ClaimID != claim || scan.Fence < 1 {
		return nil, fmt.Errorf("installation claim source differs")
	}
	return &scan, nil
}
func (c *Client) resultInstallationScan(ctx context.Context, source ii.Scan, request ii.ResultRequest) (ii.Scan, error) {
	var head ii.Scan
	if ii.ValidateResultRequest(request) != nil || request.Scan != scanReference(source) || request.ProjectID != c.ProjectID {
		return head, fmt.Errorf("invalid original installation result")
	}
	p, err := c.installationPayload(request.WorkerID)
	if err != nil {
		return head, err
	}
	p["scan"], p["idempotency_key"], p["outcome"], p["items"], p["failure_code"] = request.Scan, request.IdempotencyKey, request.Outcome, request.Items, request.FailureCode
	if request.StopProof != nil {
		p["stop_proof"] = request.StopProof
	}
	var raw json.RawMessage
	if err = c.post(ctx, "tool-installation-scans/results", p, &raw); err != nil {
		return head, err
	}
	if strictjson.Decode(raw, &head, 256<<10) != nil || ii.ValidateScan(head) != nil || !sameInstallationScan(source, head) || head.State != request.Outcome {
		return head, fmt.Errorf("installation result acknowledgement differs")
	}
	if request.Outcome != "unknown" {
		expected := ii.Result{Items: request.Items, StopProof: request.StopProof}
		a, _ := json.Marshal(expected)
		b, _ := json.Marshal(head.Result)
		if string(a) != string(b) {
			return head, fmt.Errorf("installation result proof differs")
		}
	}
	if request.Outcome != "completed" && head.ResultCode != request.FailureCode {
		return head, fmt.Errorf("installation unknown reason differs")
	}
	return head, nil
}
