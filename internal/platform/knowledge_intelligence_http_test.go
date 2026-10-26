package platform

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Exercise the actual REST decoder: internal Go defaults must not accidentally
// accept ambiguous JSON budget/version lexemes from an external caller.
func TestKnowledgeIntelligenceHTTPStrictBudgetsAndScopedDraftJourney(t *testing.T) {
	h := newKnowledgeIntelligenceFixture(t)
	f := h.f
	raw, err := json.Marshal(h.request)
	if err != nil {
		t.Fatal(err)
	}
	var base map[string]any
	if json.Unmarshal(raw, &base) != nil {
		t.Fatal("fixture request")
	}
	for _, budget := range []string{"0", "1.0", "1e0", "2", "null", `"1"`} {
		body := strings.TrimSuffix(string(raw), "}") + `,"max_steps":` + budget + `}`
		f.call("POST", "/knowledge-intelligence/analyses", json.RawMessage(body), 400)
	}
	for _, tokens := range []string{"0", "8193", "1.0", "1e3", "null"} {
		body := strings.TrimSuffix(string(raw), "}") + `,"max_output_tokens":` + tokens + `}`
		f.call("POST", "/knowledge-intelligence/analyses", json.RawMessage(body), 400)
	}
	created := f.call("POST", "/knowledge-intelligence/analyses", base, 201)
	id, _ := created["id"].(string)
	if id == "" || created["max_steps"] != float64(1) || created["max_output_tokens"] != float64(4096) {
		t.Fatal("default bounded execution", created)
	}
	f.call("GET", "/knowledge-intelligence/analyses?project_id="+f.project+"&limit=1e0", nil, 400)
	f.call("GET", "/knowledge-intelligence/analyses?project_id="+f.project+"&limit=1&limit=2", nil, 400)
	page := f.call("GET", "/knowledge-intelligence/analyses?project_id="+f.project+"&limit=20", nil, 200)
	if page["total"] != float64(1) {
		t.Fatal("invalid budget created an extra run", page)
	}
	a := h.claim(t)
	h.complete(t, h.result(a, h.output()))
	ready := f.call("GET", "/knowledge-intelligence/analyses/"+id+"?project_id="+f.project, nil, 200)
	if ready["state"] != "ready" || ready["original_output"] != h.output() {
		t.Fatal("actual completion not available through REST", ready)
	}
	body := map[string]any{"project_id": f.project, "expected_version": ready["version"], "action": "adopt", "reason": "Review the model proposal as a project draft", "acknowledge_draft": true, "idempotency_key": newID()}
	for _, version := range []string{"1.0", "1e0"} {
		raw := fmt.Sprintf(`{"project_id":%q,"expected_version":%s,"action":"dismiss","reason":"invalid lexical version","idempotency_key":%q}`, f.project, version, newID())
		f.call("POST", "/knowledge-intelligence/analyses/"+id+"/commands", json.RawMessage(raw), 400)
	}
	adopted := f.call("POST", "/knowledge-intelligence/analyses/"+id+"/commands", body, 200)
	if adopted["state"] != "adopted" || adopted["adopted_knowledge_id"] == "" {
		t.Fatal("REST adoption did not create a draft", adopted)
	}
	knowledge := f.call("GET", "/project-knowledge/"+adopted["adopted_knowledge_id"].(string)+"?project_id="+f.project, nil, 200)
	if knowledge["state"] != "draft" || knowledge["published_revision"] != float64(0) || knowledge["model_adoption"] == nil {
		t.Fatal("adoption bypassed publication or lost ancestry", knowledge)
	}
}
