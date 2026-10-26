package dag

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
)

// ManagedRunRequest identifies one independent root and its service-built flow.
type ManagedRunRequest struct {
	RunID    string   `json:"run_id"`
	Workflow Workflow `json:"workflow"`
}

func managedBatchInvalid() error {
	return dagDiagnostic(fmt.Errorf("%w: managed batch requires 1..16 unique reserved IDs, serializable workflows and authority hooks", ErrInvalid), "dag.run.managed_batch_request", nil)
}

func managedBatchReceiptInvalid() error {
	return dagDiagnostic(fmt.Errorf("%w: managed batch receipt does not match its original runs", ErrConflict), "dag.run.managed_batch_receipt", nil)
}

// Each existing operation row stores one original run ID. Fixed item scopes and
// the same ordered fingerprint avoid a new receipt table or a JSON list in the
// existing bounded result_id column. Follow the single-run route lock ordering.
func (t *transaction) managedBatchOperation(tenant, project, scope, operation, hash string) (string, error) {
	var prior string
	err := t.row(`SELECT result_id FROM dag_operations WHERE tenant_id=? AND project_id=? AND scope=? AND operation_id=?`, tenant, project, scope, operation).Scan(&prior)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	if err = t.guardExecution(tenant, project, prior); err != nil {
		return "", err
	}
	return t.operation(tenant, project, scope, operation, hash)
}

// CreateManagedRunsBound commits one project batch, every domain binding and
// its final receipt together. Hooks use tx only and never call Engine/network.
// before owns current organization authority locks; scheduler route/project/
// family locks follow it, matching CreateManagedRunBound's lock ordering.
// Replay checks current before authority but skips every bind and commit hook.
func (e *Engine) CreateManagedRunsBound(ctx context.Context, tenant, project, operationID string, items []ManagedRunRequest, before func(*sql.Tx) error, bind func(*sql.Tx, string) error, commit func(*sql.Tx, []Run) error) ([]Run, error) {
	if err := scopeValid(tenant, project); err != nil {
		return nil, err
	}
	if len(items) < 1 || len(items) > 16 || before == nil || bind == nil || commit == nil {
		return nil, managedBatchInvalid()
	}
	if operationID == "" || len(operationID) > 150 {
		return nil, dagDiagnostic(fmt.Errorf("%w: operation_id required (max 150)", ErrInvalid), "dag.operation.id", nil)
	}
	// Freeze caller-owned maps before any hook can mutate another item's input.
	raw, err := json.Marshal(items)
	var prepared []ManagedRunRequest
	if err != nil || json.Unmarshal(raw, &prepared) != nil {
		return nil, managedBatchInvalid()
	}
	seen, nodes := map[string]bool{}, 0
	for i := range prepared {
		item := &prepared[i]
		if !identifier.MatchString(item.RunID) || len(item.RunID) > 92 || seen[item.RunID] {
			return nil, managedBatchInvalid()
		}
		seen[item.RunID] = true
		item.Workflow.ID, item.Workflow.Version = "managed-"+item.RunID, 1
		item.Workflow, err = normalize(item.Workflow)
		if err != nil {
			return nil, err
		}
		nodes += len(item.Workflow.Nodes)
		if nodes > 10000 {
			return nil, dagDiagnostic(fmt.Errorf("%w: managed batch requires at most 10000 top-level nodes", ErrInvalid), "dag.run.managed_batch_nodes", nil)
		}
	}
	hash := fingerprint(prepared)
	scope := func(i int) string { return "managed_runs:" + strconv.Itoa(i) }
	var out []Run
	err = e.writeBefore(ctx, before, func(t *transaction) error {
		originalRoot := t.familyRoot
		defer func() { t.familyRoot = originalRoot }()
		previous, found := make([]string, len(prepared)), 0
		for i := range prepared {
			var err error
			previous[i], err = t.managedBatchOperation(tenant, project, scope(i), operationID, hash)
			if err != nil {
				return err
			}
			if previous[i] != "" {
				found++
			}
		}
		if found > 0 {
			if found != len(prepared) {
				return managedBatchReceiptInvalid()
			}
			for i, id := range previous {
				if id != prepared[i].RunID {
					return managedBatchReceiptInvalid()
				}
				r, err := load(ctx, t.tx, e, tenant, project, id)
				if err != nil {
					return err
				}
				out = append(out, r.public())
			}
			return nil
		}
		for _, item := range prepared {
			if err := t.validateReferences(tenant, project, item.Workflow); err != nil {
				return err
			}
		}
		for _, item := range prepared {
			// A batch item is a new independent root, unlike a true subworkflow.
			t.familyRoot = ""
			w := item.Workflow
			if err := t.exec(`INSERT INTO dag_workflows(tenant_id,project_id,id,version,name,spec)VALUES(?,?,?,?,?,?)`, tenant, project, w.ID, 1, w.Name, encoded(w)); err != nil {
				return err
			}
			if err := t.exec(`INSERT INTO dag_managed_workflows(tenant_id,project_id,workflow_id,run_id)VALUES(?,?,?,?)`, tenant, project, w.ID, item.RunID); err != nil {
				return err
			}
			r, err := t.createRunID(tenant, project, item.RunID, w, bind)
			if err != nil {
				return err
			}
			out = append(out, r.public())
		}
		t.familyRoot = originalRoot
		var commitRuns []Run
		if err := json.Unmarshal([]byte(encoded(out)), &commitRuns); err != nil {
			return err
		}
		if err := commit(t.tx, commitRuns); err != nil {
			return err
		}
		for i, item := range prepared {
			if err := t.remember(tenant, project, scope(i), operationID, hash, item.RunID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
