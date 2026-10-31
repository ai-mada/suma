package acpcontract

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/strictjson"
)

var ErrPermissionInvalid = errors.New("invalid ACP native permission source")

func permissionText(value string, maximum int, required, multiline bool) bool {
	if len(value) > maximum || !utf8.ValidString(value) || required && strings.TrimSpace(value) == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t')) {
			return false
		}
	}
	return true
}

func ValidatePermissionOption(p PermissionOption) error {
	if !permissionText(p.ID, 128, true, false) || strings.TrimSpace(p.ID) != p.ID || !permissionText(p.Name, 512, true, false) {
		return ErrPermissionInvalid
	}
	switch p.Kind {
	case "allow_once", "reject_once", "allow_always", "reject_always":
		return nil
	}
	return ErrPermissionInvalid
}

func ValidatePermissionSource(p NativeSource) error {
	if !permissionText(p.SessionID, 128, true, false) || strings.TrimSpace(p.SessionID) != p.SessionID || !permissionText(p.ToolCallID, 128, true, false) || strings.TrimSpace(p.ToolCallID) != p.ToolCallID || !permissionText(p.Title, 4096, true, true) || !permissionText(p.Kind, 128, true, false) || strings.TrimSpace(p.Kind) != p.Kind || !permissionText(p.Details, 32<<10, false, true) || p.Options == nil || len(p.Options) < 1 || len(p.Options) > 16 {
		return ErrPermissionInvalid
	}
	seen := map[string]bool{}
	for _, option := range p.Options {
		if ValidatePermissionOption(option) != nil || seen[option.ID] {
			return ErrPermissionInvalid
		}
		seen[option.ID] = true
	}
	return nil
}

func ValidatePermissionChoice(p Choice) error {
	if !permissionText(p.OptionID, 128, true, false) || strings.TrimSpace(p.OptionID) != p.OptionID || p.Kind != "allow_once" && p.Kind != "reject_once" {
		return ErrPermissionInvalid
	}
	return nil
}

func PermissionChoiceMatchesSource(source NativeSource, choice Choice) bool {
	if ValidatePermissionSource(source) != nil || ValidatePermissionChoice(choice) != nil || choice.Kind == "allow_once" && !source.DetailsComplete {
		return false
	}
	for _, option := range source.Options {
		if option.ID == choice.OptionID && option.Kind == choice.Kind {
			return true
		}
	}
	return false
}

func (p *ToolPermissionOption) UnmarshalJSON(raw []byte) error {
	type wire ToolPermissionOption
	var value wire
	if strictjson.Decode(raw, &value, 4<<10) != nil || ValidatePermissionOption(PermissionOption(value)) != nil {
		return ErrPermissionInvalid
	}
	*p = ToolPermissionOption(value)
	return nil
}

func (p *ACPToolPermissionSource) UnmarshalJSON(raw []byte) error {
	type wire ACPToolPermissionSource
	var value wire
	if strictjson.Decode(raw, &value, 64<<10) != nil || ValidatePermissionSource(NativeSource(value)) != nil {
		return ErrPermissionInvalid
	}
	*p = ACPToolPermissionSource(value)
	return nil
}

func (p *ACPToolPermissionChoice) UnmarshalJSON(raw []byte) error {
	type wire ACPToolPermissionChoice
	var value wire
	if strictjson.Decode(raw, &value, 4<<10) != nil || ValidatePermissionChoice(Choice(value)) != nil {
		return ErrPermissionInvalid
	}
	*p = ACPToolPermissionChoice(value)
	return nil
}
