package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	wc "github.com/ai-mada/suma/internal/workspaceoperationcontract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// One real official MCP session covers every ordinary workspace tool, an
// actual owned check process, safe rejection, and reuse of the original effect.
// No model, vendor account or second dispatcher participates.
func TestWorkspaceOperationActualMCPFactsRejectionsAndEffectReplay(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node is required for the actual host check fixture", err)
	}
	checks := 0
	n, w, _ := nativeFixture(t, []string{"fixed-check"}, func(ctx context.Context, id string) (NativeWorkspaceCheckResult, error) {
		checks++
		cmd := exec.CommandContext(ctx, node, "-e", "console.log('owned-private-check-output');process.exitCode=1")
		cmd.Env, _, _ = commandEnvironment(CommandSpec{})
		raw, runErr := cmd.CombinedOutput()
		exit := 0
		if runErr != nil {
			var failed *exec.ExitError
			if !errors.As(runErr, &failed) {
				return NativeWorkspaceCheckResult{}, runErr
			}
			exit = failed.ExitCode()
		}
		return NativeWorkspaceCheckResult{ExitCode: exit, ProfileSHA256: strings.Repeat("b", 64), Output: string(raw)}, nil
	})
	events := []wc.Event{}
	n.traceProgress = func(event wc.Event) error { events = append(events, event); return nil }
	bridge, err := n.StartMCP()
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "workspace-facts-owned", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: bridge.Endpoint, HTTPClient: &http.Client{Transport: nativeTestBearer{bridge.Token}, Timeout: 5 * time.Second}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	call := func(action, id string, args any, failed bool) {
		t.Helper()
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Meta: mcp.Meta{"suma/operation_id": id}, Name: "suma_workspace_" + action, Arguments: args})
		if err != nil || result == nil || result.IsError != failed {
			t.Fatalf("actual MCP %s failed: %+v %v", action, result, err)
		}
	}
	call("list", "list-one", map[string]any{}, false)
	call("read", "read-one", map[string]any{"path": "file.txt"}, false)
	write := map[string]any{"path": "file.txt", "content": "private-source-content\n", "expected_sha256": digest([]byte("base\n"))}
	call("write", "original-effect", write, false)
	call("write", "original-effect", write, false)
	call("write", "cas-rejection", write, true)
	call("read", "unsafe-path", map[string]any{"path": "../private-secret.txt"}, true)
	call("edit", "edit-effect", map[string]any{"path": "file.txt", "expected_sha256": digest([]byte("private-source-content\n")), "edits": []any{map[string]any{"old_text": "private-source-content", "new_text": "edited"}}}, false)
	call("check", "check-effect", map[string]any{"check_id": "fixed-check"}, false)
	call("check", "check-effect", map[string]any{"check_id": "fixed-check"}, false)
	call("remove", "remove-effect", map[string]any{"path": "file.txt", "expected_sha256": digest([]byte("edited\n"))}, false)
	if checks != 1 || n.ledger.Generation != 3 || len(events) != 20 {
		t.Fatalf("effects repeated or calls missing: checks=%d generation=%d events=%d", checks, n.ledger.Generation, len(events))
	}
	for index, event := range events {
		if wc.ValidateBoundEvent(workspaceOperationIdentity(n.assignment), event) != nil || event.Sequence != index+1 {
			t.Fatal("host event is not bound to the original invocation", event)
		}
	}
	if events[7].Phase != "completed" || !events[7].Replayed || events[7].EffectOperationID != "original-effect" || events[9].Phase != "failed" || events[11].Path != "" || events[15].Phase != "failed" || events[15].ExitCode == nil || *events[15].ExitCode != 1 || !events[17].Replayed || events[17].Phase != "failed" {
		t.Fatal("actual outcome/replay/rejection was misclassified", events)
	}
	raw, _ := json.Marshal(events)
	for _, secret := range []string{"private-source-content", "private-secret.txt", "owned-private-check-output", w.Source, bridge.Token, bridge.Endpoint} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("host progress exposed private data", secret)
		}
	}
	if _, err = os.Stat(filepath.Join(w.Source, "file.txt")); !os.IsNotExist(err) {
		t.Fatal("actual remove effect was not applied", err)
	}
	coverage := n.Evidence().OperationTrace
	if coverage == nil || coverage.InvocationCount != 10 || coverage.RecordCount != 20 || coverage.PendingCount != 0 || coverage.State != "complete" {
		t.Fatal("capture coverage differs from actual calls", coverage)
	}
}

