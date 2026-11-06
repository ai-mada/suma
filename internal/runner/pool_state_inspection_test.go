package runner

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	poolInspectionControl = "http://127.0.0.1:17422"
	poolInspectionPoolID  = "pool-identity-private"
	poolInspectionRunner  = "runner-identity-private"
	poolInspectionWorker  = "pool-" + poolInspectionRunner
	poolInspectionTenant  = "tenant-identity-private"
	poolInspectionProject = "project-identity-private"
)

func writePoolInspectionFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	config := digest([]byte("configuration-private"))
	binding, err := json.Marshal([]any{digest([]byte(poolInspectionControl)), poolInspectionPoolID, poolInspectionRunner, 1, config})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "identity.json"), binding, 0600); err != nil {
		t.Fatal(err)
	}
	return dir, config
}

func makePoolInspectionSlot(t *testing.T, pool, slot string) string {
	t.Helper()
	dir := filepath.Join(pool, "slots", slot)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func recordPoolInspectionClaim(t *testing.T, dir, config, slot, stage string) *poolClaimCheckpoint {
	t.Helper()
	j := &Journal{path: filepath.Join(dir, "journal.json")}
	request := PoolClaimRequest{
		PoolID: poolInspectionPoolID, RunnerID: poolInspectionRunner, WorkerID: poolInspectionWorker, SlotID: slot,
		Scopes: []PoolCapabilityScope{{TenantID: poolInspectionTenant, ProjectID: poolInspectionProject, Capabilities: []string{"command:capability-private"}}},
	}
	checkpoint, err := beginPoolClaim(j, poolInspectionControl, config, 1, request)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint.Responded = stage != "request_recorded"
	checkpoint.Complete = stage == "complete"
	if stage == "response_recorded" {
		a := poolTestAssignment()
		a.PoolID, a.PoolRunnerID, a.WorkerID, a.PoolSlotID = poolInspectionPoolID, poolInspectionRunner, poolInspectionWorker, slot
		a.TenantID, a.ProjectID = poolInspectionTenant, poolInspectionProject
		a.RunID, a.NodeID = "claim-run-private", "claim-node-private"
		checkpoint.Assignment = &a
	}
	if err := savePoolClaim(j, checkpoint); err != nil {
		t.Fatal(err)
	}
	return checkpoint
}

type poolInspectionFile struct {
	Contents string
	Modified time.Time
}

func snapshotPoolInspectionFiles(t *testing.T, dir string) map[string]poolInspectionFile {
	t.Helper()
	files := map[string]poolInspectionFile{}
	if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if filepath.Ext(path) == ".lock" {
			// Windows denies even shared reads of active lock files. Record
			// their presence without asking for a second handle.
			files[path] = poolInspectionFile{}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err == nil {
			files[path] = poolInspectionFile{Contents: string(data), Modified: info.ModTime()}
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func TestInspectPoolStateProjectsSafeSlotCountsAndClaimStages(t *testing.T) {
	pool, config := writePoolInspectionFixture(t)
	stages := map[string]string{"2": "request_recorded", "9": "complete", "10": "response_recorded", "127": "missing"}
	for _, slot := range []string{"127", "10", "9", "2"} {
		dir := makePoolInspectionSlot(t, pool, slot)
		pending := inspectionFixtureEntry("journal-run-private")
		pending["result"] = map[string]any{"outcome_unknown": true, "output": "journal-output-private"}
		entries := map[string]any{"journal-key-private": pending}
		if slot == "10" {
			key, closed := closedInspectionFixture("journal-closed-private")
			entries[key] = closed
		}
		writeInspectionFixture(t, dir, entries)
		if stage := stages[slot]; stage != "missing" {
			recordPoolInspectionClaim(t, dir, config, slot, stage)
		}
	}
	// A running pool and slot may own their locks. Inspection is read-only
	// and must also leave the incomplete original claim untouched.
	for _, path := range []string{filepath.Join(pool, "pool.lock"), filepath.Join(pool, "slots", "10", "journal.lock")} {
		unlock, err := lockJournal(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = unlock() })
	}
	before := snapshotPoolInspectionFiles(t, pool)
	got := InspectPoolState(pool)
	if got.SchemaVersion != 1 || got.Component != "local_pool_runner_state" || got.Status != "available" || !got.Complete || len(got.Slots) != 4 {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
	counts := JournalInspectionCounts{Entries: 1, PendingResults: 1, UnknownResults: 1, PendingProgress: 1}
	for i, slot := range []string{"2", "9", "10", "127"} {
		row := got.Slots[i]
		want := counts
		if slot == "10" {
			want.Entries, want.ClosedResults = 2, 1
		}
		if row.SlotID != slot || row.ClaimState != stages[slot] || row.JournalStatus != "available" || row.JournalCounts == nil || *row.JournalCounts != want {
			t.Fatalf("slot %s: unexpected projection %+v", slot, row)
		}
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private", `"items"`, `"run_id"`, `"node_id"`, `"operation_id"`, `"attempt"`, `"fence"`, poolInspectionControl} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("pool projection exposed %q", forbidden)
		}
	}
	if after := snapshotPoolInspectionFiles(t, pool); !reflect.DeepEqual(before, after) {
		t.Fatal("inspection created files or changed the existing pool state")
	}
}

func TestInspectPoolStateKeepsHealthyRowsWhenOneSourceIsInvalid(t *testing.T) {
	pool, config := writePoolInspectionFixture(t)
	for _, slot := range []string{"0", "1", "2", "3", "4", "5"} {
		dir := makePoolInspectionSlot(t, pool, slot)
		switch slot {
		case "0":
			writeInspectionFixture(t, dir, map[string]any{})
			recordPoolInspectionClaim(t, dir, config, "1", "request_recorded")
		case "1":
			writeInspectionFixture(t, dir, map[string]any{})
			recordPoolInspectionClaim(t, dir, digest([]byte("different-config")), slot, "request_recorded")
		case "2":
			writeInspectionFixture(t, dir, map[string]any{})
			checkpoint := recordPoolInspectionClaim(t, dir, config, slot, "response_recorded")
			checkpoint.Assignment.PoolSlotID = "1"
			if err := savePoolClaim(&Journal{path: filepath.Join(dir, "journal.json")}, checkpoint); err != nil {
				t.Fatal(err)
			}
		case "3":
			if err := os.WriteFile(filepath.Join(dir, "journal.json"), []byte(`{"truncated":`), 0600); err != nil {
				t.Fatal(err)
			}
			recordPoolInspectionClaim(t, dir, config, slot, "complete")
		case "4":
			writeInspectionFixture(t, dir, map[string]any{})
			file, err := os.Create(filepath.Join(dir, "pool-claim.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := file.Truncate((2 << 20) + 1); err != nil {
				_ = file.Close()
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
		case "5":
			recordPoolInspectionClaim(t, dir, config, slot, "request_recorded")
		}
	}
	makePoolInspectionSlot(t, pool, "01")
	got := InspectPoolState(pool)
	if got.Status != "available" || got.Complete || len(got.Slots) != 6 {
		t.Fatalf("unreadable rows discarded the useful snapshot or implied completeness: %+v", got)
	}
	wantJournal := []string{"available", "available", "available", "invalid", "available", "missing"}
	wantClaim := []string{"invalid", "invalid", "invalid", "complete", "over_limit", "request_recorded"}
	for i, row := range got.Slots {
		if row.JournalStatus != wantJournal[i] || row.ClaimState != wantClaim[i] || (row.JournalCounts != nil) != (wantJournal[i] == "available") {
			t.Fatalf("slot %s: unexpected projection %+v", row.SlotID, row)
		}
	}
}

func TestInspectPoolStateNeverCreatesOrFollowsSelectedState(t *testing.T) {
	pool, _ := writePoolInspectionFixture(t)
	missing := filepath.Join(pool, "missing")
	if got := InspectPoolState(missing); got.Status != "missing" || got.Complete || len(got.Slots) != 0 {
		t.Fatalf("missing pool: %+v", got)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("inspection created missing pool state")
	}
	if got := InspectPoolState(pool); got.Status != "available" || !got.Complete || len(got.Slots) != 0 {
		t.Fatalf("empty maintenance-only pool: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(pool, "slots")); !os.IsNotExist(err) {
		t.Fatal("inspection created the slot directory")
	}
	t.Run("linked slot", func(t *testing.T) {
		outside := t.TempDir()
		writeInspectionFixture(t, outside, map[string]any{"foreign-private": inspectionFixtureEntry("foreign-run-private")})
		good := makePoolInspectionSlot(t, pool, "0")
		writeInspectionFixture(t, good, map[string]any{})
		if err := os.Symlink(outside, filepath.Join(pool, "slots", "1")); err != nil {
			t.Skipf("directory symlinks are unavailable: %v", err)
		}
		got := InspectPoolState(pool)
		if got.Status != "available" || got.Complete || len(got.Slots) != 2 || got.Slots[0].SlotID != "0" || got.Slots[0].JournalStatus != "available" {
			t.Fatalf("linked slot hid the healthy snapshot: %+v", got)
		}
		row := got.Slots[1]
		if row.SlotID != "1" || row.JournalStatus != "unreadable" || row.JournalCounts != nil || row.ClaimState != "unreadable" {
			t.Fatalf("linked slot was followed: %+v", row)
		}
		encoded, err := json.Marshal(got)
		if err != nil || strings.Contains(string(encoded), "foreign") {
			t.Fatal("linked slot contents escaped")
		}
	})
}
