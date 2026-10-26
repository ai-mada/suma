package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"github.com/ai-mada/suma/internal/dag"
)

type compositionAcceptanceScope struct{}
type compositionAcceptanceWalk struct {
	depth    int
	seen     map[string]bool
	visiting map[string]bool
	bindings int
	bases    map[string]RepositoryIntegrationBasis
}

func compositionAcceptanceContext(ctx context.Context) (context.Context, *compositionAcceptanceWalk) {
	if w, ok := ctx.Value(compositionAcceptanceScope{}).(*compositionAcceptanceWalk); ok {
		return ctx, w
	}
	w := &compositionAcceptanceWalk{seen: map[string]bool{}, visiting: map[string]bool{}, bases: map[string]RepositoryIntegrationBasis{}}
	return context.WithValue(ctx, compositionAcceptanceScope{}, w), w
}

// Formal new approval and downstream use revalidate fixed source acceptance.
// Completed execution facts themselves do not require the old creator to exist.
func (s *Service) checkCandidateCompositionAcceptanceTx(ctx context.Context, tx *sql.Tx, p Principal, project, run string) error {
	ctx, walk := compositionAcceptanceContext(ctx)
	key := p.TenantID + ":" + project + ":" + run
	if walk.seen[key] {
		return nil
	}
	if walk.visiting[key] || walk.depth >= 32 || len(walk.seen)+len(walk.visiting)+walk.bindings >= 4096 {
		return compositionError(409, "source_changed", compositionSource)
	}
	r, found, err := s.candidateCompositionExecutionRecordTx(ctx, tx, p.TenantID, project, run)
	if err != nil {
		return err
	}
	if found && (r.Execution == nil || r.Plan.RunID != run || r.Execution.PlanID != r.Plan.ID || r.Execution.PlanVersion != r.Plan.Version || ccmp.ValidateCandidateCompositionExecution(*r.Execution) != nil) {
		return compositionError(409, "source_changed", compositionSource)
	}
	walk.visiting[key] = true
	walk.depth++
	defer func() { delete(walk.visiting, key); walk.depth-- }()
	if found {
		if err = s.checkCompositionSourcesTx(ctx, tx, p, r.Plan); err != nil {
			return err
		}
	}
	if err = s.checkCodeBaselineAcceptanceTx(ctx, tx, p, project, run, walk); err != nil {
		return err
	}
	walk.seen[key] = true
	return nil
}

// Read only the protected, exact run/node grants. Editable record data and
// arbitrary JSON are not source authority. No old creator is reauthorized.
func (s *Service) checkCodeBaselineAcceptanceTx(ctx context.Context, tx *sql.Tx, p Principal, project, run string, walk *compositionAcceptanceWalk) error {
	rows, err := tx.QueryContext(ctx, s.q(`SELECT node_id,payload FROM suma_candidate_composition_executions WHERE tenant_id=? AND project_id=? AND run_id=? AND input_kind='code_baseline' ORDER BY node_id LIMIT 4097`), p.TenantID, project, run)
	if err != nil {
		return err
	}
	type input struct{ node, raw string }
	inputs := []input{}
	for rows.Next() {
		var in input
		if err = rows.Scan(&in.node, &in.raw); err != nil {
			break
		}
		inputs = append(inputs, in)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	if len(inputs) == 0 {
		return nil
	}
	bound, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, p.TenantID, project, run)
	if err != nil {
		return err
	}
	for _, in := range inputs {
		walk.bindings++
		if len(walk.seen)+len(walk.visiting)+walk.bindings > 4096 {
			return compositionError(409, "source_changed", compositionSource)
		}
		var execution codeBaselineExecution
		if json.Unmarshal([]byte(in.raw), &execution) != nil || ccmp.ValidateCodeBaselineBinding(execution.Binding) != nil || execution.Binding.ProjectID != project || execution.Route != bound {
			return compositionError(409, "baseline_changed", compositionBaseline)
		}
		binding := execution.Binding
		var taskRun, specRaw string
		var taskVersion int
		err = tx.QueryRowContext(ctx, s.q(`SELECT t.run_id,t.version,n.spec FROM suma_tasks t JOIN dag_runs d ON d.tenant_id=t.tenant_id AND d.project_id=t.project_id AND d.id=? JOIN dag_nodes n ON n.run_id=d.id AND n.id=? WHERE t.tenant_id=? AND t.project_id=? AND t.id=?`), run, in.node, p.TenantID, project, binding.TaskID).Scan(&taskRun, &taskVersion, &specRaw)
		if err != nil {
			return err
		}
		var spec dag.Node
		if taskRun != run || taskVersion <= binding.TaskVersion || json.Unmarshal([]byte(specRaw), &spec) != nil || spec.Type != "agent" || spec.Config["candidate_from"] != nil || !codeBaselineConfigMatches(spec.Config["code_baseline"], binding) {
			return compositionError(409, "baseline_changed", compositionBaseline)
		}
		current, _, err := s.taskCodeBaselineTx(ctx, tx, p.TenantID, project, binding.TaskID)
		if err != nil {
			return err
		}
		if current == nil || *current != binding {
			return compositionError(409, "baseline_changed", compositionBaseline)
		}
		basisKey := binding.CompositionID + ":" + strconv.Itoa(binding.CompositionVersion)
		basis, known := walk.bases[basisKey]
		if !known {
			source, err := s.candidateCompositionTx(ctx, tx, p.TenantID, project, binding.CompositionID)
			if err != nil {
				return err
			}
			if source.Plan.Version != binding.CompositionVersion || source.Plan.RunID == "" || source.Execution == nil {
				return compositionError(409, "baseline_changed", compositionBaseline)
			}
			if err = s.checkCandidateCompositionAcceptanceTx(ctx, tx, p, project, source.Plan.RunID); err != nil {
				return err
			}
			basis, err = s.repositoryBasisReadTx(ctx, tx, p, project, source.Plan.TaskID, source.Plan.ProducerNodeID)
			if err != nil {
				return err
			}
			if basis.RunID != source.Plan.RunID {
				return compositionError(409, "baseline_changed", compositionBaseline)
			}
			walk.bases[basisKey] = basis
		}
		if basis.Source != binding.Source || basis.BasisSHA256 != binding.AcceptedBasisSHA256 {
			return compositionError(409, "baseline_changed", compositionBaseline)
		}
	}
	return nil
}
