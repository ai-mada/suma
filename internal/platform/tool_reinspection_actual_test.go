package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/runner"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ic "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

// This fault happens after the real server commits and returns a successful
// response. It does not forge a server result or make the probe run again.
type inspectionACKLoss194 struct {
	mu                               sync.Mutex
	createLost, inventoryLost        bool
	inspectionID, inventoryOperation string
	inventories                      []map[string]string
}

func (f *inspectionACKLoss194) RoundTrip(request *http.Request) (*http.Response, error) {
	var body map[string]any
	if request.GetBody != nil {
		r, err := request.GetBody()
		if err != nil {
			return nil, err
		}
		defer r.Close()
		if err = json.NewDecoder(r).Decode(&body); err != nil && err != io.EOF {
			return nil, err
		}
	}
	response, err := http.DefaultTransport.RoundTrip(request)
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return response, err
	}
	f.mu.Lock()
	drop := false
	if request.URL.Path == "/api/v1/runner/tool-inventory" {
		op, _ := body["operation_id"].(string)
		id := ""
		if ref, ok := body["inspection"].(map[string]any); ok {
			id, _ = ref["inspection_id"].(string)
		}
		f.inventories = append(f.inventories, map[string]string{"operation_id": op, "inspection_id": id})
	}
	if request.Method == http.MethodPost && strings.HasPrefix(request.URL.Path, "/api/v1/tool-profiles/") && strings.HasSuffix(request.URL.Path, "/inspections") && !f.createLost {
		f.createLost, drop = true, true
	}
	if request.URL.Path == "/api/v1/runner/tool-inventory" && body["inspection"] != nil && !f.inventoryLost {
		f.inventoryLost, drop = true, true
		ref, _ := body["inspection"].(map[string]any)
		f.inspectionID, _ = ref["inspection_id"].(string)
		f.inventoryOperation, _ = body["operation_id"].(string)
	}
	f.mu.Unlock()
	if !drop {
		return response, nil
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	return nil, errors.New("controlled loss of a persisted acknowledgement")
}

// A stdlib-only, explicitly owned Harness protocol probe exercises real Node
// child processes. It supports no execute/model/installation operation and is
// not evidence of a vendor account or production Harness deployment.
const inspectionProbe194 = `import fs from 'node:fs';
const args=process.argv.slice(2),counter=process.env.SUMA_INSPECTION_COUNTER;
if(!counter)throw Error('missing owned counter');
if(args.length===1&&args[0]==='--version'){fs.appendFileSync(counter,'version\n');console.log('owned-inspection-fixture-194');}
else if(args.join(' ')==='--profile multica --probe'){fs.appendFileSync(counter,'probe\n');console.log(JSON.stringify({v:1,type:'probe',runtime:'dsh',plugin_version:'fixture-194',protocol_version:1}));}
else {fs.appendFileSync(counter,'unexpected\n');process.exitCode=2;}
`

