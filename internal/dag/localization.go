package dag

import "github.com/ai-mada/suma/internal/localization"

type diagnosticParams = localization.Params

// dagDiagnostic annotates only explicit, owned diagnostics. The cause remains
// the canonical source and unwraps unchanged; display language never enters
// workflow state, dispatch evidence, durable receipts, or scheduler decisions.
func dagDiagnostic(cause error, key string, params diagnosticParams) error {
	return localization.AnnotateError(cause, key, params)
}
