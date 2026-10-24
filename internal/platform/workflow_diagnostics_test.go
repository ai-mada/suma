package platform

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ai-mada/suma/internal/dag"
	"net/http/httptest"
	"testing"
)

func TestWorkflowValidationStructuredDiagnosticsKeepRawAndLanguage(t *testing.T) {
	f := newFixture(t)
	for _, locale := range []string{"zh-CN", "en-US"} {
		raw, _ := json.Marshal(map[string]any{"project_id": f.project, "workflow": dag.Workflow{Name: "原始流程", Nodes: []dag.Node{}}})
		r := httptest.NewRequest("POST", "http://suma.test/api/v1/workflows/validate", bytes.NewReader(raw))
		r.AddCookie(f.cookie)
		r.Header.Set("X-Suma-Language", locale)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		var body map[string]any
		json.Unmarshal(w.Body.Bytes(), &body)
		if w.Code != 200 || body["valid"] != false {
			t.Fatal(w.Code, w.Body.String())
		}
		if body["errors"].([]any)[0] != "invalid: workflow requires 1..1000 nodes" {
			t.Fatal("original changed", body)
		}
		items, ok := body["diagnostics"].([]any)
		if !ok || len(items) != 1 {
			t.Fatal("structured diagnostics missing", body)
		}
		d := items[0].(map[string]any)
		if d["localized"] != true || d["locale"] != locale || d["message_key"] == "" || d["message"] == body["errors"].([]any)[0] {
			t.Fatal("untranslated", d)
		}
	}
}

func TestDAGPublicErrorProjectionKeepsStatusAndUnknownContext(t *testing.T) {
	for _, item := range []struct {
		err       error
		status    int
		localized bool
	}{
		{dag.Compile(dag.Workflow{}), 400, true}, {dag.ErrInvalid, 400, true}, {dag.ErrConflict, 409, true}, {dag.ErrStale, 409, true}, {dag.ErrUnsupported, 422, true},
		{dag.ErrNotFound, 404, true}, {fmt.Errorf("foreign context: %w", dag.Compile(dag.Workflow{})), 400, false},
		{errors.Join(dag.Compile(dag.Workflow{}), errors.New("foreign")), 400, false},
		{&APIError{400, "invalid_input", "invalid", nil}, 400, false},
	} {
		w := httptest.NewRecorder()
		w.Header().Set("Content-Language", "zh-CN")
		writeError(w, item.err)
		var body struct {
			Error struct {
				Localized bool   `json:"localized"`
				Message   string `json:"message"`
			}
		}
		json.Unmarshal(w.Body.Bytes(), &body)
		if w.Code != item.status || body.Error.Localized != item.localized {
			t.Fatal(item.err, w.Code, w.Body.String())
		}
		if !item.localized && body.Error.Message != item.err.Error() {
			t.Fatal("foreign context lost")
		}
	}
}
