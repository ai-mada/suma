package localization

import (
	"bytes"
	"errors"
)

// annotatedError is presentation metadata, not a replacement error category.
// Keeping the cause and captured source preserves errors.Is/As and raw logs.
type annotatedError struct {
	cause       error
	source, key string
	params      Params
}

func (e *annotatedError) Error() string { return e.source }
func (e *annotatedError) Unwrap() error { return e.cause }

// AnnotateError attaches a semantic key at its trusted producer. Parameters are
// snapshotted and checked again at display time. No language enters persistence.
func AnnotateError(cause error, key string, params Params) error {
	if cause == nil {
		return nil
	}
	copy := make(Params, len(params))
	for k, v := range params {
		copy[k] = v
	}
	return &annotatedError{cause: cause, source: cause.Error(), key: key, params: copy}
}

// FormatError accepts only a known annotation whose complete original message
// is reproducible by the declared canonical template. Additional wrapping or a
// joined foreign cause cannot silently lose context through translation.
func FormatError(locale Locale, err error) (Rendered, bool) {
	var e *annotatedError
	if err == nil || !errors.As(err, &e) || err.Error() != e.source || e.cause.Error() != e.source {
		return Rendered{}, false
	}
	d, ok := definitions[e.key]
	if !ok {
		return Rendered{}, false
	}
	checked, validateErr := validateParameters(d.Parameters, e.params)
	if validateErr != nil {
		return Rendered{}, false
	}
	var b bytes.Buffer
	if executeErr := sourceTemplates[e.key].Execute(&b, checked); executeErr != nil || b.String() != e.source {
		return Rendered{}, false
	}
	out, formatErr := Format(locale, e.key, checked)
	return out, formatErr == nil && out.Localized
}
