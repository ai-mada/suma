// Package taskconversationreplycontract is the closed, read-only reply payload.
// It contains no execution, workspace, billing or approval authority.
package taskconversationreplycontract

import (
	"errors"
	"strings"
	"unicode/utf8"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

const Protocol = "suma-task-conversation-reply-v1"
const MaxOutputBytes = 32768
const MaxReplyBytes = 32000
const MaxReplyScalars = 8000

var ErrInvalid = errors.New("invalid task conversation reply")

type Output struct {
	SchemaVersion int    `json:"schema_version"`
	Reply         string `json:"reply"`
}

func Validate(value Output) error {
	if value.SchemaVersion != 1 || !utf8.ValidString(value.Reply) || strings.TrimSpace(value.Reply) == "" || strings.ContainsRune(value.Reply, 0) || len(value.Reply) > MaxReplyBytes || utf8.RuneCountInString(value.Reply) > MaxReplyScalars {
		return ErrInvalid
	}
	return nil
}

// The existing closed decoder checks duplicate/case-aliased/unknown keys,
// null scalars, lexical integers and malformed Unicode before typed decoding.
func Parse(raw string) (Output, error) {
	var value Output
	if len(raw) > MaxOutputBytes || !utf8.ValidString(raw) || woc.Decode([]byte(raw), &value) != nil || Validate(value) != nil {
		return Output{}, ErrInvalid
	}
	return value, nil
}
