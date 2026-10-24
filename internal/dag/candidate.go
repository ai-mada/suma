package dag

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
)

var commitDigest = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
var evidenceDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)
var errCandidatePending = errors.New("candidate source still active")

// A candidate is an explicit dependency of an immutable build/test consumer,
// never arbitrary user input naming a commit. The trusted Runner additionally
// restricts command consumers to administrator-designated artifact builds.
// Dynamic cross-map and cross-subrun handoffs are not inferred.
func validateCandidateReferences(w Workflow) error {
	nodes := map[string]Node{}
	parents := map[string][]string{}
	for _, n := range w.Nodes {
		nodes[n.ID] = n
	}
	for _, edge := range w.Edges {
		parents[edge.To] = append(parents[edge.To], edge.From)
	}
	for _, n := range w.Nodes {
		for _, reserved := range []string{"candidate_commit", "candidate_attempt", "candidate_fence", "candidate_evidence_sha256"} {
			if _, exists := n.Config[reserved]; exists {
				return fmt.Errorf("%w: candidate proof fields belong to scheduler assignments", ErrInvalid)
			}
		}
		raw, exists := n.Config["candidate_from"]
		if !exists {
			continue
		}
		source, ok := raw.(string)
		if !ok || source == "" || n.Type != "test" && n.Type != "command" && n.Type != "agent" {
			return fmt.Errorf("%w: candidate_from requires an immutable build/test consumer and a source node", ErrInvalid)
		}
		sourceNode, exists := nodes[source]
		if !exists || sourceNode.Type != "command" && sourceNode.Type != "agent" {
			return fmt.Errorf("%w: candidate source must be a static code command or agent", ErrInvalid)
		}
		ancestors := map[string]bool{}
		queue := append([]string{}, parents[n.ID]...)
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			if ancestors[id] {
				continue
			}
			ancestors[id] = true
			queue = append(queue, parents[id]...)
		}
		if !ancestors[source] {
			return fmt.Errorf("%w: candidate source must be an explicit ancestor", ErrInvalid)
		}
	}
	return nil
}

func validateArtifactReferences(w Workflow) error {
	nodes := map[string]Node{}
	parents := map[string][]string{}
	for _, n := range w.Nodes {
		nodes[n.ID] = n
	}
	for _, edge := range w.Edges {
		parents[edge.To] = append(parents[edge.To], edge.From)
	}
	for _, n := range w.Nodes {
		raw, exists := n.Config["artifact_from"]
		if !exists {
			continue
		}
		source, ok := raw.(string)
		if !ok || source == "" || n.Type != "test" {
			return fmt.Errorf("%w: artifact_from requires a test and an explicit build ancestor", ErrInvalid)
		}
		upstream, exists := nodes[source]
		if !exists || upstream.Type != "command" && upstream.Type != "agent" {
			return fmt.Errorf("%w: artifact source must be a static build command", ErrInvalid)
		}
		seen := map[string]bool{}
		queue := append([]string(nil), parents[n.ID]...)
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			if seen[id] {
				continue
			}
			seen[id] = true
			queue = append(queue, parents[id]...)
		}
		if !seen[source] {
			return fmt.Errorf("%w: artifact source is not an ancestor", ErrInvalid)
		}
	}
	return nil
}

func numericZero(v any) bool {
	switch n := v.(type) {
	case int:
		return n == 0
	case float64:
		return n == 0
	case int64:
		return n == 0
	}
	return false
}
func validWorkspaceProof(proof map[string]any) bool {
	head, _ := proof["head_commit"].(string)
	base, _ := proof["base_commit"].(string)
	hash, _ := proof["evidence_sha256"].(string)
	return commitDigest.MatchString(head) && commitDigest.MatchString(base) && evidenceDigest.MatchString(hash) && proof["state"] == "completed" && proof["clean"] == true && proof["outcome_unknown"] == false && numericZero(proof["exit_code"])
}
func candidateFor(r *runState, n *nodeState) (*Assignment, error) {
	source, _ := n.spec.Config["candidate_from"].(string)
	if source == "" {
		return nil, nil
	}
	upstream := r.find(source)
	if upstream != nil && !terminal(upstream.State) {
		return nil, errCandidatePending
	}
	if n.Type != "test" && n.Type != "command" && n.Type != "agent" || upstream == nil || upstream.State != "succeeded" || upstream.Attempt < 1 || upstream.Fence < 1 || !validWorkspaceProof(upstream.WorkspaceEvidence) {
		return nil, fmt.Errorf("%w: candidate source has no clean, committed current-attempt evidence", ErrInvalid)
	}
	return &Assignment{CandidateCommit: upstream.WorkspaceEvidence["head_commit"].(string), CandidateFrom: source, CandidateAttempt: upstream.Attempt, CandidateFence: upstream.Fence, CandidateEvidenceSHA256: upstream.WorkspaceEvidence["evidence_sha256"].(string)}, nil
}
func (t *transaction) freezeCandidate(r *runState, n *nodeState, a *Assignment) error {
	if a == nil {
		return nil
	}
	if err := t.exec(`INSERT INTO dag_candidate_revisions(run_id,node_id,attempt,commit_id,source_node,source_attempt,source_fence,evidence_sha256)VALUES(?,?,?,?,?,?,?,?)`, r.ID, n.ID, n.Attempt, a.CandidateCommit, a.CandidateFrom, a.CandidateAttempt, a.CandidateFence, a.CandidateEvidenceSHA256); err != nil {
		return err
	}
	return t.event(r, n.ID, "candidate_pinned", map[string]any{"attempt": n.Attempt, "commit": a.CandidateCommit, "source_node": a.CandidateFrom, "source_attempt": a.CandidateAttempt, "source_fence": a.CandidateFence, "evidence_sha256": a.CandidateEvidenceSHA256})
}
func (t *transaction) validateCandidateResult(r *runState, n *nodeState, result Result) error {
	// Failed, cancelled and unknown attempts must still report their outcome even
	// when preparation stopped before a workspace could be created.
	if result.ExitCode != 0 || result.Error != "" || result.Stopped && r.State == "cancelling" || result.OutcomeUnknown {
		return nil
	}
	var commit, source, hash string
	var attempt int
	var fence int64
	err := t.row(`SELECT commit_id,source_node,source_attempt,source_fence,evidence_sha256 FROM dag_candidate_revisions WHERE run_id=? AND node_id=? AND attempt=?`, result.RunID, n.ID, n.Attempt).Scan(&commit, &source, &attempt, &fence, &hash)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	proof := result.WorkspaceEvidence
	if !validWorkspaceProof(proof) || proof["base_commit"] != commit || proof["head_commit"] != commit || n.Type == "test" && proof["test_command"] != true {
		return fmt.Errorf("%w: successful immutable build/test must prove the scheduler-pinned candidate commit", ErrInvalid)
	}
	if proof["candidate_from"] != source || !sameProofInteger(proof["candidate_attempt"], int64(attempt)) || !sameProofInteger(proof["candidate_fence"], fence) || proof["candidate_evidence_sha256"] != hash {
		return fmt.Errorf("%w: candidate source provenance differs from the frozen assignment", ErrInvalid)
	}
	return nil
}
func sameProofInteger(value any, expected int64) bool {
	switch n := value.(type) {
	case int:
		return int64(n) == expected
	case int64:
		return n == expected
	case float64:
		return n == float64(expected)
	}
	return false
}
