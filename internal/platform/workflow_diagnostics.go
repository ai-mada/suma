package platform

import (
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/localization"
	"net/http"
)

// WorkflowValidation separates immutable compiler evidence from the current
// presentation. Each diagnostic corresponds to the same-position raw error.
type WorkflowValidation struct {
	Valid       bool                    `json:"valid"`
	Errors      []string                `json:"errors"`
	Diagnostics []localization.Rendered `json:"diagnostics"`
}

func workflowValidation(w http.ResponseWriter, workflow dag.Workflow) WorkflowValidation {
	out := WorkflowValidation{Valid: true, Errors: []string{}, Diagnostics: []localization.Rendered{}}
	if err := dag.Compile(workflow); err != nil {
		out.Valid = false
		out.Errors = append(out.Errors, err.Error())
		locale := localization.Locale(w.Header().Get("Content-Language"))
		d, ok := localization.FormatError(locale, err)
		if !ok {
			d, _ = localization.WithSource(locale, "untranslated.invalid_graph", nil, err.Error())
		}
		out.Diagnostics = append(out.Diagnostics, d)
	}
	return out
}

// Only exact bare sentinel identities receive an annotation here. Do not wrap
// upstream sentinels: some scheduling paths intentionally compare them directly.
func bareDAGDiagnostic(err error) error {
	key := ""
	switch err {
	case dag.ErrInvalid:
		key = "dag.error.invalid"
	case dag.ErrConflict:
		key = "dag.error.conflict"
	case dag.ErrStale:
		key = "dag.error.stale_assignment"
	case dag.ErrUnsupported:
		key = "dag.error.unsupported"
	}
	if key != "" {
		return localization.AnnotateError(err, key, nil)
	}
	return err
}
