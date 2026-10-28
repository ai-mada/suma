package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func inspectionFixtureEntry(run string) map[string]any {
	return map[string]any{
		"assignment": map[string]any{"run_id": run, "node_id": "develop", "attempt": 1, "fence": 7, "worker_id": "worker-private", "command": []string{"command-private"}, "config": map[string]any{"token": "configuration-private"}},
		"acked":      false,
		"progress":   []any{map[string]any{"acked": false, "message": "progress-private", "tool_event": map[string]any{"payload": "tool-event-private"}}, map[string]any{"acked": true}},
	}
}

func writeInspectionFixture(t *testing.T, dir string, content any) []byte {
	t.Helper()
	raw, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "journal.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestInspectJournalStateProjectsOnlySafeLocalFactsWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	pending := inspectionFixtureEntry("run-pending")
	pending["result"] = map[string]any{"outcome_unknown": true, "error": "error-private", "output": "output-private", "operation_id": "operation-private", "workspace_evidence": map[string]any{"secret": "evidence-private"}}
	pending["progress_truncated"] = true
	acked := inspectionFixtureEntry("run-acked")
	acked["result"] = map[string]any{"outcome_unknown": false}
	acked["acked"] = true
	acked["progress"] = []any{}
	inbox := inspectionFixtureEntry("run-inbox")
	artifact := inspectionFixtureEntry("run-artifact")
	artifact["artifact_pending"] = map[string]any{"token": "artifact-private"}
	candidate := inspectionFixtureEntry("run-candidate")
	candidate["candidate_pending"] = map[string]any{"token": "candidate-private"}
	before := writeInspectionFixture(t, dir, map[string]any{"entry-key-private": pending, "acked": acked, "inbox": inbox, "artifact": artifact, "candidate": candidate})
	beforeInfo, err := os.Stat(filepath.Join(dir, "journal.json"))
	if err != nil {
		t.Fatal(err)
	}
	// The reader must not acquire this lock or call OpenJournal, which would
	// generate unknown results for the uncompleted inbox and rewrite the file.
	unlock, err := lockJournal(filepath.Join(dir, "journal.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	got := InspectJournalState(dir)
	if got.SchemaVersion != 1 || got.Component != "local_execution_journal" || got.Status != "available" || !got.Complete || got.Limit != 100 || len(got.Items) != 5 {
		t.Fatalf("unexpected local inspection metadata: %+v", got)
	}
	want := JournalInspectionCounts{Entries: 5, PendingResults: 1, AcknowledgedResults: 1, UnknownResults: 1, PendingProgress: 4, UnresolvedInbox: 1, PendingArtifacts: 1, PendingCandidates: 1, TruncatedProgress: 1}
	if got.Counts != want {
		t.Fatalf("counts %+v, want %+v", got.Counts, want)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"entry-key-private", "worker-private", "command-private", "configuration-private", "progress-private", "tool-event-private", "error-private", "output-private", "operation-private", "evidence-private", "artifact-private", "candidate-private", dir} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("private journal content escaped through inspection: %s", forbidden)
		}
	}
	after, err := os.ReadFile(filepath.Join(dir, "journal.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("inspection rewrote the durable journal")
	}
	afterInfo, err := os.Stat(filepath.Join(dir, "journal.json"))
	if err != nil || !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatal("inspection changed journal metadata")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 2 {
		t.Fatal("inspection created additional state files")
	}
}

func TestInspectJournalStateMissingInvalidAndOversizedRemainReadOnly(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent", "state")
	if got := InspectJournalState(missing); got.Status != "missing" || got.Complete || len(got.Items) != 0 {
		t.Fatalf("missing journal is not a healthy complete snapshot: %+v", got)
	}
	if _, err := os.Stat(filepath.Dir(missing)); !os.IsNotExist(err) {
		t.Fatal("inspection created the missing directory")
	}
	for _, raw := range []string{"null", "[]", "{", "{} {}", `{"same":{"assignment":{}},"same":{"assignment":{}}}`, `{"entry":null}`, `{"entry":{"assignment":{}}}` + "\nprivate-trailer"} {
		t.Run(fmt.Sprintf("invalid-%x", raw), func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "journal.json"), []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			got := InspectJournalState(dir)
			if got.Status != "invalid" || got.Complete || got.Counts.Entries != 0 || len(got.Items) != 0 {
				t.Fatalf("invalid journal emitted a partial healthy summary: %+v", got)
			}
		})
	}
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "journal.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(journalInspectionMaxBytes + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	if got := InspectJournalState(dir); got.Status != "over_limit" || got.Complete || got.Counts.Entries != 0 {
		t.Fatalf("oversized journal was read or summarized: %+v", got)
	}
}

func TestInspectJournalStateBoundsItemsAndOmitsUnsafeIdentifiers(t *testing.T) {
	dir := t.TempDir()
	entries := map[string]any{}
	for i := 0; i < 105; i++ {
		entries[fmt.Sprintf("key-%03d", i)] = inspectionFixtureEntry(fmt.Sprintf("run-%03d", i))
	}
	unsafe := inspectionFixtureEntry("private\nidentifier-payload")
	unsafe["assignment"].(map[string]any)["node_id"] = "../../private-path"
	entries["unsafe-key"] = unsafe
	writeInspectionFixture(t, dir, entries)
	got := InspectJournalState(dir)
	if got.Status != "available" || got.Complete || got.Counts.Entries != 106 || len(got.Items) != 100 {
		t.Fatalf("bounded listing lost total/partial distinction: %+v", got)
	}
	if next := InspectJournalState(dir); !reflect.DeepEqual(next, got) {
		t.Fatal("unchanged local snapshots are not deterministic")
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "identifier-payload") || strings.Contains(string(encoded), "private-path") {
		t.Fatal("unsafe identifiers escaped the whitelist")
	}
}

func TestInspectJournalStateRejectsLinkedAndNonregularInputs(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "journal.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if got := InspectJournalState(dir); got.Status != "unreadable" {
		t.Fatalf("nonregular journal was accepted: %+v", got)
	}
	target := t.TempDir()
	writeInspectionFixture(t, target, map[string]any{})
	linked := t.TempDir()
	if err := os.Symlink(filepath.Join(target, "journal.json"), filepath.Join(linked, "journal.json")); err != nil {
		t.Skip("file symlink creation is unavailable on this host")
	}
	if got := InspectJournalState(linked); got.Status != "unreadable" {
		t.Fatalf("linked journal was read: %+v", got)
	}
	parent := t.TempDir()
	if err := os.Symlink(target, filepath.Join(parent, "state")); err != nil {
		t.Fatal(err)
	}
	if got := InspectJournalState(filepath.Join(parent, "state")); got.Status != "unreadable" {
		t.Fatalf("linked state directory was read: %+v", got)
	}
}
