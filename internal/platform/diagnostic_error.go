package platform

import "github.com/ai-mada/suma/internal/localization"

// diagnosticAPIError adds display metadata to one precise APIError. Its raw
// Error(), status, code and details remain the original diagnostic: presentation
// is applied only at the HTTP boundary, never to receipts or persisted evidence.
type diagnosticAPIError struct {
	cause  *APIError
	key    string
	params localization.Params
}

func (e *diagnosticAPIError) Error() string { return e.cause.Error() }
func (e *diagnosticAPIError) Unwrap() error { return e.cause }

// Keep a private parameter snapshot. Later changes to a caller's map cannot
// relabel an already-created error or make concurrent responses race.
func invalidDiagnostic(key, source string, params map[string]any) error {
	copy := make(localization.Params, len(params))
	for name, value := range params {
		copy[name] = value
	}
	return &diagnosticAPIError{cause: &APIError{400, "invalid_input", source, nil}, key: key, params: copy}
}
