package runner

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/ai-mada/suma/internal/workbuddy"
)

func workBuddySubmissionAssignment() Assignment {
	return Assignment{TenantID: "tenant-143", ProjectID: "project-143", RunID: "run-143", NodeID: "node-143", Attempt: 1, Fence: 1, WorkerID: "worker-143", Type: "external_workflow", Config: map[string]any{"plugin": "workbuddy"}}
}

func workBuddyTestClient(t *testing.T, server *httptest.Server) *workbuddy.Client {
	t.Helper()
	client, err := workbuddy.NewClient(workbuddy.Config{BaseURL: server.URL, Token: "local-fixture-token", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func workBuddyUnknown(t *testing.T, err error) {
	t.Helper()
	var failure *workbuddy.Error
	if !errors.As(err, &failure) || !failure.OutcomeUnknown {
		t.Fatalf("expected explicit unknown effect, got %v", err)
	}
}

func TestWorkBuddySubmissionRestartReadsOriginalTaskWithoutCreatingAgain(t *testing.T) {
	var creates, reads atomic.Int32
	var failRead atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/openapi/v2/tasks":
			creates.Add(1)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"task_id":"task-143","status":"working","token":"must-not-persist"}`))
		case r.Method == "GET" && r.URL.Path == "/openapi/v2/tasks/task-143":
			reads.Add(1)
			if failRead.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"task_id":"task-143","status":"completed"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := workBuddyTestClient(t, server)
	directory := t.TempDir()
	journal, err := OpenJournal(directory)
	if err != nil {
		t.Fatal(err)
	}
	assignment, request := workBuddySubmissionAssignment(), workbuddy.CreateRequest{Prompt: "Inspect the approved task", Name: "Scoped fixture"}
	if err = journal.Accept(assignment); err != nil {
		t.Fatal(err)
	}
	task, err := submitWorkBuddyTask(context.Background(), journal, assignment, client, request, "credential-143", 1)
	if err != nil || task.ID != "task-143" {
		t.Fatal(task, err)
	}
	if err = journal.Close(); err != nil {
		t.Fatal(err)
	}
	journal, err = OpenJournal(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	task, err = submitWorkBuddyTask(context.Background(), journal, assignment, client, request, "credential-143", 1)
	if err != nil || task.ID != "task-143" || task.Status != "completed" || creates.Load() != 1 || reads.Load() != 1 {
		t.Fatal("recovery must only read the exact remote task", task, err, creates.Load(), reads.Load())
	}
	// A new request or credential revision cannot replace the fixed submission.
	_, err = submitWorkBuddyTask(context.Background(), journal, assignment, client, workbuddy.CreateRequest{Prompt: "Expanded scope"}, "credential-143", 1)
	workBuddyUnknown(t, err)
	_, err = submitWorkBuddyTask(context.Background(), journal, assignment, client, request, "credential-143", 2)
	workBuddyUnknown(t, err)
	if creates.Load() != 1 || reads.Load() != 1 {
		t.Fatal("scope drift reached the provider")
	}
	failRead.Store(true)
	task, err = submitWorkBuddyTask(context.Background(), journal, assignment, client, request, "credential-143", 1)
	if err == nil || task.ID != "task-143" || creates.Load() != 1 || reads.Load() != 2 {
		t.Fatal("failed query erased the remote identity or retried a create", task, err)
	}
}

func TestWorkBuddySubmissionLostResponseIsNeverRepostedAfterRestart(t *testing.T) {
	var creates atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/openapi/v2/tasks" {
			t.Error("unexpected reconciliation network request")
			w.WriteHeader(500)
			return
		}
		creates.Add(1)
		// Simulates a provider that created a task but lost its JSON receipt.
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"task_id":`))
	}))
	defer server.Close()
	directory := t.TempDir()
	journal, err := OpenJournal(directory)
	if err != nil {
		t.Fatal(err)
	}
	assignment, request := workBuddySubmissionAssignment(), workbuddy.CreateRequest{Prompt: "Initial fixed scope"}
	if err = journal.Accept(assignment); err != nil {
		t.Fatal(err)
	}
	client := workBuddyTestClient(t, server)
	_, err = submitWorkBuddyTask(context.Background(), journal, assignment, client, request, "credential-143", 1)
	workBuddyUnknown(t, err)
	if err = journal.Close(); err != nil {
		t.Fatal(err)
	}
	journal, err = OpenJournal(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	_, err = submitWorkBuddyTask(context.Background(), journal, assignment, client, request, "credential-143", 1)
	workBuddyUnknown(t, err)
	if creates.Load() != 1 {
		t.Fatal("unknown creation was repeated", creates.Load())
	}
}

func TestWorkBuddySubmissionRejectsInvalidOrUndurableWorkBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(500)
	}))
	defer server.Close()
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	client, assignment := workBuddyTestClient(t, server), workBuddySubmissionAssignment()
	_, err = submitWorkBuddyTask(context.Background(), journal, assignment, client, workbuddy.CreateRequest{Prompt: "Not in durable inbox"}, "credential-143", 1)
	if err == nil {
		t.Fatal("unaccepted assignment submitted")
	}
	if err = journal.Accept(assignment); err != nil {
		t.Fatal(err)
	}
	_, err = submitWorkBuddyTask(context.Background(), journal, assignment, client, workbuddy.CreateRequest{}, "credential-143", 1)
	if err == nil {
		t.Fatal("empty request submitted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = submitWorkBuddyTask(ctx, journal, assignment, client, workbuddy.CreateRequest{Prompt: "Cancelled before prepare"}, "credential-143", 1)
	if err == nil || requests.Load() != 0 {
		t.Fatal("invalid request reached provider", err, requests.Load())
	}
	if _, exists, readErr := journal.WorkBuddySubmission(assignment); readErr != nil || exists {
		t.Fatal("local validation created a submission record", exists, readErr)
	}
}

func TestWorkBuddySubmissionKnownTaskSurvivesCheckpointFailureWithoutReposting(t *testing.T) {
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	originalPath := journal.path
	assignment, request := workBuddySubmissionAssignment(), workbuddy.CreateRequest{Prompt: "One create even if the task checkpoint fails"}
	if err = journal.Accept(assignment); err != nil {
		t.Fatal(err)
	}
	var creates atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		creates.Add(1)
		// The preparation is already durable. Simulate storage becoming
		// unavailable between the remote effect and storing its returned ID.
		journal.mu.Lock()
		journal.path = filepath.Join(originalPath, "not-a-directory", "journal.json")
		journal.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"task_id":"known-but-not-checkpointed","status":"working"}`))
	}))
	defer server.Close()
	client := workBuddyTestClient(t, server)
	task, err := submitWorkBuddyTask(context.Background(), journal, assignment, client, request, "credential-143", 1)
	workBuddyUnknown(t, err)
	if task.ID != "known-but-not-checkpointed" {
		t.Fatal("caller lost the known remote identity", task)
	}
	journal.mu.Lock()
	journal.path = originalPath
	journal.mu.Unlock()
	_, err = submitWorkBuddyTask(context.Background(), journal, assignment, client, request, "credential-143", 1)
	workBuddyUnknown(t, err)
	if creates.Load() != 1 {
		t.Fatal("failed checkpoint led to a second creation")
	}
	checkpoint, exists, err := journal.WorkBuddySubmission(assignment)
	if err != nil || !exists || checkpoint.State != "prepared" || checkpoint.TaskID != "" {
		t.Fatal("failed storage changed the durable prepared checkpoint", checkpoint, exists, err)
	}
}
