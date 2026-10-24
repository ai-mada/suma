package platform

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func TestLocalizedProjectionCopiesDisplayFactsAndPreservesEvidence(t *testing.T) {
	const source = "共享池配置准入资源不足；不是已测硬件负载"
	admission := dag.NodeAdmissionSnapshot{Blockers: []dag.SchedulingBlocker{{Code: "pool_resource_capacity", Message: source, Source: "pool"}}}
	page := dag.ReadyPage{Items: []dag.ReadyEntry{{Admission: &admission}}}
	w := httptest.NewRecorder()
	w.Header().Set("Content-Language", "en-US")
	writeJSON(w, 200, page)
	if !strings.Contains(w.Body.String(), "not a measurement of hardware load") || page.Items[0].Admission.Blockers[0].Message != source {
		t.Fatal("projection missing or modified original read snapshot")
	}
	// The exact same words are deliberately retained as user data / tool
	// evidence. A recursive replacement would silently corrupt their meaning.
	evidence := map[string]any{"message": source, "reason": source, "output": source, "title": source}
	w = httptest.NewRecorder()
	w.Header().Set("Content-Language", "en-US")
	writeJSON(w, 200, evidence)
	var decoded map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	for key := range evidence {
		if decoded[key] != source {
			t.Fatal("raw evidence translated", key)
		}
	}
}
