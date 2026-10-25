package displaycontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolReasonCompleteSourceAndParameterSnapshot(t *testing.T) {
	reason := "vendor {{.reason}} native.run.failed"
	params := map[string]any{"reason": reason}
	source := "执行节点报告工具不可用：" + reason
	d := BindToolReason(source, "display.tool.reported_unavailable", params)
	if d == nil {
		t.Fatal("valid producer lost identity")
	}
	params["reason"] = "mutated"
	if d.MessageParams["reason"] != reason {
		t.Fatal("caller params aliased")
	}
	copy := ValidateToolReason(source, d)
	if copy == nil {
		t.Fatal("valid binding rejected")
	}
	copy.MessageParams["reason"] = "copy changed"
	if d.MessageParams["reason"] != reason {
		t.Fatal("validation params aliased")
	}
	if BindToolReason(source+" suffix", "display.tool.reported_unavailable", map[string]any{"reason": reason}) != nil {
		t.Fatal("partial source proof accepted")
	}
	for _, n := range []int{1, 512} {
		v := strings.Repeat("x", n)
		if BindToolReason("执行节点报告工具不可用："+v, "display.tool.reported_unavailable", map[string]any{"reason": v}) == nil {
			t.Errorf("valid %d-byte text rejected", n)
		}
	}
}
func TestToolReasonRejectsUnprovableMetadata(t *testing.T) {
	for _, c := range []struct {
		name, source, key string
		params            map[string]any
		version           int
	}{
		{"unknown", "工具档案已暂停，尚未授权派发", "display.tool.future", nil, 1},
		{"native", "Agent aborted or timed out", "native.run.aborted", nil, 1},
		{"wrong-source", "工具档案已永久撤销", "display.tool.profile_paused", nil, 1},
		{"translated-source", "The tool profile is paused and is not authorized for dispatch.", "display.tool.profile_paused", nil, 1},
		{"extra", "工具档案已暂停，尚未授权派发", "display.tool.profile_paused", map[string]any{"extra": "x"}, 1},
		{"version", "工具档案已暂停，尚未授权派发", "display.tool.profile_paused", nil, 2},
		{"missing", "执行节点报告工具不可用：x", "display.tool.reported_unavailable", nil, 1},
		{"type", "执行节点报告工具不可用：1", "display.tool.reported_unavailable", map[string]any{"reason": 1}, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			if ValidateToolReason(c.source, &DisplayDiagnostic{SchemaVersion: c.version, MessageKey: c.key, MessageParams: c.params}) != nil {
				t.Fatal("forged metadata accepted")
			}
		})
	}
	for _, v := range []string{"", strings.Repeat("x", 513), "line\nreason", string([]byte{0xff}), "\x00"} {
		if BindToolReason("执行节点报告工具不可用："+v, "display.tool.reported_unavailable", map[string]any{"reason": v}) != nil {
			t.Errorf("out-of-bound text accepted %q", v)
		}
	}
}
func TestSourceDefinitionsAreApprovedSortedSnapshots(t *testing.T) {
	before := SourceDefinitions()
	if len(before) != 16 {
		t.Fatalf("got %d definitions", len(before))
	}
	for i, d := range before {
		if i > 0 && before[i-1].Key >= d.Key {
			t.Fatal("definitions not sorted")
		}
		if d.SourceTemplate == "" {
			t.Fatal("missing source")
		}
	}
	for i, d := range before {
		if d.Key == "display.tool.reported_unavailable" {
			d.Parameters[0].Name = "corrupt"
		}
		before[i].Key = "corrupt"
		before[i].SourceTemplate = "corrupt"
	}
	for _, d := range SourceDefinitions() {
		if d.Key == "corrupt" || d.SourceTemplate == "corrupt" {
			t.Fatal("definitions aliased")
		}
		for _, p := range d.Parameters {
			if p.Name == "corrupt" {
				t.Fatal("parameter slice aliased")
			}
		}
	}
}
func TestDisplayDiagnosticRejectsNonClosedJSON(t *testing.T) {
	for _, raw := range []string{
		`{"schema_version":1,"message_key":"display.tool.profile_paused","extra":true}`,
		`{"schema_version":1,"schema_version":2,"message_key":"display.tool.profile_paused"}`,
		`{"schema_version":1,"message_key":"display.tool.reported_unavailable","message_params":{"reason":"x","reason":"y"}}`,
		`{"schema_version":1,"message_key":"display.tool.reported_unavailable","message_params":{"reason":"\ud800"}}`,
		`{"schema_version":1,"message_key":"display.tool.profile_paused","message_params":null}`,
		`{"schema_version":1,"MessageKey":"display.tool.profile_paused"}`,
		`null`,
	} {
		var d DisplayDiagnostic
		if json.Unmarshal([]byte(raw), &d) == nil {
			t.Errorf("nonclosed or lossy JSON accepted: %s", raw)
		}
	}
	var d DisplayDiagnostic
	if err := json.Unmarshal([]byte(`{"schema_version":1,"message_key":"display.tool.reported_unavailable","message_params":{"reason":"vendor"}}`), &d); err != nil || ValidateToolReason("执行节点报告工具不可用：vendor", &d) == nil {
		t.Fatal("valid closed JSON rejected", err)
	}
}
