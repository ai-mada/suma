package workbuddycloudcontract

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func collaborationSource() CloudCollaborationSource {
	return CloudCollaborationSource{ProjectID: "project", RunID: "run", NodeID: "node", SubmissionID: "submission", SubmissionVersion: 2, TaskID: "cloud_task", NodeSpecSHA256: strings.Repeat("a", 64), RequestSHA256: strings.Repeat("b", 64)}
}

func collaborationLoad() CloudHistoryLoad {
	return CloudHistoryLoad{ID: "load", Version: 2, OperationID: "history-operation", HeadVersionAtStart: 0, Source: collaborationSource(), State: "loaded", Coverage: "unverified", StartedAt: "2026-10-31T01:00:00Z", DeadlineAt: "2026-10-31T01:00:30Z", FinishedAt: "2026-10-31T01:00:01Z", EntryCount: 2, UnsupportedCount: 1, TextBytes: 5}
}

func TestCloudHistoryRequestRequiresExplicitHeadVersionAndClosedInput(t *testing.T) {
	valid := `{"project_id":"project","expected_submission_version":2,"expected_version":0,"idempotency_key":"history-operation"}`
	var request CloudHistoryLoadRequest
	if Decode([]byte(valid), &request) != nil || request.Validate() != nil || request.ExpectedVersion != 0 {
		t.Fatal("explicit first head version rejected")
	}
	for name, raw := range map[string]string{
		"missing":       strings.Replace(valid, `,"expected_version":0`, "", 1),
		"null":          strings.Replace(valid, `"expected_version":0`, `"expected_version":null`, 1),
		"negative":      strings.Replace(valid, `"expected_version":0`, `"expected_version":-1`, 1),
		"exhausted":     strings.Replace(valid, `"expected_version":0`, `"expected_version":2147483647`, 1),
		"duplicate":     strings.Replace(valid, `"expected_version":0`, `"expected_version":0,"expected_version":1`, 1),
		"case_alias":    strings.Replace(valid, `"expected_version":0`, `"Expected_Version":0`, 1),
		"caller_task":   strings.Replace(valid, `"project_id":"project"`, `"project_id":"project","task_id":"other"`, 1),
		"caller_prompt": strings.Replace(valid, `"project_id":"project"`, `"project_id":"project","prompt":"private"`, 1),
		"unicode":       strings.Replace(valid, `"project_id":"project"`, `"project_id":"\ud800"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			var out CloudHistoryLoadRequest
			if Decode([]byte(raw), &out) == nil && out.Validate() == nil {
				t.Fatal("invalid or caller-controlled source accepted")
			}
		})
	}
}

func TestCloudHistoryProjectionRetainsUnverifiedCoverageAndClosedStates(t *testing.T) {
	valid := collaborationLoad()
	if valid.Validate() != nil {
		t.Fatal("valid stored load rejected")
	}
	for _, mutate := range []func(*CloudHistoryLoad){
		func(v *CloudHistoryLoad) { v.Coverage = "complete" },
		func(v *CloudHistoryLoad) { v.Source.TaskID = "../other" },
		func(v *CloudHistoryLoad) { v.Source.SubmissionVersion = 0 },
		func(v *CloudHistoryLoad) { v.EntryCount = 501 },
		func(v *CloudHistoryLoad) { v.UnsupportedCount = 3 },
		func(v *CloudHistoryLoad) { v.TextBytes = (1 << 20) + 1 },
		func(v *CloudHistoryLoad) { v.State = "completed" },
		func(v *CloudHistoryLoad) { v.State = "loading" },
		func(v *CloudHistoryLoad) { v.FinishedAt = "" },
		func(v *CloudHistoryLoad) { v.FinishedAt = "2026-10-31T00:59:59Z" },
		func(v *CloudHistoryLoad) { v.DeadlineAt = v.StartedAt },
		func(v *CloudHistoryLoad) { v.Overdue = true },
		func(v *CloudHistoryLoad) { v.OperationID = "short" },
		func(v *CloudHistoryLoad) { v.HeadVersionAtStart = -1 },
		func(v *CloudHistoryLoad) { v.HeadVersionAtStart = MaxVersion },
		func(v *CloudHistoryLoad) { v.ErrorCode = "provider_private_body" },
	} {
		bad := valid
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid load projection admitted")
		}
	}
	for _, state := range []string{"interrupted", "limited", "rejected"} {
		load := valid
		load.State, load.ErrorCode = state, "workbuddy_cloud_history_interrupted"
		if load.Validate() != nil {
			t.Fatalf("bounded terminal observation rejected: %s", state)
		}
	}
}

func TestCloudHistoryRequiredFalseAndZeroFieldsCannotBeOmitted(t *testing.T) {
	load := collaborationLoad()
	load.State, load.FinishedAt, load.EntryCount, load.UnsupportedCount, load.TextBytes = "loading", "", 0, 0, 0
	load.Overdue = true
	if load.Validate() != nil {
		t.Fatal("expired loading observation rejected")
	}
	load.Overdue = false
	page := CloudHistoryPage{Load: load, Items: []CloudHistoryEntry{}}
	view := CloudCollaborationView{Source: collaborationSource(), AllowedActions: []string{}}
	for _, value := range []any{load, page, view} {
		raw, _ := json.Marshal(value)
		var out any
		switch value.(type) {
		case CloudHistoryLoad:
			out = &CloudHistoryLoad{}
		case CloudHistoryPage:
			out = &CloudHistoryPage{}
		case CloudCollaborationView:
			out = &CloudCollaborationView{}
		}
		if Decode(raw, out) != nil {
			t.Fatal("explicit zero/false projection rejected")
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		field := "overdue"
		switch value.(type) {
		case CloudHistoryPage:
			field = "has_more"
		case CloudCollaborationView:
			field = "version"
		}
		delete(fields, field)
		missing, _ := json.Marshal(fields)
		if Decode(missing, out) == nil {
			t.Fatalf("missing required %s admitted", field)
		}
		if _, ok := value.(CloudHistoryLoad); ok {
			for _, required := range []string{"operation_id", "head_version_at_start"} {
				var loadFields map[string]json.RawMessage
				_ = json.Unmarshal(raw, &loadFields)
				delete(loadFields, required)
				missing, _ = json.Marshal(loadFields)
				if Decode(missing, &CloudHistoryLoad{}) == nil {
					t.Fatalf("missing required %s admitted", required)
				}
			}
		}
	}
}

func TestCloudHistoryEntryRejectsUnprojectedContentAndPreservesLiteralText(t *testing.T) {
	entry := CloudHistoryEntry{Sequence: 1, Kind: "assistant_text", Text: "literal\nprivate text\t你好"}
	if entry.Validate() != nil {
		t.Fatal("literal text rejected")
	}
	raw, _ := json.Marshal(entry)
	if !strings.Contains(string(raw), `private text`) || strings.Contains(fmt.Sprintf("%+v %#v", entry, entry), "private text") {
		t.Fatal("JSON lost literal content or diagnostics exposed text")
	}
	for _, bad := range []CloudHistoryEntry{
		{Sequence: 0, Kind: "assistant_text", Text: "text"},
		{Sequence: 501, Kind: "user_text", Text: "text"},
		{Sequence: 1, Kind: "agent_thought", Text: "private thought"},
		{Sequence: 1, Kind: "unsupported", Text: "raw body"},
		{Sequence: 1, Kind: "assistant_text", Text: strings.Repeat("a", (64<<10)+1)},
		{Sequence: 1, Kind: "assistant_text", Text: "a\x00b"},
		{Sequence: 1, Kind: "assistant_text", Text: string([]byte{0xff})},
	} {
		if bad.Validate() == nil {
			t.Fatal("unsafe or oversized entry admitted")
		}
	}
}

func TestCloudHistoryViewBindsStableSourceAndPageUsesLocalSequence(t *testing.T) {
	load := collaborationLoad()
	view := CloudCollaborationView{Source: collaborationSource(), Version: 3, LatestLoad: &load, LastLoaded: &load, AllowedActions: []string{"load_history"}}
	view.Source.SubmissionVersion = 4
	if view.Validate() != nil {
		t.Fatal("later task observation invalidated same-source archive")
	}
	other := load
	other.Source.TaskID = "other"
	view.LastLoaded = &other
	if view.Validate() == nil {
		t.Fatal("archive from different supplier task admitted")
	}
	view.LastLoaded = &load
	view.AllowedActions = []string{"load_history", "load_history"}
	if view.Validate() == nil {
		t.Fatal("duplicate action admitted")
	}
	page := CloudHistoryPage{Load: load, Items: []CloudHistoryEntry{{Sequence: 1, Kind: "assistant_text", Text: "hello"}, {Sequence: 2, Kind: "unsupported"}}, Total: 2, NextAfter: 2}
	if page.Validate() != nil {
		t.Fatal("valid local page rejected")
	}
	page.HasMore = true
	if page.Validate() == nil {
		t.Fatal("more local entries asserted without persisted source")
	}
	page.HasMore = false
	page.Items = nil
	if page.Validate() == nil {
		t.Fatal("null items accepted")
	}
}

func TestCloudHistoryArtifactContainsOnlyClosedMetadata(t *testing.T) {
	const payload = `{"sequence":1,"kind":"artifact","artifact_type":"media","artifact_event":"deleted"}`
	var entry CloudHistoryEntry
	if err := json.Unmarshal([]byte(payload), &entry); err != nil || entry.Validate() != nil {
		t.Fatal("replayed artifact metadata rejected")
	}
	got, err := json.Marshal(entry)
	if err != nil || string(got) != payload {
		t.Fatal("artifact metadata did not round-trip without text or download authority", string(got))
	}
	for _, rejected := range []string{
		strings.Replace(payload, `"media"`, `"git_bundle"`, 1),
		strings.Replace(payload, `"deleted"`, `"sealed"`, 1),
		strings.Replace(payload, `,"artifact_event":"deleted"`, "", 1),
		strings.Replace(payload, `"artifact"`, `"assistant_text"`, 1),
		strings.TrimSuffix(payload, "}") + `,"text":"private artifact contents"}`,
	} {
		var other CloudHistoryEntry
		if json.Unmarshal([]byte(rejected), &other) == nil && other.Validate() == nil {
			t.Fatal("unapproved artifact metadata or text accepted")
		}
	}
}
