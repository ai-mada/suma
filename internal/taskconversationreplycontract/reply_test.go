package taskconversationreplycontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTaskConversationReplyClosedOutput(t *testing.T) {
	valid := `{"schema_version":1,"reply":"中文 / English: advice only, no tool action."}`
	if value, err := Parse(valid); err != nil || !strings.Contains(value.Reply, "中文") {
		t.Fatal("valid multilingual plain reply rejected", value, err)
	}
	tooManyScalars, _ := json.Marshal(Output{SchemaVersion: 1, Reply: strings.Repeat("中", 8001)})
	for name, raw := range map[string]string{
		"duplicate":       `{"schema_version":1,"reply":"first","reply":"last"}`,
		"case alias":      `{"schema_version":1,"Reply":"advice"}`,
		"role forgery":    `{"schema_version":1,"reply":"advice","role":"assistant"}`,
		"command":         `{"schema_version":1,"reply":"advice","command":"run"}`,
		"null":            `{"schema_version":1,"reply":null}`,
		"fraction":        `{"schema_version":1.0,"reply":"advice"}`,
		"invalid Unicode": `{"schema_version":1,"reply":"\ud800"}`,
		"nul":             `{"schema_version":1,"reply":"a\u0000b"}`,
		"blank":           `{"schema_version":1,"reply":" \n\t "}`,
		"extra document":  valid + `{}`,
		"scalar limit":    string(tooManyScalars),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(raw); err == nil {
				t.Fatal("unsafe or ambiguous model output accepted")
			}
		})
	}
}
