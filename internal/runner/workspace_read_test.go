package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkspaceReadSnapshotRejectsIdentityAndDigestChanges(t *testing.T) {
	for _, change := range []string{"stable", "same-length-content", "replacement"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			name := filepath.Join(dir, "file.txt")
			if err := os.WriteFile(name, []byte("base\n"), 0600); err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			file, err := root.Open("file.txt")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			initial, err := file.Stat()
			if err != nil {
				t.Fatal(err)
			}
			if change == "same-length-content" {
				if err = os.WriteFile(name, []byte("next\n"), 0600); err != nil {
					t.Fatal(err)
				}
				// Matching size and timestamp alone cannot prove unchanged bytes.
				if err = os.Chtimes(name, initial.ModTime(), initial.ModTime()); err != nil {
					t.Fatal(err)
				}
			}
			if change == "replacement" {
				if err = os.Rename(name, filepath.Join(dir, "previous.txt")); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(name, []byte("base\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err = nativeReadSnapshotCurrent(root, "file.txt", file, initial, []byte("base\n"))
			if change == "stable" && err != nil || change != "stable" && observationErrorCode(err) != "file_changed" {
				t.Fatalf("snapshot %s was incorrectly admitted: %v", change, err)
			}
		})
	}
}

func TestReadOnlyWorkspaceRegistrationNeverClaimsWritesOrReopensAfterClose(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "src")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	assignment := Assignment{RunID: "read-run", NodeID: "read-node", WorkerID: "read-worker", Type: "agent", Attempt: 1, Fence: 2, Config: map[string]any{"tool_profile_id": "read-profile"}}
	workspace := &attemptWorkspace{AttemptKey: attemptKey(assignment), Assignment: assignment, Directory: directory, Source: source, State: "prepared"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, err := openLiveWorkspaceRead(ctx, assignment, ExecConfig{}, workspace, strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	registry := NewWorkspaceObservationRegistry("project")
	unregister, err := registry.registerRead(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()
	scope, policy, active := registry.active()
	if !active {
		t.Fatal("current read-only workspace was unavailable")
	}
	if _, _, write := registry.activeWrite(); write {
		t.Fatal("read-only registration supplied write admission")
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	brokerCtx, stop := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer stop()
	runWorkspaceWriteBroker(brokerCtx, &Client{BaseURL: server.URL, ProjectID: "project"}, journal, assignment, registry, time.Millisecond)
	if calls.Load() != 0 {
		t.Fatal("read-only registry initiated a write claim")
	}
	lease := codexObservationLease(scope, policy)
	workspace.Assignment.Fence++
	if observed := registry.Observe(context.Background(), lease); observed.ErrorCode != "workspace_closed" || observed.ObservationResult != nil {
		t.Fatal("changed attempt identity remained readable")
	}
	workspace.Assignment.Fence--
	cancel()
	if observed := registry.Observe(context.Background(), lease); observed.ErrorCode != "workspace_closed" || observed.ObservationResult != nil {
		t.Fatal("cancelled attempt remained readable")
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "file.txt"), []byte("later source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if observed := registry.Observe(context.Background(), lease); observed.ErrorCode != "workspace_closed" || observed.ObservationResult != nil || reader.root != nil {
		t.Fatal("closed read handle reopened historical source")
	}
}
