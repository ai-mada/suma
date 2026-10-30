package platform

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/toolcontract"
)

func TestToolInventorySubsecondDatabaseRoundTripPreservesFreshnessAndOriginalReceipt(t *testing.T) {
	f, token, profile := toolFixture(t)
	ctx := context.Background()
	actor := toolActorForTest(t, f, token)
	// Align real SQLite time early in a second, leaving room for two actual
	// transactions. No application clock or fabricated observation is written.
	deadline := time.Now().Add(5 * time.Second)
	for {
		var fraction string
		if err := f.s.DB.QueryRow(`SELECT strftime('%f','now')`).Scan(&fraction); err != nil {
			t.Fatal(err)
		}
		parts := strings.Split(fraction, ".")
		millis, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil {
			t.Fatal(err)
		}
		if millis >= 100 && millis < 250 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("real database clock did not reach the bounded same-second window")
		}
		time.Sleep(5 * time.Millisecond)
	}
	firstRequest := ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: profile.Profile, Reason: "Initial installation observation"}}}
	first, err := f.s.ReportToolInventory(ctx, actor, f.project, firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	firstAt, err := time.Parse(time.RFC3339Nano, first.ObservedAt)
	if err != nil || firstAt.Nanosecond() == 0 {
		t.Fatalf("real database subsecond report was truncated: %q %v", first.ObservedAt, err)
	}
	time.Sleep(5 * time.Millisecond)
	second, err := f.s.ReportToolInventory(ctx, actor, f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: profile.Profile, Available: true}}})
	if err != nil {
		t.Fatal(err)
	}
	secondAt, err := time.Parse(time.RFC3339Nano, second.ObservedAt)
	if err != nil || secondAt.Unix() != firstAt.Unix() || !secondAt.After(firstAt) {
		t.Fatalf("two real same-second reports lost their order: %q %q %v", first.ObservedAt, second.ObservedAt, err)
	}
	discovery, err := f.s.DiscoverTools(ctx, f.actor, f.project, profile.WorkerID)
	if err != nil || discovery.State != "fresh" || discovery.ObservedAt != second.ObservedAt || len(discovery.Items) != 1 || !discovery.Items[0].Available {
		t.Fatalf("subsecond report did not round-trip as fresh: %+v %v", discovery, err)
	}
	asOf, err := time.Parse(time.RFC3339Nano, discovery.AsOf)
	if err != nil || asOf.Before(secondAt) {
		t.Fatal("discovery clock lost database precision", err)
	}
	expires, err := time.Parse(time.RFC3339Nano, discovery.ExpiresAt)
	if err != nil || expires.Sub(secondAt) != 60*time.Second {
		t.Fatal("subsecond TTL was shortened or extended", err)
	}
	var stored string
	if err = f.s.DB.QueryRow(`SELECT observed_at FROM suma_tool_inventories WHERE worker_id=?`, profile.WorkerID).Scan(&stored); err != nil || stored != second.ObservedAt {
		t.Fatal("persistent report timestamp differs", err)
	}
	replayed, err := f.s.ReportToolInventory(ctx, actor, f.project, firstRequest)
	if err != nil || replayed.ObservedAt != first.ObservedAt {
		t.Fatal("original receipt was refreshed", err)
	}
	if err = f.s.DB.QueryRow(`SELECT observed_at FROM suma_tool_inventories WHERE worker_id=?`, profile.WorkerID).Scan(&stored); err != nil || stored != second.ObservedAt {
		t.Fatal("old receipt replay replaced the latest observation", err)
	}
	enabled, err := f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: 1, IdempotencyKey: newID()})
	if err != nil || !enabled.Available {
		t.Fatalf("immediate current dispatch qualification treated fractional database time as future: %+v %v", enabled, err)
	}
}
