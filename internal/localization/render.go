package localization

import "errors"

type Params map[string]any
type ParameterKind string

const (
	Text       ParameterKind = "text"
	Identifier ParameterKind = "identifier"
	Integer    ParameterKind = "integer"
)

type Parameter struct {
	Name string        `json:"name"`
	Kind ParameterKind `json:"kind"`
}
type Definition struct {
	Key        string            `json:"key"`
	Parameters []Parameter       `json:"parameters"`
	Texts      map[Locale]string `json:"texts"`
	// SourceTemplate proves the original producer's complete diagnostic. It is
	// never inferred from user content or exposed as an additional source alias.
	// Older Chinese diagnostics default to Texts[ZhCN].
	SourceTemplate string `json:"-"`
}
type Rendered struct {
	Key       string `json:"message_key"`
	Params    Params `json:"message_params,omitempty"`
	Locale    Locale `json:"locale"`
	Text      string `json:"message"`
	Localized bool   `json:"localized"`
	Fallback  string `json:"fallback,omitempty"`
}

var ErrInvalidParameters = errors.New("localization parameters invalid")
var ErrInvalidKey = errors.New("localization message key invalid")
