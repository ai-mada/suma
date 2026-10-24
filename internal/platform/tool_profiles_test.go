package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/toolcontract"
)

func toolTestProfile() toolcontract.Profile {
	return toolcontract.Profile{CommandID: "native-agent", Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "suma fixture v1"}
}
func toolFixture(t *testing.T) (*fixture, string, ToolProfile) {
	t.Helper()
	f := newFixture(t)
	if err := f.s.MigrateToolProfiles(context.Background()); err != nil {
		t.Fatal(err)
	}
	worker := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "tool-worker", "profiles": []any{}, "deployment": false}), 201)
	pr, err := f.s.CreateToolProfile(context.Background(), f.user, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "tool-worker", Name: "Local native", Profile: toolTestProfile(), IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return f, worker["token"].(string), pr
}
func TestToolProfileInventoryExpiryReplayAndCAS(t *testing.T) {
	f, token, p := toolFixture(t)
	ctx := context.Background()
	if p.State != "paused" || p.Available {
		t.Fatalf("created active %+v", p)
	}
	request := ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: p.Profile, Available: true}}}
	r, err := f.s.ReportToolInventory(ctx, f.user, f.project, token, request)
	if err != nil || !r.Acknowledged {
		t.Fatal(r, err)
	}
	p, err = f.s.CommandToolProfile(ctx, f.user, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: 1, IdempotencyKey: newID()})
	if err != nil || !p.Available {
		t.Fatal(p, err)
	}
	if _, err = f.s.CommandToolProfile(ctx, f.user, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "pause", ExpectedVersion: 1, IdempotencyKey: newID()}); err == nil {
		t.Fatal("stale CAS accepted")
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_tool_inventories SET observed_at='2000-01-01T00:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	replay, err := f.s.ReportToolInventory(ctx, f.user, f.project, token, request)
	if err != nil || replay.ObservedAt != r.ObservedAt {
		t.Fatal("receipt changed", replay, err)
	}
	p, err = f.s.GetToolProfile(ctx, f.user, f.project, p.ID)
	if err != nil || p.Available || !strings.Contains(p.Reason, "过期") {
		t.Fatal("stale report became online", p, err)
	}
	request.Items[0].Available = false
	request.Items[0].Reason = "disabled"
	if _, err = f.s.ReportToolInventory(ctx, f.user, f.project, token, request); err == nil {
		t.Fatal("changed inventory identity accepted")
	}
}
func TestToolInventoryWorkerAuthorityAndExactMatching(t *testing.T) {
	f, token, p := toolFixture(t)
	ctx := context.Background()
	req := ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: p.Profile, Available: true}}}
	wrong := req
	wrong.WorkerID = "unregistered"
	if _, err := f.s.ReportToolInventory(ctx, f.user, f.project, token, wrong); err == nil {
		t.Fatal("worker identity escaped")
	}
	if _, err := f.s.ReportToolInventory(ctx, f.user, f.project, token, req); err != nil {
		t.Fatal(err)
	}
	p, err := f.s.CommandToolProfile(ctx, f.user, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: 1, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.validateToolDispatchTx(ctx, tx, f.user, f.project, p.ID, p.Version, p.Profile.ProfileSHA256); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	tx.Rollback()
	req.OperationID = newID()
	req.Items[0].Profile.ExecutableSHA256 = strings.Repeat("c", 64)
	if _, err = f.s.ReportToolInventory(ctx, f.user, f.project, token, req); err != nil {
		t.Fatal(err)
	}
	p, err = f.s.GetToolProfile(ctx, f.user, f.project, p.ID)
	if err != nil || p.Available || !strings.Contains(p.Reason, "不匹配") {
		t.Fatal("profile drift allowed", p, err)
	}
	key := newID()
	p, err = f.s.CommandToolProfile(ctx, f.user, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "revoke", ExpectedVersion: p.Version, IdempotencyKey: key})
	if err != nil || p.State != "revoked" {
		t.Fatal(p, err)
	}
	if _, err = f.s.CommandToolProfile(ctx, f.user, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: p.Version, IdempotencyKey: newID()}); err == nil {
		t.Fatal("revoked profile reenabled")
	}
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), token) {
		t.Fatal("worker credential leaked")
	}
}