func TestWorkspaceOperationActualParentSealRetainsOriginalCandidate(t *testing.T) {
	w, _ := nativeSealFixture(t)
	a := w.Assignment
	spec := CommandSpec{NativeWorkspace: &NativeWorkspacePolicy{Checks: []string{}}}
	events := []wc.Event{}
	n, err := openNativeWorkspace(context.Background(), a, ExecConfig{WorkspaceProgress: func(e wc.Event) error { events = append(events, e); return nil }}, w, spec, strings.Repeat("a", 64), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = nativeCall(t, n, "write", "seal-file-effect", map[string]any{"path": "file.txt", "content": "sealed real source\n", "expected_sha256": digest([]byte("base\n"))}); err != nil {
		t.Fatal(err)
	}
	if err = n.Close(); err != nil {
		t.Fatal(err)
	}
	sealed, err := n.seal(context.Background(), n.Evidence())
	if err != nil || sealed.State != "sealed" || len(events) != 4 || events[3].Action != "seal" || events[3].Phase != "completed" || events[3].EffectOperationID != sealed.OperationID {
		t.Fatal("actual parent seal lacks its host trace", sealed, events, err)
	}
	if head := strings.TrimSpace(gitTest(t, w.Repository, "rev-parse", "HEAD")); head != w.BaseCommit {
		t.Fatal("parent seal changed the administrator checkout")
	}
	if head := strings.TrimSpace(gitTest(t, w.Source, "rev-parse", "HEAD")); head != sealed.Commit {
		t.Fatal("seal trace does not describe the actual candidate")
	}
}

// A full SDK log cannot consume host admission. The completion is deliberately
// lost only from the Journal; restart imports the already durable private fact.
// A second started-only check becomes unknown without executing any callback.
func TestWorkspaceOperationJournalRecoversLostFactsWithoutRepeatingChecks(t *testing.T) {
	a := Assignment{TenantID: "tenant", ProjectID: "project", RunID: "run", NodeID: "node", Attempt: 1, Fence: 2, WorkerID: "worker", Type: "agent", Command: []string{"native"}, Config: map[string]any{"tool_profile_sha256": strings.Repeat("a", 64)}}
	n, w, _ := nativeFixtureAssignment(t, a, []string{"never-run"}, nil)
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if j != nil {
			_ = j.Close()
		}
	}()
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	if err = j.attachWorkspaceTrace(a, w.Directory, n.policySHA, n.traceProfileSHA); err != nil {
		t.Fatal(err)
	}
	e := j.entries[key(a)]
	for index := 0; index < 1024; index++ {
		e.Progress = append(e.Progress, Progress{Assignment: a, OperationID: fmt.Sprintf("%064x", index+1), Message: "owned SDK log fixture", Acked: true})
	}
	j.entries[key(a)] = e
	if err = j.save(); err != nil {
		t.Fatal(err)
	}
	n.traceJournal = j
	n.traceProgress = func(event wc.Event) error {
		if event.Revision == 2 {
			return errors.New("owned lost completion")
		}
		return j.RecordWorkspaceProgress(a, event)
	}
	if _, err = nativeCall(t, n, "write", "durable-write", map[string]any{"path": "file.txt", "content": "one durable write\n", "expected_sha256": digest([]byte("base\n"))}); err == nil {
		t.Fatal("lost parent outbox acknowledgement was hidden")
	}
	if n.ledger.Operations["durable-write"].State != "completed" || n.ledger.Trace.Calls[0].Terminal == nil {
		t.Fatal("completed original effect was overwritten")
	}
	n.traceProgress = func(event wc.Event) error { return j.RecordWorkspaceProgress(a, event) }
	n.mu.Lock()
	args := map[string]any{"check_id": "never-run"}
	raw, _ := json.Marshal(args)
	_, err = n.beginTraceLocked("check", raw, args, "unexecuted-check")
	n.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err = n.Close(); err != nil {
		t.Fatal(err)
	}
	if err = j.RecordProgress(a, "extra SDK log"); err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal("restart could not import original safe facts", err)
	}
	orderedProgress := func() []Progress {
		pending := j.PendingProgress()
		for _, p := range pending {
			if p.HostOperation == nil || p.ToolEvent != nil || p.Message != "" {
				t.Fatal("workspace recovery mixed another progress channel")
			}
		}
		// Recovered terminal facts can be imported after another already-saved
		// start. Transport order is not the original host sequence.
		sort.Slice(pending, func(i, k int) bool { return pending[i].HostOperation.Sequence < pending[k].HostOperation.Sequence })
		return pending
	}
	pending := orderedProgress()
	if len(pending) != 4 || pending[1].HostOperation.Phase != "completed" || pending[3].HostOperation.Phase != "unknown" || pending[3].HostOperation.CompletedAt != "" || pending[3].HostOperation.ErrorCode != "runner_restart" {
		t.Fatal("restart lost host facts or invented check completion", pending)
	}
	last := *pending[3].HostOperation
	if data, err := os.ReadFile(filepath.Join(w.Source, "file.txt")); err != nil || string(data) != "one durable write\n" {
		t.Fatal("restart changed the original write", err)
	}
	if len(j.Pending()) != 1 || !j.Pending()[0].OutcomeUnknown {
		t.Fatal("unfinished attempt was silently resumed")
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if again := orderedProgress(); len(again) != 4 || !reflectWorkspaceEvent(last, *again[3].HostOperation) {
		t.Fatal("second recovery duplicated or replaced an original unknown event")
	}
	// A child JSON frame cannot select the independent Go-host channel.
	stream := newToolStream(structuredProfile("suma-jsonl-v1"), nil, nil, "request")
	_, _ = stream.Write([]byte(`{"request_id":"request","type":"started","host_operation":{}}` + "\n"))
	if stream.bad == nil {
		t.Fatal("SDK forged host operation facts")
	}
	if j.RecordToolProgress(a, tc.Event{SchemaVersion: 1, CommandID: "native", ProfileSHA256: n.traceProfileSHA, Protocol: wc.Protocol, Kind: "activity_completed"}) == nil {
		t.Fatal("SDK journal accepted the host protocol")
	}
}

