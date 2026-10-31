package workbuddycloudcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCloudHistorySharingClosedSelectionRequest(t *testing.T) {
	raw := `{"project_id":"project","task_id":"task","expected_task_version":1,"expected_load_version":3,"sequences":[1,3],"preview_sha256":"` + strings.Repeat("a", 64) + `","idempotency_key":"share-operation"}`
	var req CloudHistoryShareRequest
	if Decode([]byte(raw), &req) != nil || req.Validate() != nil {
		t.Fatal("valid fixed share rejected")
	}
	for _, bad := range []string{
		strings.Replace(raw, `[1,3]`, `[3,1]`, 1), strings.Replace(raw, `[1,3]`, `[1,1]`, 1), strings.Replace(raw, `[1,3]`, `[]`, 1), strings.Replace(raw, `[1,3]`, `null`, 1), strings.Replace(raw, `[1,3]`, `[1,2,3,4,5,6,7,8,9]`, 1),
		strings.Replace(raw, `"task_id":"task"`, `"task_id":"task","body":"forged body"`, 1), strings.Replace(raw, `"task_id":"task"`, `"task_id":"task","actor_kind":"assistant"`, 1), strings.Replace(raw, `"expected_load_version":3`, `"expected_load_version":3,"expected_load_version":3`, 1), strings.Replace(raw, `"expected_task_version":1`, `"expected_task_version":null`, 1),
	} {
		if Decode([]byte(bad), &req) == nil && req.Validate() == nil {
			t.Fatal("open or invalid request accepted")
		}
	}
}

func TestCloudHistorySharingPreviewAndSafeProvenance(t *testing.T) {
	entries := []CloudHistoryEntry{{Sequence: 1, Kind: "user_text", Text: "  literal 中文  "}, {Sequence: 3, Kind: "assistant_text", Text: "line\ntwo"}}
	body := entries[0].Text + "\n\n" + entries[1].Text
	selected := []CloudHistorySharedEntry{{Sequence: 1, Kind: "user_text", TextSHA256: CloudHistoryTextSHA256(entries[0].Text)}, {Sequence: 3, Kind: "assistant_text", TextSHA256: CloudHistoryTextSHA256(entries[1].Text)}}
	selection, err := CloudHistorySelectionSHA256(selected)
	if err != nil {
		t.Fatal(err)
	}
	preview := CloudHistorySharePreview{ProjectID: "project", TaskID: "task", TaskTitle: "Target task", TaskVersion: 1, LoadID: "load", LoadVersion: 3, LoadState: "limited", Coverage: "unverified", Source: collaborationSource(), Entries: entries, Body: body, BodySHA256: CloudHistoryTextSHA256(body), SelectionSHA256: selection, PreviewSHA256: strings.Repeat("b", 64), SharingScope: "project_task_readers"}
	if preview.Validate() != nil {
		t.Fatal("literal preview rejected")
	}
	preview.Body = strings.TrimSpace(body)
	if preview.Validate() == nil {
		t.Fatal("preview silently normalized literal body")
	}
	provenance := CloudHistorySharedObservation{LoadID: "load", LoadVersion: 3, LoadState: "limited", Coverage: "unverified", SourceRunID: "run", SourceNodeID: "node", SourceSubmissionID: "submission", Entries: selected, BodySHA256: CloudHistoryTextSHA256(body), SelectionSHA256: selection}
	if provenance.Validate() != nil {
		t.Fatal("safe provenance rejected")
	}
	encoded, err := json.Marshal(provenance)
	if err != nil {
		t.Fatal(err)
	}
	for _, secretField := range []string{"task_id", "request_sha256", "operation_id", "ticket", "source\"", "text\""} {
		if strings.Contains(string(encoded), `"`+secretField) {
			t.Fatal("private source field projected")
		}
	}
	provenance.Entries[0].TextSHA256 = strings.Repeat("c", 64)
	if provenance.Validate() == nil {
		t.Fatal("selection digest did not bind ordered entries")
	}
}

func TestCloudHistorySharingUsesExistingTaskTitleAndTextBoundaries(t *testing.T) {
	if ValidCloudHistoryShareBody("literal\bhidden control") {
		t.Fatal("source-forbidden control accepted as share body")
	}
	entries := []CloudHistoryEntry{{Sequence: 1, Kind: "user_text", Text: "literal"}}
	selection, _ := CloudHistorySelectionSHA256([]CloudHistorySharedEntry{{Sequence: 1, Kind: "user_text", TextSHA256: CloudHistoryTextSHA256("literal")}})
	preview := CloudHistorySharePreview{ProjectID: "project", TaskID: "task", TaskTitle: strings.Repeat("x", 500), TaskVersion: 1, LoadID: "load", LoadVersion: 3, LoadState: "loaded", Coverage: "unverified", Source: collaborationSource(), Entries: entries, Body: "literal", BodySHA256: CloudHistoryTextSHA256("literal"), SelectionSHA256: selection, PreviewSHA256: strings.Repeat("a", 64), SharingScope: "project_task_readers"}
	if preview.Validate() != nil {
		t.Fatal("existing 500-byte task title cannot be previewed")
	}
}
