package runner

import "bytes"

// PoolConfigDiagnostic is an actionable failure location, not an authorization,
// freshness or execution receipt. Message is fixed display text supplied by the
// CLI; neither the typed error nor this projection carries an underlying cause.
type PoolConfigDiagnostic struct {
	Protocol     string `json:"protocol"`
	Code         string `json:"code"`
	ScopeIndex   int    `json:"scope_index"`
	Field        string `json:"field"`
	InputSHA256  string `json:"input_sha256"`
	OutputSHA256 string `json:"output_sha256"`
	Message      string `json:"message"`
}

const PoolConfigDiagnosticProtocol = "suma-pool-config-diagnostic-v1"

// PoolConfigPreparationError deliberately has no Unwrap/cause/path. Callers use
// errors.As for the closed safe facts rather than serializing a filesystem error.
type PoolConfigPreparationError struct {
	Code         string
	ScopeIndex   int
	Field        string
	InputSHA256  string
	OutputSHA256 string
}

func (*PoolConfigPreparationError) Error() string { return "Pool configuration preparation failed" }

func poolPreparationFailure(code string, row int, field string) *PoolConfigPreparationError {
	return &PoolConfigPreparationError{Code: code, ScopeIndex: row, Field: field}
}

// A diagnostic may correlate only a complete bounded private input. A truncated
// read/oversized stream is not the input the user reviewed and gets no digest.
func poolPreparationInputDigest(raw []byte) string {
	if len(raw) > poolPreparationInputLimit {
		return ""
	}
	return digest(bytes.TrimSpace(raw))
}