func reflectWorkspaceEvent(a, b wc.Event) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}

func TestWorkspaceOperationReplayDrainsHostBeforeStaleSDK(t *testing.T) {
	for _, pool := range []bool{false, true} {
		t.Run(fmt.Sprintf("pool-%t", pool), func(t *testing.T) {
			a := Assignment{TenantID: "tenant", ProjectID: "project", RunID: "run", NodeID: "node", Attempt: 1, Fence: 2, WorkerID: "worker"}
			if pool {
				a = poolTestAssignment()
			}
			a.Type, a.Command = "agent", []string{"native"}
			a.Config = map[string]any{"tool_profile_sha256": strings.Repeat("a", 64)}
			n, w, _ := nativeFixtureAssignment(t, a, nil, nil)
			j, err := OpenJournal(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer j.Close()
			if err = j.Accept(a); err != nil {
				t.Fatal(err)
			}
			if err = j.attachWorkspaceTrace(a, w.Directory, n.policySHA, n.traceProfileSHA); err != nil {
				t.Fatal(err)
			}
			if err = j.RecordProgress(a, "owned stale SDK chunk"); err != nil {
				t.Fatal(err)
			}
			n.traceJournal = j
			if _, err = nativeCall(t, n, "read", "original-read", map[string]any{"path": "file.txt"}); err != nil {
				t.Fatal(err)
			}
			host := []int{}
			sdk := 0
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.Method != "POST" || request.Header.Get("Authorization") != "Bearer owned-machine" {
					t.Error("invalid original progress transport")
					response.WriteHeader(400)
					return
				}
				var body map[string]json.RawMessage
				if json.NewDecoder(request.Body).Decode(&body) != nil {
					t.Error("invalid progress JSON")
					response.WriteHeader(400)
					return
				}
				if raw, present := body["host_operation"]; present {
					event, err := wc.DecodeEvent(raw)
					if err != nil || wc.ValidateBoundEvent(workspaceOperationIdentity(a), event) != nil || body["message"] != nil || body["tool_event"] != nil {
						t.Error("host replay mixed another channel")
						response.WriteHeader(400)
						return
					}
					host = append(host, event.Sequence)
					response.WriteHeader(200)
					return
				}
				sdk++
				response.WriteHeader(409)
			}))
			defer server.Close()
			client := &Client{BaseURL: server.URL, Token: "owned-machine", ProjectID: a.ProjectID}
			if pool {
				client.Pool = &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID}
			}
			replay := func() error {
				if pool {
					return replayPool(context.Background(), client, j)
				}
				return Replay(context.Background(), client, j)
			}
			err = replay()
			var status *runnerHTTPStatusError
			if !errors.As(err, &status) || status.Status != 409 || len(host) != 2 || host[0] != 1 || host[1] != 2 || sdk != 1 {
				t.Fatal("stale SDK blocked original host history or its error disappeared", host, sdk, err)
			}
			pending := j.PendingProgress()
			if len(pending) != 1 || pending[0].HostOperation != nil || pending[0].Message != "owned stale SDK chunk" {
				t.Fatal("SDK progress was dropped or host facts were not acknowledged", pending)
			}
			original := pending[0].OperationID
			if err = replay(); !errors.As(err, &status) || len(host) != 2 || sdk != 2 || len(j.PendingProgress()) != 1 || j.PendingProgress()[0].OperationID != original {
				t.Fatal("replay duplicated acknowledged host facts or replaced original SDK progress", err)
			}
		})
	}
}