func TestToolReinspectionActualWorkerJournalAndLostAcknowledgements(t *testing.T) {
	if os.Getenv("SUMA_TOOL_REINSPECTION_194_ACTUAL") != "1" {
		t.Skip("explicit isolated Worker reinspection journey required")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err = os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("repository unavailable")
		}
		root = parent
	}
	evidenceName := "actual"
	if suffix := os.Getenv("SUMA_TOOL_REINSPECTION_194_EVIDENCE_SUFFIX"); suffix != "" {
		if suffix != "final" {
			t.Fatal("only the separately preserved final evidence directory is allowed")
		}
		evidenceName += "-" + suffix
	}
	dir := filepath.Join(root, ".cache", "tool-reinspection-194", evidenceName)
	if _, err = os.Stat(filepath.Join(dir, "receipt.json")); !os.IsNotExist(err) {
		t.Fatal("retain the original actual receipt; do not reseed")
	}
	if err = os.MkdirAll(filepath.Join(dir, "tmp"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TEMP", "TMP", "TMPDIR"} {
		t.Setenv(name, filepath.Join(dir, "tmp"))
	}
	node := os.Getenv("SUMA_TOOL_REINSPECTION_194_NODE")
	if !filepath.IsAbs(node) {
		t.Fatal("explicit existing Node executable required")
	}
	repository := filepath.Join(dir, "repository")
	if err = os.Mkdir(repository, 0700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		argv := append([]string{"-c", "core.hooksPath=" + filepath.Join(dir, "disabled-hooks"), "-c", "commit.gpgsign=false", "-c", "user.name=Owned Inspection Fixture", "-c", "user.email=inspection@example.invalid"}, args...)
		command := exec.Command("git", argv...)
		command.Dir = repository
		out, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("owned Git fixture failed: %v", err)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-b", "main")
	if os.WriteFile(filepath.Join(repository, ".gitignore"), []byte(".cache/\n"), 0600) != nil || os.WriteFile(filepath.Join(repository, "README.md"), []byte("Owned read-only inspection transport fixture.\n"), 0600) != nil {
		t.Fatal("fixture source unavailable")
	}
	git("add", "--", ".gitignore", "README.md")
	git("commit", "-m", "owned inspection fixture")
	base := git("rev-parse", "HEAD")
	entry, counter := filepath.Join(dir, "probe.mjs"), filepath.Join(dir, "probe-count.txt")
	if err = os.WriteFile(entry, []byte(inspectionProbe194), 0600); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t)
	var requestsMu sync.Mutex
	requests := []map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsMu.Lock()
		requests = append(requests, map[string]string{"method": r.Method, "path": r.URL.Path, "query": r.URL.RawQuery})
		requestsMu.Unlock()
		f.handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	fault := &inspectionACKLoss194{}
	httpClient := &http.Client{Transport: fault, Timeout: 15 * time.Second}
	browser := func(method, route string, body any) (map[string]any, error) {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		request, err := http.NewRequest(method, server.URL+"/api/v1"+route, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		request.AddCookie(f.cookie)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Suma-Expected-User", f.user.ID)
		request.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		response, err := httpClient.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		var value map[string]any
		if err = json.NewDecoder(response.Body).Decode(&value); err != nil {
			return nil, err
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, fmt.Errorf("%s %s rejected: status %d", method, route, response.StatusCode)
		}
		return value, nil
	}
	must := func(method, route string, body any) map[string]any {
		t.Helper()
		value, err := browser(method, route, body)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	workerID := "owned-inspection-worker194"
	identity := must("POST", "/release-workers", map[string]any{"project_id": f.project, "worker_id": workerID, "profiles": []any{}, "deployment": false, "idempotency_key": newID()})
	token, ok := identity["token"].(string)
	if !ok || token == "" {
		t.Fatal("original worker credential absent")
	}
	journal, err := runner.OpenJournal(filepath.Join(dir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	worker := &runner.Worker{ID: workerID, Client: &runner.Client{BaseURL: server.URL, ProjectID: f.project, Token: token, HTTP: httpClient}, Journal: journal, PollInterval: 100 * time.Millisecond, Exec: runner.ExecConfig{AllowExec: true, Workspace: repository, Repository: repository, BaseCommit: base, WorktreeRoot: filepath.Join(repository, ".cache", "runner", "worktrees"), Secrets: []string{token}, Commands: map[string]runner.CommandSpec{"owned-harness194": {Adapter: "harness", Protocol: "dsh-multica-jsonl-v1", Executable: node, Args: []string{entry, "--profile", "multica", "--stdio"}, Input: "dsh-jsonl", Env: map[string]string{"SUMA_INSPECTION_COUNTER": counter}}}}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	var stop sync.Once
	stopWorker := func() {
		stop.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil && !errors.Is(err, context.Canceled) {
					t.Errorf("original worker stopped unexpectedly: %v", err)
				}
			case <-time.After(8 * time.Second):
				t.Error("original worker did not stop")
			}
		})
	}
	defer stopWorker()
	wait := func(description string, ready func() bool) {
		t.Helper()
		deadline := time.Now().Add(22 * time.Second)
		for time.Now().Before(deadline) {
			if ready() {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("timed out waiting for " + description)
	}
	var observations []tc.Observation
	wait("original fresh inventory", func() bool {
		value, err := browser("GET", "/tool-discovery?project_id="+f.project+"&worker_id="+workerID, nil)
		if err != nil || value["state"] != "fresh" {
			return false
		}
		raw, _ := json.Marshal(value["items"])
		return json.Unmarshal(raw, &observations) == nil && len(observations) == 1 && observations[0].Available
	})
	created := must("POST", "/tool-profiles", ToolProfileCreateRequest{ProjectID: f.project, WorkerID: workerID, Name: "Owned Harness inspection194", Profile: observations[0].Profile, IdempotencyKey: newID()})
	profileID, ok := created["id"].(string)
	if !ok || created["state"] != "paused" {
		t.Fatal("original paused profile absent")
	}
	createBody := ic.ToolInspectionCreateRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()}
	createPath := "/tool-profiles/" + profileID + "/inspections"
	if _, err = browser("POST", createPath, createBody); err == nil {
		t.Fatal("the committed creation acknowledgement was not lost")
	}
	original := must("POST", createPath, createBody)
	raw, _ := json.Marshal(original)
	var inspection ic.ToolInspection
	if json.Unmarshal(raw, &inspection) != nil || inspection.State != "pending" || inspection.Version != 1 {
		t.Fatal("same creation operation did not recover its original snapshot")
	}
	wait("associated inventory completion and original journal ACK", func() bool {
		value, err := browser("GET", "/tool-inspections/"+inspection.ID+"?project_id="+f.project, nil)
		if err != nil {
			return false
		}
		raw, _ := json.Marshal(value)
		return json.Unmarshal(raw, &inspection) == nil && inspection.State == "completed" && journal.PendingInventory() == nil
	})
	// Recovering an original ACK does not renew its server timestamp. The next
	// independent periodic snapshot restores current readiness, without ever
	// re-executing the requested inspection or relying on the local wall clock.
	wait("independent current snapshot after ACK recovery", func() bool {
		value, err := browser("GET", "/tool-discovery?project_id="+f.project+"&worker_id="+workerID, nil)
		if err != nil || value["state"] != "fresh" || value["observed_at"] == inspection.ObservedAt || journal.PendingInventory() != nil {
			return false
		}
		fault.mu.Lock()
		defer fault.mu.Unlock()
		ordinary := map[string]bool{}
		for _, report := range fault.inventories {
			if report["inspection_id"] == "" {
				ordinary[report["operation_id"]] = true
			}
		}
		return len(ordinary) == 2
	})
	stopWorker()
	countRaw, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, value := range strings.Fields(string(countRaw)) {
		counts[value]++
	}
	if counts["version"] != 3 || counts["probe"] != 3 || counts["unexpected"] != 0 {
		t.Fatalf("new inspection or lost-ACK recovery reprobed unexpectedly: %v", counts)
	}
	fault.mu.Lock()
	createLost, inventoryLost, reportedID, operationID := fault.createLost, fault.inventoryLost, fault.inspectionID, fault.inventoryOperation
	reports := append([]map[string]string(nil), fault.inventories...)
	fault.mu.Unlock()
	if !createLost || !inventoryLost || reportedID != inspection.ID || operationID != inspection.InventoryOperationID || inspection.Fence < 1 {
		t.Fatal("inspection did not retain its original report and claim")
	}
	currentProfile := must("GET", "/tool-profiles/"+profileID+"?project_id="+f.project, nil)
	if currentProfile["state"] != "paused" || git("rev-parse", "HEAD") != base || git("status", "--porcelain") != "" {
		t.Fatal("inspection changed enablement or the owned code baseline")
	}
	var executions int
	if err = f.s.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM dag_runs WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&executions); err != nil || executions != 0 {
		t.Fatal("inspection created a business execution", err)
	}
	if _, err = f.s.DB.ExecContext(context.Background(), `VACUUM INTO ?`, filepath.Join(dir, "private.sqlite")); err != nil {
		t.Fatal(err)
	}
	requestsMu.Lock()
	recorded := append([]map[string]string(nil), requests...)
	requestsMu.Unlock()
	entrySHA := sha256.Sum256([]byte(inspectionProbe194))
	receipt := map[string]any{"stage": "T-TOOL-REINSPECTION-194", "status": "actual_http_sqlite_worker_journal_probe_and_ack_recovery_confirmed", "boundary": "Real Go Worker, HTTP, SQLite, Journal, Git and explicitly owned Node Harness protocol probe; no vendor CLI/account, model, installation, business task or acceptance.", "inspection": inspection, "create_ack_lost": createLost, "inventory_ack_lost": inventoryLost, "probe_counts": counts, "new_requested_probes": 1, "journal_inventory_pending": journal.PendingInventory() != nil, "profile_state": currentProfile["state"], "base_commit": base, "head_commit": git("rev-parse", "HEAD"), "fixture_source_sha256": hex.EncodeToString(entrySHA[:]), "http_requests": recorded, "business_runs": executions, "vendor_calls": 0, "model_calls": 0, "customer_acceptance": false}
	receipt["post_recovery_periodic_probes"] = 1
	receipt["inventory_reports"] = reports
	b, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil || bytes.Contains(b, []byte(token)) || bytes.Contains(b, []byte(f.cookie.Value)) {
		t.Fatal("receipt could not be safely serialized")
	}
	if err = os.WriteFile(filepath.Join(dir, "receipt.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
}
