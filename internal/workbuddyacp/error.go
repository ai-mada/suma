package workbuddyacp

import (
	"fmt"
	"io"
)

// Error contains classifications only. EffectUnknown means a POST crossed the
// transport boundary; it never asserts that a JSON-RPC operation completed.
type Error struct {
	Code          string
	StatusCode    int
	EffectUnknown bool
}

func (*Error) Error() string { return "WorkBuddy ACP operation did not produce a verified result" }
func (*Error) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddyacp.Error{details:redacted}")
}
func failure(code string, status int, unknown bool) error {
	return &Error{Code: code, StatusCode: status, EffectUnknown: unknown}
}
