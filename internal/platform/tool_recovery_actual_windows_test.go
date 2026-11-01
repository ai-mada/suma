//go:build windows

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
	"io/fs"
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
	ti "github.com/ai-mada/suma/internal/toolinspectioncontract"
	"golang.org/x/sys/windows"
)

// Owned stdlib-only transport fixture, not a vendor Harness/model executable.
// The second --probe deliberately keeps a real native parent and child alive.
const toolRecoveryProbe195 = `import fs from 'node:fs';
import {spawn} from 'node:child_process';
const args=process.argv.slice(2),counter=process.env.SUMA_TOOL_RECOVERY_COUNTER,marker=process.env.SUMA_TOOL_RECOVERY_MARKER;
if(!counter||!marker)throw Error('missing owned fixture evidence');
if(args.length===1&&args[0]==='--version'){
 fs.appendFileSync(counter,'version\n');console.log('owned-recovery-fixture-195');
}else if(args.join(' ')==='--profile multica --probe'){
 fs.appendFileSync(counter,'probe\n');
 const count=fs.readFileSync(counter,'utf8').split('\n').filter(x=>x==='probe').length;
 if(count===2){
  const child=spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{stdio:'ignore',windowsHide:true});
  if(!child.pid)throw Error('owned child did not start');
  fs.writeFileSync(marker,JSON.stringify({parent:process.pid,child:child.pid}));
  setInterval(()=>{},1000);
 }else console.log(JSON.stringify({v:1,type:'probe',runtime:'dsh',plugin_version:'fixture-195',protocol_version:1}));
}else {fs.appendFileSync(counter,'unexpected\n');process.exitCode=2;}
`

type recoveryActualACK195 struct {
	Operation       string `json:"operation"`
	BodySHA256      string `json:"body_sha256"`
	ProofSHA256     string `json:"proof_sha256"`
	ExpectedVersion int    `json:"expected_version"`
	ProbeCount      int    `json:"probe_count"`
	Status          int    `json:"status"`
}
type recoveryActualCheckpoint195 struct {
	Started      bool           `json:"started"`
	Complete     bool           `json:"complete"`
	FailureAcked bool           `json:"failure_acked"`
	Inspection   *ti.Inspection `json:"inspection"`
	Recovery     *struct {
		Accepted     *ti.ToolInspectionRecovery `json:"accepted"`
		Closed       *ti.ToolInspectionRecovery `json:"closed"`
		AckOperation string                     `json:"ack_operation"`
	} `json:"recovery"`
}

func recoveryActualCounts195(path string) map[string]int {
	raw, _ := os.ReadFile(path)
	counts := map[string]int{}
	for _, word := range strings.Fields(string(raw)) {
		counts[word]++
	}
	return counts
}
func recoveryActualCheckpointRead195(path string) (out recoveryActualCheckpoint195, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	if len(raw) > 64<<10 {
		return out, errors.New("owned checkpoint exceeds its read bound")
	}
	err = json.Unmarshal(raw, &out)
	return
}

