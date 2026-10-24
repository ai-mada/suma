package runner

import (
	"encoding/json"
	"fmt"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	"strconv"
)

// PoolContext belongs to a derived client, never to a mutable current project.
// Assignment is immutable for an execution; an inventory client instead fixes a
// single tenant/project scope before reporting local observations.
type PoolContext struct {
	PoolID, RunnerID    string
	TenantID, ProjectID string
	Assignment          *Assignment
}

type PoolCapabilityScope struct {
	TenantID     string   `json:"tenant_id"`
	ProjectID    string   `json:"project_id"`
	Capabilities []string `json:"capabilities"`
}

type PoolClaimRequest struct {
	PoolID      string                `json:"pool_id"`
	RunnerID    string                `json:"runner_id"`
	WorkerID    string                `json:"worker_id"`
	SlotID      string                `json:"slot_id"`
	OperationID string                `json:"operation_id"`
	Scopes      []PoolCapabilityScope `json:"scopes"`
}

// Only execution identity is repeated on callbacks: command inputs, prompts and
// local configuration must not be copied into every heartbeat and progress RPC.
func poolAssignmentIdentity(a Assignment) Assignment {
	return Assignment{TenantID: a.TenantID, ProjectID: a.ProjectID, PoolID: a.PoolID, PoolRunnerID: a.PoolRunnerID, PoolSlotID: a.PoolSlotID, PoolGrantID: a.PoolGrantID, PoolGrantVersion: a.PoolGrantVersion, PoolCredentialID: a.PoolCredentialID, PoolCredentialVersion: a.PoolCredentialVersion, RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, DispatchID: a.DispatchID, DispatchState: a.DispatchState, LeaseUntil: a.LeaseUntil}
}

func validPoolAssignment(a Assignment) error {
	if a.TenantID == "" || a.ProjectID == "" || a.PoolID == "" || a.PoolRunnerID == "" || a.PoolSlotID == "" || a.PoolGrantID == "" || a.PoolGrantVersion < 1 || a.PoolCredentialID == "" || a.PoolCredentialVersion < 1 {
		return fmt.Errorf("incomplete frozen pool assignment scope")
	}
	for _, id := range []string{a.TenantID, a.ProjectID, a.PoolID, a.PoolRunnerID, a.PoolGrantID, a.PoolCredentialID} {
		if !validPoolLocalID(id) {
			return fmt.Errorf("invalid frozen pool scope identifier")
		}
	}
	slot, err := strconv.Atoi(a.PoolSlotID)
	if err != nil || slot < 0 || slot >= 128 || strconv.Itoa(slot) != a.PoolSlotID {
		return fmt.Errorf("invalid frozen pool slot")
	}
	return validateDispatch(a, a.WorkerID)
}

func validPoolLocalID(id string) bool { return tc.ValidCommandID(id) }

func samePoolAssignmentIdentity(a, b Assignment) bool {
	x, _ := json.Marshal(poolAssignmentIdentity(a))
	y, _ := json.Marshal(poolAssignmentIdentity(b))
	return string(x) == string(y)
}

func (c *Client) poolPayload(raw []byte) ([]byte, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil || body == nil {
		return nil, fmt.Errorf("pool RPC requires an object")
	}
	set := func(key string, v any) { body[key], _ = json.Marshal(v) }
	set("runner_id", c.Pool.RunnerID)
	if a := c.Pool.Assignment; a != nil {
		if err := validPoolAssignment(*a); err != nil {
			return nil, err
		}
		if a.PoolID != c.Pool.PoolID || a.PoolRunnerID != c.Pool.RunnerID {
			return nil, fmt.Errorf("pool client identity changed")
		}
		if value := body["assignment"]; value != nil {
			var supplied Assignment
			if json.Unmarshal(value, &supplied) != nil || !samePoolAssignmentIdentity(supplied, *a) {
				return nil, fmt.Errorf("callback assignment differs from frozen pool scope")
			}
		}
		if value := body["result"]; value != nil {
			var result Result
			if json.Unmarshal(value, &result) != nil || result.RunID != a.RunID || result.NodeID != a.NodeID || result.Attempt != a.Attempt || result.Fence != a.Fence || result.WorkerID != a.WorkerID {
				return nil, fmt.Errorf("result differs from frozen pool attempt")
			}
		}
		set("assignment", poolAssignmentIdentity(*a))
		set("tenant_id", a.TenantID)
		set("project_id", a.ProjectID)
	} else if c.Pool.ProjectID != "" {
		set("tenant_id", c.Pool.TenantID)
		set("project_id", c.Pool.ProjectID)
	}
	return json.Marshal(body)
}
