package platform

import (
	"encoding/json"
	"strings"
	"testing"
)

func knowledgePromptLargeBasis() *DeliveryKnowledgeBasis {
	b := &DeliveryKnowledgeBasis{Protocol: "suma-accepted-delivery-v1", SHA256: strings.Repeat("b", 64), Tests: []DeliveryKnowledgeTestEvidence{}, Candidates: []DeliveryKnowledgeCandidateEvidence{}}
	for i := 0; i < 256; i++ {
		b.Tests = append(b.Tests, DeliveryKnowledgeTestEvidence{RunID: "PRIVATE_FULL_TEST_METADATA_" + strings.Repeat("x", 180), NodeID: "independent-test", Attempt: 1, Fence: 1, WorkerID: "test-worker", ResultSHA256: strings.Repeat("c", 64)})
	}
	for i := 0; i < 128; i++ {
		b.Candidates = append(b.Candidates, DeliveryKnowledgeCandidateEvidence{Kind: "workspace_revision", RunID: "PRIVATE_FULL_CANDIDATE_METADATA_" + strings.Repeat("y", 180), NodeID: "develop", Attempt: 1, Fence: 1, WorkerID: "development-worker", HeadCommit: strings.Repeat("d", 40), EvidenceSHA256: strings.Repeat("e", 64)})
	}
	return b
}
func TestKnowledgePromptProjectionPreservesFullAuthority(t *testing.T) {
	basis := knowledgePromptLargeBasis()
	source := KnowledgeIntelligenceSource{Sources: []ProjectKnowledgeSource{{Kind: "documents", ID: "ordinary-before", Version: 2, Title: "Source before", Quote: "Keep original quote before", SHA256: strings.Repeat("a", 64)}, {Kind: "accepted_delivery", ID: "accepted-task", Version: 7, Title: "Accepted work", Quote: "Exact reviewed delivery quote 用户\n<raw>", SHA256: strings.Repeat("f", 64), Delivery: basis}, {Kind: "issues", ID: "ordinary-after", Version: 3, Title: "Source after", Quote: "Keep original quote after", SHA256: strings.Repeat("a", 64)}}}
	source.SHA256 = digest(stringify(source.Sources))
	before := stringify(source)
	raw := stringify(knowledgeIntelligencePromptSource(source))
	if strings.Contains(raw, "PRIVATE_FULL_") || strings.Contains(raw, `"delivery":`) || len(raw) >= len(before)/4 {
		t.Fatal("model input repeats the complete delivery evidence")
	}
	var model struct {
		SHA256  string `json:"sha256"`
		Sources []struct {
			Kind, ID, Quote, SHA256 string
			DeliverySummary         map[string]any `json:"delivery_summary"`
		} `json:"sources"`
	}
	if json.Unmarshal([]byte(raw), &model) != nil || model.SHA256 != source.SHA256 || len(model.Sources) != 3 {
		t.Fatal("model projection changed authoritative source identity or cardinality")
	}
	for i, s := range model.Sources {
		if s.Kind != source.Sources[i].Kind || s.ID != source.Sources[i].ID || s.Quote != source.Sources[i].Quote || s.SHA256 != source.Sources[i].SHA256 {
			t.Fatal("source index or exact source content changed")
		}
	}
	summary := model.Sources[1].DeliverySummary
	if len(summary) != 3 || summary["basis_sha256"] != basis.SHA256 || summary["test_count"] != float64(256) || summary["candidate_count"] != float64(128) {
		t.Fatal("compact delivery summary lost exact basis/counts")
	}
	if stringify(source) != before || source.SHA256 != digest(stringify(source.Sources)) || len(source.Sources[1].Delivery.Tests) != 256 || len(source.Sources[1].Delivery.Candidates) != 128 {
		t.Fatal("projection mutated the full persistent source or its digest")
	}
}
func TestTaskKnowledgePromptFitsWithoutRepeatingDeliveryEvidence(t *testing.T) {
	w, p, task := taskDispatchBuildFixture()
	task.Knowledge = []ProjectKnowledgeContextItem{{ID: "published-knowledge", Revision: 4, Kind: "skill", Title: "Reviewed knowledge", Content: "Exact published content 用户\n<raw>", SHA256: strings.Repeat("a", 64), Sources: []ProjectKnowledgeContextSource{{Kind: "accepted_delivery", ID: "accepted-task", Version: 7, Title: "Accepted work", SHA256: strings.Repeat("c", 64), Delivery: knowledgePromptLargeBasis()}}}}
	before := stringify(task)
	got, err := buildTaskDispatchWorkflow(w, "develop", "profile-one", p, task)
	if err != nil {
		t.Fatal("bounded content was rejected because complete delivery metadata entered the prompt", err)
	}
	goal := got.Nodes[0].Config["goal"].(string)
	if len(goal) > 32000 || strings.Contains(goal, "PRIVATE_FULL_") || strings.Contains(goal, `"delivery":`) {
		t.Fatal("task prompt repeated full source evidence")
	}
	raw := strings.Split(strings.Split(goal, "<task_context_json>\n")[1], "\n</task_context_json>")[0]
	var model struct {
		Knowledge []struct {
			Content, SHA256 string
			Sources         []struct {
				SHA256  string
				Summary map[string]any `json:"delivery_summary"`
			}
		}
	}
	if json.Unmarshal([]byte(raw), &model) != nil || len(model.Knowledge) != 1 || model.Knowledge[0].Content != task.Knowledge[0].Content || model.Knowledge[0].SHA256 != task.Knowledge[0].SHA256 || model.Knowledge[0].Sources[0].SHA256 != task.Knowledge[0].Sources[0].SHA256 || model.Knowledge[0].Sources[0].Summary["basis_sha256"] != task.Knowledge[0].Sources[0].Delivery.SHA256 {
		t.Fatal("task prompt changed published content or original identity")
	}
	if stringify(task) != before {
		t.Fatal("building a prompt changed persistent TaskContext")
	}
}
func TestKnowledgePromptWithoutDeliveryKeepsOriginalBytes(t *testing.T) {
	for _, sources := range [][]ProjectKnowledgeSource{nil, {}, {{Kind: "documents", ID: "doc", Version: 2, Title: "Original", Quote: " Original\nquote ", SHA256: strings.Repeat("a", 64)}}} {
		s := KnowledgeIntelligenceSource{SHA256: strings.Repeat("b", 64), Sources: sources}
		if stringify(s) != stringify(knowledgeIntelligencePromptSource(s)) {
			t.Fatal("ordinary extraction changed its original bytes")
		}
	}
	_, _, task := taskDispatchBuildFixture()
	for _, items := range [][]ProjectKnowledgeContextItem{nil, {}, {{ID: "knowledge", Revision: 2, Content: "Original content", Sources: []ProjectKnowledgeContextSource{{Kind: "issues", ID: "issue", Version: 1, SHA256: strings.Repeat("a", 64)}}}}} {
		task.Knowledge = items
		if stringify(task) != stringify(taskKnowledgePromptContext(task)) {
			t.Fatal("ordinary task context changed its original bytes")
		}
	}
}
