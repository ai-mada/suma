package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

// Plain fixed command/test flows may still attach tasks. Agent flows require
// the explicit preview which freezes actual business context into their input.
func (s *Service) BindDirectRunTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project, taskID, runID string, expected int) error {
	if err := assertProjectCommand(tx, auth, project); err != nil {
		return err
	}
	p := auth.currentPrincipal
	if taskID == "" {
		return nil
	}
	if err := s.rejectReservedCandidateRepairTaskTx(ctx, tx, p.TenantID, project, taskID, ""); err != nil {
		return err
	}
	var raw string
	err := tx.QueryRowContext(ctx, s.q(`SELECT w.spec FROM dag_runs r JOIN dag_workflows w ON w.tenant_id=r.tenant_id AND w.project_id=r.project_id AND w.id=r.workflow_id AND w.version=r.workflow_version WHERE r.tenant_id=? AND r.project_id=? AND r.id=?`), p.TenantID, project, runID).Scan(&raw)
	if err != nil {
		return err
	}
	var w dag.Workflow
	if err = json.Unmarshal([]byte(raw), &w); err != nil {
		return err
	}
	for _, node := range w.Nodes {
		basis, e := rc.DecodeApplicationBuild(node.Config)
		if e != nil {
			return invalid("固定应用构建配置无效")
		}
		if e = s.checkApplicationBuildBasisTx(ctx, tx, p, project, basis); e != nil {
			return e
		}
	}
	agent, err := s.taskWorkflowHasAgentTx(ctx, tx, p, project, w, map[string]bool{}, 0)
	if err != nil {
		return err
	}
	if agent {
		return invalid("智能体任务必须先安排工具执行并核对固定任务上下文；直接关联运行不会发送任务内容")
	}
	return s.BindRunTx(ctx, tx, auth, project, taskID, runID, expected)
}

func (s *Service) taskWorkflowHasAgentTx(ctx context.Context, tx *sql.Tx, p Principal, project string, w dag.Workflow, seen map[string]bool, depth int) (bool, error) {
	if depth > 32 {
		return false, invalid("任务流程嵌套过深，无法完整核对派发依据")
	}
	for _, n := range w.Nodes {
		switch n.Type {
		case "agent":
			return true, nil
		case "map":
			raw, err := json.Marshal(n.Config["template"])
			if err != nil {
				return false, err
			}
			var template dag.Node
			if err = json.Unmarshal(raw, &template); err != nil {
				return false, err
			}
			if yes, err := s.taskWorkflowHasAgentTx(ctx, tx, p, project, dag.Workflow{Nodes: []dag.Node{template}}, seen, depth+1); err != nil || yes {
				return yes, err
			}
		case "subworkflow":
			id, _ := n.Config["workflow_id"].(string)
			version := number(n.Config, "workflow_version")
			key := fmt.Sprintf("%s@%d", id, version)
			if seen[key] {
				continue
			}
			seen[key] = true
			var raw string
			if err := tx.QueryRowContext(ctx, s.q(`SELECT spec FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), p.TenantID, project, id, version).Scan(&raw); err != nil {
				return false, err
			}
			var child dag.Workflow
			if err := json.Unmarshal([]byte(raw), &child); err != nil {
				return false, err
			}
			if yes, err := s.taskWorkflowHasAgentTx(ctx, tx, p, project, child, seen, depth+1); err != nil || yes {
				return yes, err
			}
		}
	}
	return false, nil
}
