package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestInventoryJournalRejectsMissingScopeWithoutChangingOriginalBytes(t *testing.T) {
	directory := t.TempDir()
	j, err := OpenJournal(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = j.NewInventory("http://127.0.0.1:4310", "project", "worker", nil); err != nil {
		t.Fatal(err)
	}
	path := j.inventoryPath()
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	delete(document, "scope_sha256")
	raw, _ = json.Marshal(document)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if check := InspectDesktopRunnerRenewalState(directory); check.Safe {
		t.Fatal("unscoped inventory was considered safe by the read-only inspector")
	}
	if opened, openErr := OpenJournal(directory); openErr == nil {
		_ = opened.Close()
		t.Fatal("unscoped uncertain inventory was accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(raw, after) {
		t.Fatal("unscoped original inventory was rewritten", err)
	}
}

func TestInventoryScopeDriftCannotReplayThroughSingleOrContinuousWorker(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	for _, kind := range []string{"control_path", "project", "worker"} {
		t.Run(kind, func(t *testing.T) {
			j, err := OpenJournal(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer j.Close()
			original, err := j.NewInventory(server.URL, "original-project", "worker", nil)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(j.inventoryPath())
			if err != nil {
				t.Fatal(err)
			}
			client := Client{BaseURL: server.URL, ProjectID: "original-project", Token: "registered-token"}
			worker := Worker{Client: &client, ID: "worker", Journal: j}
			switch kind {
			case "control_path":
				client.BaseURL += "/other-control"
			case "project":
				client.ProjectID = "other-project"
			case "worker":
				worker.ID = "other-worker"
			}
			if _, _, err = worker.ReportTools(context.Background()); err == nil {
				t.Fatal("single report accepted a changed scope")
			}
			if err = worker.refreshInventory(context.Background()); err == nil {
				t.Fatal("continuous worker accepted a changed scope")
			}
			if kind != "worker" {
				if _, err = client.ToolInventory(context.Background(), original); err == nil {
					t.Fatal("client sent inventory to a different scope")
				}
			}
			after, err := os.ReadFile(j.inventoryPath())
			if err != nil || !bytes.Equal(before, after) || j.PendingInventory() == nil || j.PendingInventory().OperationID != original.OperationID {
				t.Fatal("scope rejection changed the original unknown operation", err)
			}
		})
	}
	if calls != 0 {
		t.Fatal("scope rejection contacted a control endpoint", calls)
	}
}

func TestInventoryScopeRetainsExactControlPathAndNormalizesTrailingSlash(t *testing.T) {
	a, err := InventoryScopeSHA256("https://control.example/suma/", "project", "worker")
	if err != nil {
		t.Fatal(err)
	}
	b, err := InventoryScopeSHA256("https://control.example/suma", "project", "worker")
	if err != nil || a != b {
		t.Fatal("equivalent original control endpoints changed scope", err)
	}
	c, err := InventoryScopeSHA256("https://control.example/other", "project", "worker")
	if err != nil || a == c {
		t.Fatal("different API locations shared an inventory scope", err)
	}
}
