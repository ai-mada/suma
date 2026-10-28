package workspaceoperationcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func eventFixture() (Identity, Event) {
	i := Identity{TenantID: "tenant", ProjectID: "project", RunID: "run", NodeID: "node", WorkerID: "worker", Attempt: 1, Fence: 2}
	e := Event{Protocol: Protocol, Source: Source, SchemaVersion: 1, Sequence: 1, OperationID: "invocation-1", Revision: 1, Action: "read", Phase: "started", StartedAt: "2026-10-29T01:00:00Z", RecordedAt: "2026-10-29T01:00:00Z", RequestSHA256: strings.Repeat("a", 64), PolicySHA256: strings.Repeat("b", 64), ProfileSHA256: strings.Repeat("b", 64)}
	e.EventID = CanonicalEventID(i, e)
	return i, e
}

func TestWorkspaceOperationClosedWireAndBoundIdentity(t *testing.T) {
	i, e := eventFixture()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = DecodeEvent(raw); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.TrimSuffix(string(raw), "}") + `,"command":"echo secret"}`,
		strings.Replace(string(raw), `"sequence":1`, `"sequence":1,"sequence":2`, 1),
		strings.Replace(string(raw), `"sequence":1`, `"sequence":1.0`, 1),
		strings.TrimSuffix(string(raw), "}") + `,"path":null}`,
		strings.TrimSuffix(string(raw), "}") + `,"path":"../outside"}`,
		strings.TrimSuffix(string(raw), "}") + `,"path":"C:/private"}`,
		strings.TrimSuffix(string(raw), "}") + `,"path":"\ud800"}`,
	} {
		if _, err = DecodeEvent([]byte(bad)); err == nil {
			t.Fatal("unsafe host wire accepted", bad)
		}
	}
	i.ProjectID = "other"
	if err = ValidateBoundEvent(i, e); err == nil {
		t.Fatal("host event crossed original project")
	}
	_, e = eventFixture()
	e.Phase, e.Revision, e.Sequence, e.CompletedAt = "completed", 2, 2, "2026-10-29T01:00:01Z"
	e.RecordedAt, e.Action = e.CompletedAt, "check"
	exit := 1
	e.ExitCode = &exit
	e.EventID = CanonicalEventID(i, e)
	if err = ValidateBoundEvent(i, e); err != nil {
		t.Fatal("completed failed check was confused with passed", err)
	}
	e.Phase = "unknown"
	if err = ValidateEvent(e); err == nil {
		t.Fatal("unknown event claimed completion time")
	}
}
