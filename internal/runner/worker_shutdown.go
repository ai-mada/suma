package runner

import "fmt"

func (j *Journal) acknowledgedUnknownResults() map[string]struct{} {
	j.mu.Lock()
	defer j.mu.Unlock()
	results := map[string]struct{}{}
	for _, e := range j.entries {
		acknowledged := e.Acked || e.Terminal != nil && e.Terminal.ReconciliationOperationID != ""
		if e.Result != nil && e.Result.OutcomeUnknown && acknowledged {
			results[e.Result.OperationID] = struct{}{}
		}
	}
	return results
}

// A local stop receipt can leave durable delivery and an unanswered claim to
// the same worker's restart. It must leave no accepted execution whose native
// outcome is still unknown. The business reconciliation record is separate.
func (j *Journal) requireShutdownBusinessEvidence(priorUnknown map[string]struct{}) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, e := range j.entries {
		if !e.validResultDisposition() {
			return fmt.Errorf("Runner shutdown has an invalid original result disposition")
		}
		result := e.Result
		if e.ArtifactPending != nil {
			result = &e.ArtifactPending.Result
		}
		if e.CandidatePending != nil {
			result = &e.CandidatePending.Result
		}
		if result == nil {
			return fmt.Errorf("Runner shutdown needs reconciliation for an unfinished original execution")
		}
		if _, historical := priorUnknown[result.OperationID]; result.OutcomeUnknown && !historical {
			return fmt.Errorf("Runner shutdown cannot confirm an original execution outcome")
		}
	}
	for _, e := range j.workspaceWrites.Entries {
		if e.ProcessingStarted && e.Result == nil {
			return fmt.Errorf("Runner shutdown needs reconciliation for an unfinished workspace write")
		}
	}
	return nil
}

func (j *Journal) requireNoActiveToolMaintenance() error {
	if p, err := j.toolInspectionCheckpoint(); err != nil || p != nil && p.Started && !p.Complete {
		return fmt.Errorf("Runner shutdown needs original tool inspection reconciliation")
	}
	if p, err := j.installationCheckpoint(); err != nil || p != nil && p.Started && !p.Complete {
		return fmt.Errorf("Runner shutdown needs original installation scan reconciliation")
	}
	if p, err := j.preparationCheckpoint(); err != nil || p != nil && p.Started && !p.Complete {
		return fmt.Errorf("Runner shutdown needs original tool preparation reconciliation")
	}
	return nil
}