// This opt-in acceptance source exercises the real HTTP handler, SQLite,
// Windows job containment, persistent Journal and three original Worker.Run
// lifetimes. It must never be counted as ordinary unit coverage or vendor use.
func TestToolRecoveryActualWindowsWorkerRestartsAndLostClosureAcknowledgement(t *testing.T) {
	if os.Getenv("SUMA_TOOL_RECOVERY_195_ACTUAL") != "1" {
		t.Skip("explicit isolated Windows native recovery acceptance required")
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
	if suffix := os.Getenv("SUMA_TOOL_RECOVERY_195_EVIDENCE_SUFFIX"); suffix != "" {
		if suffix != "final" {
			t.Fatal("only a separately retained final evidence directory is allowed")
		}
		evidenceName += "-" + suffix
	}
	dir := filepath.Join(root, ".cache", "tool-recovery-195", evidenceName)
	if _, err = os.Stat(filepath.Join(dir, "receipt.json")); !os.IsNotExist(err) {
		t.Fatal("retain original actual receipt; do not reseed")
	}
	if err = os.MkdirAll(filepath.Join(dir, "tmp"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TEMP", "TMP", "TMPDIR"} {
		t.Setenv(name, filepath.Join(dir, "tmp"))
	}
	node := os.Getenv("SUMA_TOOL_RECOVERY_195_NODE")
	if !filepath.IsAbs(node) {
		t.Fatal("explicit existing absolute Node executable required")
	}
	if st, e := os.Stat(node); e != nil || !st.Mode().IsRegular() {
		t.Fatal("existing Node executable unavailable")
	}
	repository := filepath.Join(dir, "repository")
	if err = os.Mkdir(repository, 0700); err != nil {
		t.Fatal("retain prior owned source/evidence instead of reseeding", err)
	}
	git := func(args ...string) string {
		t.Helper()
		argv := append([]string{"-c", "core.hooksPath=" + filepath.Join(dir, "disabled-hooks"), "-c", "commit.gpgsign=false", "-c", "user.name=Owned Recovery Fixture", "-c", "user.email=recovery@example.invalid"}, args...)
		cmd := exec.Command("git", argv...)
		cmd.Dir = repository
		out, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("owned Git fixture failed: %v", e)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-b", "main")
	if os.WriteFile(filepath.Join(repository, ".gitignore"), []byte(".cache/\n"), 0600) != nil || os.WriteFile(filepath.Join(repository, "README.md"), []byte("Owned read-only native recovery acceptance fixture.\n"), 0600) != nil {
		t.Fatal("owned fixture source unavailable")
	}
	git("add", "--", ".gitignore", "README.md")
	git("commit", "-m", "owned recovery fixture")
	base := git("rev-parse", "HEAD")
	entry, counter, marker := filepath.Join(dir, "probe.mjs"), filepath.Join(dir, "probe-count.txt"), filepath.Join(dir, "native-marker.json")
	if err = os.WriteFile(entry, []byte(toolRecoveryProbe195), 0600); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t)
	var faultMu sync.Mutex
	var cancelSecond context.CancelFunc
	lost := false
	ackLost := make(chan struct{})
	ackAttempts := []recoveryActualACK195{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runner/tool-inspections/recovery/acknowledgements" {
			f.handler.ServeHTTP(w, r)
			return
		}
		raw, e := io.ReadAll(io.LimitReader(r.Body, 64<<10))
		if e != nil {
			http.Error(w, "owned bounded request unavailable", 500)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var body ti.ToolInspectionRecoveryAcknowledgementRequest
		if json.Unmarshal(raw, &body) != nil {
			http.Error(w, "owned acknowledgement unreadable", 500)
			return
		}
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, r)
		sum := sha256.Sum256(raw)
		attempt := recoveryActualACK195{Operation: body.IdempotencyKey, BodySHA256: hex.EncodeToString(sum[:]), ProofSHA256: body.ProofSHA256, ExpectedVersion: body.ExpectedVersion, ProbeCount: recoveryActualCounts195(counter)["probe"], Status: response.Code}
		faultMu.Lock()
		ackAttempts = append(ackAttempts, attempt)
		drop := response.Code == 200 && !lost
		if drop {
			lost = true
		}
		cancel := cancelSecond
		faultMu.Unlock()
		if drop {
			// Commit and actual 200 response happened above. Close the real connection
			// before returning its ACK, and stop W2 before it can retry in-process.
			if cancel != nil {
				cancel()
			}
			close(ackLost)
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("owned real server cannot drop committed ACK")
				return
			}
			connection, _, e := hijacker.Hijack()
			if e != nil {
				t.Error("owned committed-ACK connection unavailable", e)
				return
			}
			_ = connection.Close()
			return
		}
		for key, values := range response.Header() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(response.Code)
		_, _ = w.Write(response.Body.Bytes())
	}))
	defer server.Close()
	httpClient := &http.Client{Timeout: 15 * time.Second}
	browser := func(method, route string, body any) (map[string]any, error) {
		raw, e := json.Marshal(body)
		if e != nil {
			return nil, e
		}
		request, e := http.NewRequest(method, server.URL+"/api/v1"+route, bytes.NewReader(raw))
		if e != nil {
			return nil, e
		}
		request.AddCookie(f.cookie)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Suma-Expected-User", f.user.ID)
		request.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		response, e := httpClient.Do(request)
		if e != nil {
			return nil, e
		}
		defer response.Body.Close()
		var out map[string]any
		if e = json.NewDecoder(response.Body).Decode(&out); e != nil {
			return nil, e
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, fmt.Errorf("%s %s status %d", method, route, response.StatusCode)
		}
		return out, nil
	}
	must := func(method, route string, body any) map[string]any {
		t.Helper()
		out, e := browser(method, route, body)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	waitFor := func(label string, ready func() bool) {
		t.Helper()
		deadline := time.Now().Add(24 * time.Second)
		for time.Now().Before(deadline) {
			if ready() {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("timed out waiting for " + label)
	}
	workerID := "owned-recovery-worker195"
	identity := must("POST", "/release-workers", map[string]any{"project_id": f.project, "worker_id": workerID, "profiles": []any{}, "deployment": false, "idempotency_key": newID()})
	token, ok := identity["token"].(string)
	if !ok || token == "" {
		t.Fatal("owned original worker credential absent")
	}
	journalDir := filepath.Join(dir, "journal")
	checkpointPath := filepath.Join(journalDir, "tool-inspection.json")
	type liveWorker struct {
		cancel  context.CancelFunc
		done    chan error
		journal *runner.Journal
		once    sync.Once
	}
	startWorker := func() *liveWorker {
		t.Helper()
		j, e := runner.OpenJournal(journalDir)
		if e != nil {
			t.Fatal(e)
		}
		ctx, cancel := context.WithCancel(context.Background())
		live := &liveWorker{cancel: cancel, done: make(chan error, 1), journal: j}
		worker := &runner.Worker{ID: workerID, Client: &runner.Client{BaseURL: server.URL, ProjectID: f.project, Token: token, HTTP: httpClient}, Journal: j, PollInterval: 100 * time.Millisecond, Exec: runner.ExecConfig{AllowExec: true, Workspace: repository, Repository: repository, BaseCommit: base, WorktreeRoot: filepath.Join(repository, ".cache", "runner", "worktrees"), Secrets: []string{token}, Commands: map[string]runner.CommandSpec{"owned-harness195": {Adapter: "harness", Protocol: "dsh-multica-jsonl-v1", Executable: node, Args: []string{entry, "--profile", "multica", "--stdio"}, Input: "dsh-jsonl", Env: map[string]string{"SUMA_TOOL_RECOVERY_COUNTER": counter, "SUMA_TOOL_RECOVERY_MARKER": marker}}}}}
		go func() { live.done <- worker.Run(ctx) }()
		return live
	}
	stopWorker := func(live *liveWorker) {
		t.Helper()
		live.once.Do(func() {
			live.cancel()
			select {
			case e := <-live.done:
				if e != nil && !errors.Is(e, context.Canceled) {
					t.Errorf("owned worker stopped unexpectedly: %v", e)
				}
			case <-time.After(9 * time.Second):
				t.Error("owned worker did not stop")
			}
			if e := live.journal.Close(); e != nil {
				t.Error(e)
			}
		})
	}
	first := startWorker()
	defer stopWorker(first)
	var observations []tc.Observation
	waitFor("initial actual configured inventory", func() bool {
		out, e := browser("GET", "/tool-discovery?project_id="+f.project+"&worker_id="+workerID, nil)
		if e != nil || out["state"] != "fresh" {
			return false
		}
		raw, _ := json.Marshal(out["items"])
		return json.Unmarshal(raw, &observations) == nil && len(observations) == 1 && observations[0].Available
	})
	profile := must("POST", "/tool-profiles", ToolProfileCreateRequest{ProjectID: f.project, WorkerID: workerID, Name: "Owned native recovery195", Profile: observations[0].Profile, IdempotencyKey: newID()})
	profileID, ok := profile["id"].(string)
	if !ok || profile["state"] != "paused" {
		t.Fatal("owned paused profile unavailable")
	}
	var ownerID string
	waitFor("authenticated continuous maintenance declaration", func() bool {
		out, e := browser("GET", "/tool-profiles/"+profileID+"?project_id="+f.project, nil)
		if e != nil {
			return false
		}
		maintenance, ok := out["maintenance"].(map[string]any)
		if !ok || maintenance["state"] != "supported" {
			return false
		}
		ownerID, _ = maintenance["instance_id"].(string)
		return ownerID != ""
	})
	createPath := "/tool-profiles/" + profileID + "/inspections"
	originalMap := must("POST", createPath, ti.CreateRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()})
	var original ti.Inspection
	raw, _ := json.Marshal(originalMap)
	if json.Unmarshal(raw, &original) != nil || original.State != "pending" {
		t.Fatal("owned original maintenance request unavailable")
	}
	var nativeMarker struct {
		Parent int `json:"parent"`
		Child  int `json:"child"`
	}
	waitFor("real held native parent and child", func() bool {
		raw, e := os.ReadFile(marker)
		return e == nil && json.Unmarshal(raw, &nativeMarker) == nil && nativeMarker.Parent > 0 && nativeMarker.Child > 0
	})
	parentHandle, e := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(nativeMarker.Parent))
	if e != nil {
		t.Fatal("parent handle must be opened before cancellation", e)
	}
	defer windows.CloseHandle(parentHandle)
	childHandle, e := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(nativeMarker.Child))
	if e != nil {
		t.Fatal("child handle must be opened before cancellation", e)
	}
	defer windows.CloseHandle(childHandle)
	for _, handle := range []windows.Handle{parentHandle, childHandle} {
		state, e := windows.WaitForSingleObject(handle, 0)
		if e != nil || state != uint32(windows.WAIT_TIMEOUT) {
			t.Fatal("native handle was not alive before cancellation", state, e)
		}
	}
	first.cancel()
	parentWait, e := windows.WaitForSingleObject(parentHandle, 6000)
	if e != nil || parentWait != windows.WAIT_OBJECT_0 {
		t.Fatal("native parent did not exit through owned job cancellation", parentWait, e)
	}
	childWait, e := windows.WaitForSingleObject(childHandle, 6000)
	if e != nil || childWait != windows.WAIT_OBJECT_0 {
		t.Fatal("native child did not exit through owned job cancellation", childWait, e)
	}
	stopWorker(first)
	if counts := recoveryActualCounts195(counter); counts["probe"] != 2 || counts["version"] != 2 || counts["unexpected"] != 0 {
		t.Fatal("initial cancellation performed unexpected native operations", counts)
	}
	second := startWorker()
	defer stopWorker(second)
	faultMu.Lock()
	cancelSecond = second.cancel
	faultMu.Unlock()
	waitFor("original unknown result and durable failure acknowledgement without re-probe", func() bool {
		out, e := browser("GET", "/tool-inspections/"+original.ID+"?project_id="+f.project, nil)
		if e != nil {
			return false
		}
		raw, _ := json.Marshal(out)
		if json.Unmarshal(raw, &original) != nil || original.State != "unknown" {
			return false
		}
		checkpoint, e := recoveryActualCheckpointRead195(checkpointPath)
		return e == nil && checkpoint.Started && !checkpoint.Complete && checkpoint.FailureAcked && second.journal.PendingInventory() == nil && recoveryActualCounts195(counter)["probe"] == 2
	})
	originalCode := original.ResultCode
	ref := ti.Reference{InspectionID: original.ID, ClaimID: original.ClaimID, Fence: original.Fence}
	recoveryMap := must("POST", "/tool-inspections/"+original.ID+"/recoveries", ti.ToolInspectionRecoveryCreateRequest{ProjectID: f.project, ExpectedVersion: original.Version, Inspection: ref, IdempotencyKey: newID()})
	var recovery ti.ToolInspectionRecovery
	raw, _ = json.Marshal(recoveryMap)
	if json.Unmarshal(raw, &recovery) != nil || recovery.State != "requested" {
		t.Fatal("explicit current recovery authorization unavailable")
	}
	select {
	case <-ackLost:
	case <-time.After(24 * time.Second):
		t.Fatal("actual native proof was not accepted and closure ACK was not deliberately lost")
	}
	stopWorker(second)
	pending, e := recoveryActualCheckpointRead195(checkpointPath)
	if e != nil || !pending.Started || pending.Complete || pending.Recovery == nil || pending.Recovery.Accepted == nil || pending.Recovery.AckOperation == "" || pending.Recovery.Closed != nil {
		t.Fatal("node did not preserve exact accepted proof and original unknown ACK intent", e)
	}
	if counts := recoveryActualCounts195(counter); counts["probe"] != 2 || counts["version"] != 2 {
		t.Fatal("reconciliation re-executed a native probe", counts)
	}
	third := startWorker()
	defer stopWorker(third)
	var closed ti.ToolInspectionRecovery
	waitFor("same closure acknowledgement recovered across the third Worker lifetime", func() bool {
		out, e := browser("GET", "/tool-inspection-recoveries/"+recovery.ID+"?project_id="+f.project, nil)
		if e != nil {
			return false
		}
		raw, _ := json.Marshal(out)
		if json.Unmarshal(raw, &closed) != nil || closed.State != "closed_unknown" {
			return false
		}
		faultMu.Lock()
		defer faultMu.Unlock()
		return len(ackAttempts) >= 2
	})
	waitFor("independent periodic inventory after confirmed original closure", func() bool {
		out, e := browser("GET", "/tool-profiles/"+profileID+"?project_id="+f.project, nil)
		if e != nil {
			return false
		}
		maintenance, ok := out["maintenance"].(map[string]any)
		counts := recoveryActualCounts195(counter)
		return ok && maintenance["state"] == "supported" && maintenance["instance_id"] == ownerID && counts["probe"] == 3 && counts["version"] == 3 && third.journal.PendingInventory() == nil
	})
	nextMap := must("POST", createPath, ti.CreateRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()})
	var next ti.Inspection
	raw, _ = json.Marshal(nextMap)
	if json.Unmarshal(raw, &next) != nil || next.ID == original.ID {
		t.Fatal("new request reused unknown execution")
	}
	waitFor("new explicitly requested fourth probe completed", func() bool {
		out, e := browser("GET", "/tool-inspections/"+next.ID+"?project_id="+f.project, nil)
		if e != nil {
			return false
		}
		raw, _ := json.Marshal(out)
		return json.Unmarshal(raw, &next) == nil && next.State == "completed" && third.journal.PendingInventory() == nil
	})
	stopWorker(third)
	faultMu.Lock()
	attempts := append([]recoveryActualACK195(nil), ackAttempts...)
	ackDropped := lost
	faultMu.Unlock()
	if len(attempts) != 2 || !ackDropped || attempts[0].Status != 200 || attempts[1].Status != 200 || attempts[0].Operation != pending.Recovery.AckOperation || attempts[0].Operation != attempts[1].Operation || attempts[0].BodySHA256 != attempts[1].BodySHA256 || attempts[0].ProofSHA256 != attempts[1].ProofSHA256 || attempts[0].ProbeCount != 2 || attempts[1].ProbeCount != 2 {
		t.Fatal("lost original ACK was not recovered exactly without re-probe", attempts)
	}
	if closed.Proof == nil || closed.Proof.NativeProcessCount < 2 || closed.Proof.StoppedProcessCount != closed.Proof.NativeProcessCount || !closed.Proof.ProbeReturned || !closed.Proof.AllNativeTreesStopped || !closed.Proof.NoPendingInventory {
		t.Fatal("actual receipt did not cover all owned native trees, including inventory Git probes")
	}
	archived := false
	matchedNativeReceipt := false
	err = filepath.WalkDir(filepath.Join(journalDir, "tool-inspection-receipts"), func(path string, entry fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("owned evidence contains unexpected symbolic link")
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Name() == "returned.json" {
			raw, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			sum := sha256.Sum256(raw)
			if hex.EncodeToString(sum[:]) != closed.Proof.ReceiptSHA256 {
				return nil
			}
			var receipt struct {
				Source struct {
					Inspection   ti.Reference `json:"inspection"`
					SourceSHA256 string       `json:"source_sha256"`
				} `json:"source"`
				NativeProcessCount  int               `json:"native_process_count"`
				StoppedProcessCount int               `json:"stopped_process_count"`
				Processes           []json.RawMessage `json:"processes"`
			}
			if json.Unmarshal(raw, &receipt) != nil || receipt.Source.Inspection != closed.Inspection || receipt.Source.SourceSHA256 != closed.SourceSHA256 || receipt.NativeProcessCount != closed.Proof.NativeProcessCount || receipt.StoppedProcessCount != receipt.NativeProcessCount || len(receipt.Processes) != receipt.NativeProcessCount {
				return errors.New("original receipt coverage differs from its authenticated proof")
			}
			matchedNativeReceipt = true
			return nil
		}
		if entry.Name() != "closed-checkpoint.json" {
			return nil
		}
		checkpoint, e := recoveryActualCheckpointRead195(path)
		if e != nil {
			return e
		}
		if checkpoint.Inspection != nil && checkpoint.Inspection.ID == original.ID && checkpoint.Started && checkpoint.Complete && checkpoint.Recovery != nil && checkpoint.Recovery.Closed != nil && checkpoint.Recovery.Closed.ProofSHA256 == closed.ProofSHA256 {
			archived = true
		}
		return nil
	})
	if err != nil || !archived || !matchedNativeReceipt {
		t.Fatal("original Started/unknown checkpoint was not retained in immutable closure history", err)
	}
	finalMap := must("GET", "/tool-inspections/"+original.ID+"?project_id="+f.project, nil)
	var final ti.Inspection
	raw, _ = json.Marshal(finalMap)
	if json.Unmarshal(raw, &final) != nil || final.State != "closed_unknown" || final.ResultCode != originalCode || final.RecoveryID != closed.ID {
		t.Fatal("closure changed original unknown truth")
	}
	counts := recoveryActualCounts195(counter)
	if counts["probe"] != 4 || counts["version"] != 4 || counts["unexpected"] != 0 {
		t.Fatal("native execution count differs from original plus two explicit requests and one independent periodic report", counts)
	}
	currentProfile := must("GET", "/tool-profiles/"+profileID+"?project_id="+f.project, nil)
	head := git("rev-parse", "HEAD")
	if currentProfile["state"] != "paused" || head != base || git("status", "--porcelain") != "" {
		t.Fatal("maintenance enabled a tool or changed the owned code baseline")
	}
	var runs int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&runs); err != nil || runs != 0 {
		t.Fatal("maintenance created a business run", err)
	}
	if _, err = f.s.DB.Exec(`VACUUM INTO ?`, filepath.Join(dir, "private.sqlite")); err != nil {
		t.Fatal(err)
	}
	sourceSHA := sha256.Sum256([]byte(toolRecoveryProbe195))
	receipt := map[string]any{"stage": "T-TOOL-RECOVERY-195", "status": "actual_windows_http_sqlite_native_stop_and_three_worker_journal_lifetimes_confirmed", "boundary": "Owned stdlib Node Harness transport, real Windows parent/child handles, original Go Worker/HTTP/SQLite/Journal; no vendor, model, installation, task or customer acceptance.", "worker_lifetimes": 3, "original_inspection": final, "recovery": closed, "new_explicit_inspection": next, "logical_owner_preserved": ownerID, "native_parent_handle_open_before_cancel": true, "native_child_handle_open_before_cancel": true, "native_parent_wait_result": parentWait, "native_child_wait_result": childWait, "proof_counts_are_owned_root_trees": true, "closure_ack_lost_after_commit": ackDropped, "closure_ack_attempts": attempts, "probe_counts": counts, "unknown_started_checkpoint_archived": archived, "prepared_inventory_after_interruption": false, "profile_state": currentProfile["state"], "base_commit": base, "head_commit": head, "fixture_source_sha256": hex.EncodeToString(sourceSHA[:]), "business_runs": runs, "vendor_calls": 0, "model_calls": 0, "customer_acceptance": false}
	encoded, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil || bytes.Contains(encoded, []byte(token)) || bytes.Contains(encoded, []byte(f.cookie.Value)) {
		t.Fatal("public actual receipt could not be safely serialized")
	}
	if err = os.WriteFile(filepath.Join(dir, "receipt.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
}
