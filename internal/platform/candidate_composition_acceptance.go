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
	if found && (r.Plan.RunID != run || !validCandidateCompositionExecutionRecord(r)) {
		return compositionError(409, "source_changed", compositionSource)
	}
	walk.visiting[key] = true
	walk.depth++
	defer func() { delete(walk.visiting, key); walk.depth-- }()
	if found {
		if r.Plan.ConflictBasis != nil {
			if err = s.checkCompositionConflictBasisTx(ctx, tx, p.TenantID, project, *r.Plan.ConflictBasis); err != nil {
				return err
			}
		}
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
		if _, err = s.codeBaselineTaskRelationTx(ctx, tx, p.TenantID, project, run, in.node, binding, false); err != nil {
			return err
		}
		current, _, err := s.taskCodeBaselineTx(ctx, tx, p.TenantID, project, binding.TaskID)
		if err != nil {
			return err
		}
		if current == nil || *current != binding {
			return compositionError(409, "baseline_changed", compositionBaseline)
		}
		basisKey := "composition:" + binding.CompositionID + ":" + strconv.Itoa(binding.CompositionVersion)
		if binding.TerminalCandidatePlanID != "" {
			basisKey = "terminal:" + binding.TerminalCandidatePlanID + ":" + strconv.Itoa(binding.TerminalCandidatePlanVersion)
		}
		if binding.SourceTaskID != "" {
			basisKey = acceptedTaskBaselineMemoKey(binding.SourceTaskID, binding.SourceTaskVersion, binding.Source)
		}
		basis, known := walk.bases[basisKey]
		if !known {
			basis, err = s.codeBaselineAcceptedOriginTx(ctx, tx, p, project, binding)
			if err != nil {
				return err
			}
			walk.bases[basisKey] = basis
		}
		if basis.Source != binding.Source || basis.BasisSHA256 != binding.AcceptedBasisSHA256 {
			return compositionError(409, "baseline_changed", compositionBaseline)
		}
	}
	return nil
}
