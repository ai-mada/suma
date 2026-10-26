package platform

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func TestRepositoryHTTPStrictScopeAndMachineTransport(t *testing.T) {
	f := newFixture(t)
	query := "?project_id=" + f.project
	for _, suffix := range []string{"&task_id=x&task_id=y", "&task_id=x&limit=1e0", "&task_id=x&limit=01", "&task_id=x&limit=101", "&task_id=x&unknown=1"} {
		f.call("GET", "/repository-integrations/plans"+query+suffix, nil, 400)
	}
	f.call("GET", "/repository-integrations/options"+query, nil, 400)
	body := f.body(map[string]any{"task_id": "task", "expected_task_version": 1, "source_node_id": "code", "target_id": "target", "expected_target_version": 1})
	raw, _ := json.Marshal(body)
	for _, replacement := range []string{"1.0", "1e0", "null", "true", "\"1\""} {
		mutated := strings.Replace(string(raw), `"expected_task_version":1`, `"expected_task_version":`+replacement, 1)
		if mutated == string(raw) {
			t.Fatal("numeric fixture did not mutate")
		}
		f.call("POST", "/repository-integrations/plans", json.RawMessage(mutated), 400)
	}
	f.call("POST", "/repository-integrations/plans", json.RawMessage(strings.TrimSuffix(string(raw), "}")+`,"bare_repository":"D:/untrusted"}`), 400)
	// Cookies and a human Bearer token never become provider or pool authority.
	for _, path := range []string{"repository-provider/consume", "repository-provider/observations", "pool-runner/repository-integrations/authorize"} {
		req := httptest.NewRequest("POST", "/api/v1/"+path, bytes.NewBufferString(`{}`))
		req.AddCookie(f.cookie)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Code < 400 || w.Code >= 500 {
			t.Fatalf("machine cookie transport %s => %d", path, w.Code)
		}
	}
	// A dedicated provider endpoint rejects duplicate JSON before typed decoding.
	req := httptest.NewRequest("POST", "/api/v1/repository-provider/consume", bytes.NewBufferString(`{"operation_id":"a","operation_id":"b"}`))
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("duplicate provider body => %d: %s", w.Code, w.Body.String())
	}
}

func TestRepositoryLocalizationPreservesApprovedEvidence(t *testing.T) {
	preview := &ri.RepositoryPreview{Diff: repositoryBasis, BlockedReason: repositoryTarget}
	observation := &ri.RepositoryObservation{Reason: repositoryUnknown, Preview: preview}
	plan := RepositoryIntegrationPlan{Blockers: []string{repositoryBusy}, Preview: preview, Observation: observation, Target: RepositoryIntegrationTarget{Blockers: []string{repositoryTarget}}}
	for _, locale := range []string{"zh-CN", "en-US"} {
		w := httptest.NewRecorder()
		w.Header().Set("Content-Language", locale)
		writeJSON(w, 200, plan)
		var out RepositoryIntegrationPlan
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if locale == "zh-CN" && (out.Blockers[0] == repositoryBusy || out.Target.Blockers[0] == repositoryTarget) {
			t.Fatal("closed reasons untranslated")
		}
		if out.Preview.Diff != repositoryBasis || out.Preview.BlockedReason != repositoryTarget || out.Observation.Reason != repositoryUnknown {
			t.Fatal("approved evidence bytes were translated")
		}
		if plan.Blockers[0] != repositoryBusy || plan.Target.Blockers[0] != repositoryTarget {
			t.Fatal("projection mutated source")
		}
		w = httptest.NewRecorder()
		w.Header().Set("Content-Language", locale)
		writeError(w, repositoryConflict("unknown", repositoryUnknown))
		if !strings.Contains(w.Body.String(), `"message_key":"repository_integration.unknown"`) || !strings.Contains(w.Body.String(), `"localized":true`) {
			t.Fatal(w.Body.String())
		}
	}
}
