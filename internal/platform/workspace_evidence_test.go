package platform

import (
	"github.com/ai-mada/suma/internal/dag"
	"strings"
	"testing"
)

func TestCodeAcceptanceRequiresExactCleanCandidateAndAdminTest(t *testing.T) {
	code := reviewWorkspaceEvidence("c", false)
	tested := reviewWorkspaceEvidence("c", true)
	valid := reviewCandidatePairRun(code, tested)
	if err := validateCodeTestEvidence(valid); err != nil {
		t.Fatal("same committed revision should be certifiable", err)
	}
	for _, scenario := range []string{"missing-code", "missing-test", "dirty-code", "unknown", "uncommitted-test", "wrong-candidate", "not-admin-test", "invalid-hash"} {
		t.Run(scenario, func(t *testing.T) {
			a := reviewWorkspaceEvidence("c", false)
			b := reviewWorkspaceEvidence("c", true)
			b["base_commit"] = strings.Repeat("c", 40)
			switch scenario {
			case "missing-code":
				a = nil
			case "missing-test":
				b = nil
			case "dirty-code":
				a["clean"] = false
			case "unknown":
				a["outcome_unknown"] = true
			case "uncommitted-test":
				b["head_commit"] = strings.Repeat("d", 40)
			case "wrong-candidate":
				b["base_commit"] = strings.Repeat("b", 40)
				b["head_commit"] = strings.Repeat("b", 40)
			case "not-admin-test":
				b["test_command"] = false
			case "invalid-hash":
				a["evidence_sha256"] = "bad"
			}
			run := reviewCandidatePairRun(a, b)
			if err := validateCodeTestEvidence(run); err == nil {
				t.Fatal("unproved code accepted")
			}
		})
	}
}

// Numeric attempt values may legitimately be the same on different nodes.
// The provenance points at this exact producer, not merely a matching SHA.
func reviewCandidatePairRun(code, tested map[string]any) dag.Run {
	producer := dag.NodeRun{ID: "code", Type: "agent", State: "succeeded", Attempt: 1, Fence: 7, WorkerID: "producer", WorkspaceEvidence: code}
	if tested != nil {
		tested["base_commit"] = tested["head_commit"]
		tested["candidate_from"] = producer.ID
		tested["candidate_attempt"] = producer.Attempt
		tested["candidate_fence"] = producer.Fence
		if code != nil {
			tested["candidate_evidence_sha256"] = code["evidence_sha256"]
		}
	}
	return dag.Run{Nodes: []dag.NodeRun{producer, {ID: "test", Type: "test", State: "succeeded", Attempt: 1, Fence: 8, WorkerID: "tester", WorkspaceEvidence: tested}}}
}

func TestCodeAcceptanceRequiresIndependentCurrentProducerPair(t *testing.T) {
	valid := func() dag.Run {
		return reviewCandidatePairRun(reviewWorkspaceEvidence("c", false), reviewWorkspaceEvidence("c", true))
	}
	if err := validateCodeTestEvidence(valid()); err != nil {
		t.Fatal("different workers with the same numeric attempt should be eligible", err)
	}
	for _, scenario := range []string{"same-worker", "missing-producer-worker", "missing-test-worker", "missing-producer-id", "missing-test-id", "same-node-id", "producer-attempt", "test-attempt", "producer-fence", "test-fence", "producer-state", "test-state", "wrong-source", "missing-source", "old-attempt", "old-fence", "old-evidence", "fractional-attempt", "another-producer-same-sha"} {
		t.Run(scenario, func(t *testing.T) {
			run := valid()
			producer, tested := &run.Nodes[0], &run.Nodes[1]
			switch scenario {
			case "same-worker":
				tested.WorkerID = producer.WorkerID
			case "missing-producer-worker":
				producer.WorkerID = ""
			case "missing-test-worker":
				tested.WorkerID = ""
			case "missing-producer-id":
				producer.ID = ""
			case "missing-test-id":
				tested.ID = ""
			case "same-node-id":
				tested.ID = producer.ID
			case "producer-attempt":
				producer.Attempt = 0
			case "test-attempt":
				tested.Attempt = 0
			case "producer-fence":
				producer.Fence = 0
			case "test-fence":
				tested.Fence = 0
			case "producer-state":
				producer.State = "failed"
			case "test-state":
				tested.State = "failed"
			case "wrong-source":
				tested.WorkspaceEvidence["candidate_from"] = "other"
			case "missing-source":
				delete(tested.WorkspaceEvidence, "candidate_from")
			case "old-attempt":
				tested.WorkspaceEvidence["candidate_attempt"] = producer.Attempt + 1
			case "old-fence":
				tested.WorkspaceEvidence["candidate_fence"] = producer.Fence + 1
			case "old-evidence":
				tested.WorkspaceEvidence["candidate_evidence_sha256"] = strings.Repeat("d", 64)
			case "fractional-attempt":
				tested.WorkspaceEvidence["candidate_attempt"] = 1.5
			case "another-producer-same-sha":
				other := *producer
				other.ID, other.WorkerID = "another-code", "another-producer"
				run.Nodes = append(run.Nodes, other)
			}
			if err := validateCodeTestEvidence(run); err == nil {
				t.Fatal("unmatched or non-independent producer was certified")
			}
		})
	}
}

func TestTestOnlyNonCodeFactsCannotCertifyCode(t *testing.T) {
	if err := validateCodeTestEvidence(dag.Run{Nodes: []dag.NodeRun{{Type: "test", State: "succeeded"}}}); err != nil {
		t.Fatal("non-code test facts changed", err)
	}
	if err := validateCodeTestEvidence(dag.Run{Nodes: []dag.NodeRun{{Type: "command", State: "succeeded"}, {Type: "test", State: "succeeded"}}}); err == nil {
		t.Fatal("non-code facts certified code")
	}
}
