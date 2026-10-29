package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
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

	mc "github.com/ai-mada/suma/internal/modelcontract"
	"github.com/ai-mada/suma/internal/runner"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Opt-in actual Go -> OS Node bundle -> local Go gateway -> owned provider
// journey. Responses are deterministic protocol fixtures, not a commercial
// model-quality evaluation or a claim that the assistant executed terminal code.
func TestTaskConversationReplyActualTwoTurnGatewayAndRecovery(t *testing.T) {
	cli := os.Getenv("SUMA_CONVERSATION_REPLY_CLI")
	if cli == "" {
		t.Skip("an explicitly built current native reply bundle is required")
	}
	ctx := context.Background()
	f := newFixture(t)
	f.e.AttemptAccounting = f.s
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cli, err = filepath.Abs(cli)
	if err != nil || !under(filepath.Join(root, ".cache", "e2e", "reply-128", "native"), cli) {
		t.Fatal("use the isolated current stage native bundle", err)
	}
	bundle, err := os.ReadFile(cli)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		EntrySHA256 string `json:"entry_sha256"`
	}
	workbenchRetainedJSON(t, filepath.Join(filepath.Dir(cli), "suma-agent.manifest.json"), &manifest)
	bundleSHA := fmt.Sprintf("%x", sha256.Sum256(bundle))
	if manifest.EntrySHA256 != bundleSHA {
		t.Fatal("native bundle manifest differs from actual bytes")
	}
	// Validate and reserve the unique evidence directory before model execution.
	// A failed live attempt must survive the fixture's temporary DB cleanup.
	evidenceDir, evidenceWeb := os.Getenv("SUMA_CONVERSATION_REPLY_UI_DIR"), os.Getenv("SUMA_CONVERSATION_REPLY_WEB_DIR")
	if evidenceDir != "" || evidenceWeb != "" {
		owned := filepath.Join(root, ".cache", "e2e", "reply-128")
		evidenceWeb = workbenchRetainedDirectory(t, evidenceWeb, owned, false)
		if _, err = os.Stat(filepath.Join(evidenceWeb, "index.html")); err != nil {
			t.Fatal(err)
		}
		evidenceDir = workbenchRetainedDirectory(t, evidenceDir, owned, true)
		if under(evidenceDir, evidenceWeb) || under(evidenceWeb, evidenceDir) {
			t.Fatal("evidence and web assets must be separate")
		}
		modelSelectionGUIJSON(t, filepath.Join(evidenceDir, "attempt.started.public.json"), map[string]any{"stage": "T-REPLY-128", "bundle_sha256": bundleSHA, "kind": "actual_native_gateway_two_turn_conversation"})
	}
	code := t.TempDir()
	const original = "module.exports=(a,b)=>a+b;\n"
	workspaceGitTestWrite(t, code, "sum.cjs", original)
	t.Setenv("SUMA_WORKSPACE_ROOT", code)
	requirement := issueRepairRuntimeDecode[Record](t, f.call("POST", "/requirements", f.body(map[string]any{"title": "Continuous task conversation / 持续任务讨论", "description": "Keep model and budget configuration bound to each actual reply, without executing suggested actions."}), 201))
	task := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "真实智能体多轮讨论 / Real assistant discussion", "description": "Discussion remains separate from code execution and formal acceptance.", "requirement_id": requirement.ID}), 201))
	f.call("POST", "/workspace/binding", f.body(map[string]any{"path": code}), 200)
	const firstHuman = "请根据当前需求说明下一步。"
	const followupHuman = "继续说明预算和执行门禁，不启动开发任务。"
	answers := []string{"先确认需求和验收条目，再通过正式开发计划执行；此回复未运行终端、开发或测试。", "Use the explicit model budget and reviewed execution plan. The prior discussion is context, and no terminal or code task has been started.", "The UI-requested follow-up uses the selected model and budget. 两轮真实回复已作为固定上下文保留，本次仍未启动终端、开发、测试或验收。"}
	var mu sync.Mutex
	calls := 0
	requestHashes, responseHashes := []string{}, []string{}
	httpTranscripts := []map[string]any{}
	var priorReplyID, priorMessageID string
	t.Cleanup(func() {
		if evidenceDir == "" {
			return
		}
		if _, e := os.Stat(filepath.Join(evidenceDir, "result.public.json")); e == nil {
			return
		}
		mu.Lock()
		actualCalls := calls
		transcript := append([]map[string]any{}, httpTranscripts...)
		mu.Unlock()
		state := map[string]any{"stage": "T-REPLY-128", "status": "partial_failure_before_gui", "test_failed": t.Failed(), "model_calls": actualCalls, "bundle_sha256": bundleSHA, "source_sha256": dispatchRecommendationRuntimeHash([]byte(original)), "production_acceptance": false}
		path := filepath.Join(evidenceDir, "failure-fixture.sqlite")
		if _, e := f.s.DB.Exec("VACUUM INTO '" + strings.ReplaceAll(path, "'", "''") + "'"); e != nil {
			t.Error("retain failed actual DB", e)
		} else if raw, e := os.ReadFile(path); e != nil {
			t.Error(e)
		} else {
			state["database_sha256"], state["database_bytes"] = dispatchRecommendationRuntimeHash(raw), len(raw)
		}
		if e := os.WriteFile(filepath.Join(evidenceDir, "sum.cjs"), []byte(original), 0600); e != nil {
			t.Error(e)
		}
		modelSelectionGUIJSON(t, filepath.Join(evidenceDir, "model-http.public.json"), transcript)
		modelSelectionGUIJSON(t, filepath.Join(evidenceDir, "failure.public.json"), state)
	})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, e := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var body map[string]any
		if e != nil || json.Unmarshal(raw, &body) != nil || r.Method != "POST" || r.URL.Path != "/v1/chat/completions" {
			t.Error("unexpected owned model protocol")
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		calls++
		turn := calls
		maxCalls := 2
		if evidenceDir != "" {
			maxCalls = 3
		}
		if turn > maxCalls || body["model"] != "conversation128" || body["stream"] != false || body["tool_choice"] != "none" || body["tools"] != nil || r.Header.Get("Authorization") != "Bearer owned-conversation128" {
			t.Error("unselected model, tools or repeated model execution")
			w.WriteHeader(400)
			return
		}
		allowance := float64(512)
		if turn == 2 {
			allowance = 1024
		}
		if turn == 3 {
			allowance, _ = body["max_completion_tokens"].(float64)
			if allowance != 512 && allowance != 1024 {
				t.Error("GUI must explicitly select 512 or 1024 output tokens")
				w.WriteHeader(400)
				return
			}
		}
		if body["max_completion_tokens"] != allowance {
			t.Error("selected output parameter did not reach actual model request")
			w.WriteHeader(400)
			return
		}
		messages, ok := body["messages"].([]any)
		if !ok || len(messages) != 2 {
			t.Error("bounded system/user context missing")
			w.WriteHeader(400)
			return
		}
		message, ok := messages[1].(map[string]any)
		content, _ := message["content"].(string)
		if !ok || !strings.Contains(content, firstHuman) || turn >= 2 && (!strings.Contains(content, followupHuman) || !strings.Contains(content, answers[0]) || priorReplyID == "" || !strings.Contains(content, priorReplyID) || !strings.Contains(content, priorMessageID)) || turn == 3 && (!strings.Contains(content, "T128 browser follow-up") || !strings.Contains(content, answers[1])) {
			t.Error("follow-up model context lacks original protected assistant and latest human message")
			w.WriteHeader(400)
			return
		}
		reply := []byte(stringify(map[string]any{"id": fmt.Sprintf("owned-conversation-response-%d", turn), "model": "conversation128", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"content": stringify(map[string]any{"schema_version": 1, "reply": answers[turn-1]})}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 7, "completion_tokens": 3}}))
		requestHashes = append(requestHashes, dispatchRecommendationRuntimeHash(raw))
		responseHashes = append(responseHashes, dispatchRecommendationRuntimeHash(reply))
		httpTranscripts = append(httpTranscripts, map[string]any{"turn": turn, "request_json": string(raw), "response_json": string(reply), "request_sha256": dispatchRecommendationRuntimeHash(raw), "response_sha256": dispatchRecommendationRuntimeHash(reply)})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(reply)
	}))
	defer provider.Close()
	endpoint := provider.URL + "/v1"
	endpointSHA, err := mc.EndpointDigest(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	deployment := mc.Deployment{ID: "conversation-deployment128", Revision: 1, Protocol: mc.ProviderProtocol, Model: "conversation128", EndpointOrigin: provider.URL, EndpointConfigSHA256: endpointSHA, ContextWindow: 32768}
	deployment.SHA256 = mc.DeploymentDigest(deployment)
	t.Setenv("SUMA_OWNED_CONVERSATION128", "owned-conversation128")
	cfg := runner.ExecConfig{AllowExec: true, Workspace: code, WorktreeRoot: filepath.Join(t.TempDir(), "private-model-attempts"), Commands: map[string]runner.CommandSpec{"reply128": {Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: node, Args: []string{cli}, Input: "agent-jsonl", ReadOnly: true, TaskConversationReply: true, ModelExecutionProtocol: mc.Protocol}}, ModelDeployments: []runner.ModelDeploymentSpec{{Deployment: deployment, BaseURL: endpoint, APIKeyEnvRef: "SUMA_OWNED_CONVERSATION128"}}}
	profile, token := issueRepairRuntimeTool(t, f, "conversation-worker128", &cfg)
	rate := modelRateForTest(t, f, profile, deployment, 0)
	originalHandler := f.handler
	dropped, replayed := false, 0
	firstCallbackSHA := ""
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/runner/results" {
			originalHandler.ServeHTTP(w, r)
			return
		}
		raw, readErr := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(raw))
		if readErr != nil {
			t.Error("read original callback", readErr)
			w.WriteHeader(400)
			return
		}
		callbackSHA := dispatchRecommendationRuntimeHash(raw)
		mu.Lock()
		first := !dropped
		if !first && callbackSHA == firstCallbackSHA {
			replayed++
		}
		mu.Unlock()
		if !first {
			originalHandler.ServeHTTP(w, r)
			return
		}
		response := httptest.NewRecorder()
		originalHandler.ServeHTTP(response, r)
		if response.Code == 200 {
			if h, ok := w.(http.Hijacker); ok {
				connection, _, e := h.Hijack()
				if e == nil {
					mu.Lock()
					dropped = true
					firstCallbackSHA = callbackSHA
					mu.Unlock()
					_ = connection.Close()
					return
				}
			}
		}
		for key, values := range response.Header() {
			w.Header()[key] = values
		}
		w.WriteHeader(response.Code)
		_, _ = w.Write(response.Body.Bytes())
	}))
	defer control.Close()
	client := &runner.Client{BaseURL: control.URL, Token: token, ProjectID: f.project}
	replies := []TaskConversationReply{}
	results := []runner.Result{}
	tokenLimit, costLimit := int64(40000), int64(40000)
	for turn := 0; turn < 2; turn++ {
		text := firstHuman
		if turn == 1 {
			text = followupHuman
		}
		note := issueRepairRuntimeDecode[TaskConversationMessage](t, f.call("POST", "/task-conversations", taskConversationRequest(f, task.ID, task.Version, text), 201))
		sequence := int64(1 + turn*2)
		if note.Sequence != sequence {
			t.Fatal("human message ordering changed", note)
		}
		output := int64(512)
		locale := "zh-CN"
		if turn == 1 {
			output = 1024
			locale = "en-US"
		}
		selection := &mc.Selection{DeploymentID: deployment.ID, DeploymentRevision: deployment.Revision, DeploymentSHA256: deployment.SHA256, Parameters: mc.Parameters{MaxSteps: 1, MaxOutputTokens: output}, Budget: &mc.Budget{TokenLimit: &tokenLimit, CostLimitMicros: &costLimit, Currency: "CNY"}}
		request := TaskConversationReplyCreateRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, ContextMessageLimit: 20, ExpectedLastSequence: sequence, ReplyToSequence: sequence, ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version, ModelExecutionSelection: selection, Locale: locale, AcknowledgeModelUse: true, IdempotencyKey: newID()}
		reply := issueRepairRuntimeDecode[TaskConversationReply](t, f.call("POST", "/task-conversation-replies", request, 201))
		duplicate := issueRepairRuntimeDecode[TaskConversationReply](t, f.call("POST", "/task-conversation-replies", request, 201))
		if duplicate.ID != reply.ID || duplicate.RunID != reply.RunID {
			t.Fatal("create replay made another model run")
		}
		if reply.ModelExecution == nil || reply.ModelExecution.Parameters.MaxOutputTokens != output || reply.ModelExecution.Budget == nil || *reply.ModelExecution.Budget.TokenLimit != tokenLimit {
			t.Fatal("explicit model parameters or hard budget were not frozen")
		}
		assignment, e := client.Claim(ctx, profile.WorkerID, []string{"command:reply128"})
		if e != nil || assignment == nil || assignment.RunID != reply.RunID {
			t.Fatal("exact conversation run was not claimed", e)
		}
		result := runner.Execute(ctx, *assignment, cfg)
		result.OperationID = newID()
		if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || result.ToolEvidence == nil || result.ToolEvidence.ModelExecution == nil || len(result.WorkspaceEvidence) > 0 {
			t.Fatalf("actual assistant runtime failed: exit%d unknown%v error%s", result.ExitCode, result.OutcomeUnknown, result.Error)
		}
		if turn == 0 {
			if e = client.Complete(ctx, result); e == nil {
				t.Fatal("post-commit result ACK was not lost")
			}
		}
		if e = client.Complete(ctx, result); e != nil {
			t.Fatal("original callback recovery failed", e)
		}
		reply = issueRepairRuntimeDecode[TaskConversationReply](t, f.call("GET", "/task-conversation-replies/"+reply.ID+"?project_id="+f.project, nil, 200))
		recoveredCreate := issueRepairRuntimeDecode[TaskConversationReply](t, f.call("POST", "/task-conversation-replies", request, 201))
		if recoveredCreate.ID != reply.ID || recoveredCreate.MessageID != reply.MessageID || recoveredCreate.State != "completed" {
			t.Fatal("original create recovery after completion changed message identity")
		}
		page, e := f.s.ListTaskConversation(ctx, f.actor, f.project, task.ID, 20, 0)
		if e != nil || reply.State != "completed" || reply.MessageSequence != sequence+1 || page.Total != sequence+1 {
			t.Fatal("real completed reply was not appended once", e, reply)
		}
		message := page.Items[len(page.Items)-1]
		if message.ActorKind != "assistant" || message.Body != answers[turn] || message.ReplyID != reply.ID || message.ReplyToSequence != sequence || message.ExecutionRunID != reply.RunID || message.ID != reply.MessageID {
			t.Fatal("assistant attribution lacks protected original runtime provenance", message)
		}
		current, e := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
		if e != nil || current.Version != task.Version || current.RunID != task.RunID || current.Status != task.Status {
			t.Fatal("a discussion reply changed code-task execution or acceptance", e)
		}
		mu.Lock()
		priorReplyID, priorMessageID = reply.ID, message.ID
		mu.Unlock()
		replies = append(replies, reply)
		results = append(results, result)
	}
	mu.Lock()
	initialCalls, initialDropped, initialReplayed := calls, dropped, replayed
	initialRequests, initialResponses := append([]string{}, requestHashes...), append([]string{}, responseHashes...)
	mu.Unlock()
	if initialCalls != 2 || !initialDropped || initialReplayed != 1 || len(initialRequests) != 2 || len(initialResponses) != 2 {
		t.Fatal("actual model requests or callback recovery differ", initialCalls, initialDropped, initialReplayed)
	}
	for i, result := range results {
		e := result.ToolEvidence.ModelExecution
		if !e.GatewayClosed || len(e.Requests) != 1 || e.Requests[0].RequestSHA256 != initialRequests[i] || e.Requests[0].ResponseSHA256 != initialResponses[i] || e.Requests[0].ProviderModel != deployment.Model || e.Requests[0].UsageState != tc.UsageComplete {
			t.Fatal("host model receipt differs from actual HTTP", i)
		}
	}
	attempts, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 100, "")
	if err != nil {
		t.Fatal(err)
	}
	settled := 0
	for _, attempt := range attempts.Items {
		if attempt.Model == deployment.Model {
			if attempt.State != "settled" || attempt.Rate == nil || attempt.Rate.ID != rate.ID || attempt.ReportedTokens == nil || *attempt.ReportedTokens != 10 || attempt.EstimatedCostMicros == nil || *attempt.EstimatedCostMicros != 13 || attempt.HeldTokens != 0 {
				t.Fatal("reply usage was not settled exactly once at the original rate", attempt)
			}
			settled++
		}
	}
	if settled != 2 {
		t.Fatal("two replies require two exact settled usage attempts", settled)
	}
	receipt := map[string]any{"stage": "T-REPLY-128", "kind": "actual_native_gateway_two_turn_conversation", "task_id": task.ID, "task_version": task.Version, "task_run_id": task.RunID, "task_status": task.Status, "reply_ids": []string{replies[0].ID, replies[1].ID}, "reply_run_ids": []string{replies[0].RunID, replies[1].RunID}, "message_ids": []string{replies[0].MessageID, replies[1].MessageID}, "messages": 4, "model_requests": 2, "reported_tokens": 20, "estimated_cost_micros": 26, "rate_id": rate.ID, "callback_ack_lost_after_commit": dropped, "original_callback_replays_after_loss": replayed, "bundle_sha256": bundleSHA, "source_sha256": dispatchRecommendationRuntimeHash([]byte(original)), "customer_human_signoff": false, "commercial_model_quality_verified": false, "terminal_executions": 0}
	t.Logf("actual_task_conversation_reply_receipt=%s", stringify(receipt))
	snapshot := func() taskConversationReplyRuntimeSnapshot {
		mu.Lock()
		defer mu.Unlock()
		return taskConversationReplyRuntimeSnapshot{Calls: calls, Requests: append([]string{}, requestHashes...), Responses: append([]string{}, responseHashes...), Transcripts: append([]map[string]any{}, httpTranscripts...), Dropped: dropped, Replayed: replayed}
	}
	gui := taskConversationReplyRuntimeGUI{Client: client, Config: cfg, Profile: profile, Task: task, Deployment: deployment, RateID: rate.ID, Replies: replies, ExpectedThird: answers[2], Snapshot: snapshot}
	taskConversationReplyRuntimeHold(t, f, code, original, receipt, evidenceDir, evidenceWeb, gui)
}

type taskConversationReplyRuntimeSnapshot struct {
	Calls               int
	Requests, Responses []string
	Transcripts         []map[string]any
	Dropped             bool
	Replayed            int
}
type taskConversationReplyRuntimeGUI struct {
	Client        *runner.Client
	Config        runner.ExecConfig
	Profile       ToolProfile
	Task          Record
	Deployment    mc.Deployment
	RateID        string
	Replies       []TaskConversationReply
	ExpectedThird string
	Snapshot      func() taskConversationReplyRuntimeSnapshot
}
type taskConversationReplyRuntimeWorkerResult struct {
	Reply  TaskConversationReply
	Result runner.Result
	Error  error
}

// The UI posts one specific follow-up and explicitly selects its model/budget.
// This bounded worker executes at most that one additional managed reply; it
// never creates a model request in response to opening or refreshing the page.
func taskConversationReplyRuntimeHold(t *testing.T, f *fixture, code, source string, receipt map[string]any, dir, web string, g taskConversationReplyRuntimeGUI) {
	t.Helper()
	if dir == "" && web == "" {
		return
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(root, ".cache", "e2e", "reply-128")
	web = workbenchRetainedDirectory(t, web, owned, false)
	dir = workbenchRetainedDirectory(t, dir, owned, false)
	if under(web, dir) || under(dir, web) {
		t.Fatal("web assets and private receipts must remain separate")
	}
	server, origin, port := modelSelectionGUIServer(t, f, web, "reply128.localhost")
	t.Cleanup(server.Close)
	seed, seedSHA := workbenchRetainedProtected(t, f)
	state := map[string]any{"status": "ready_for_explicit_ui_followup", "kind": "actual_native_gateway_ui_followup", "origin": origin, "receipt": receipt, "seed_counts": seed, "seed_protected_sha256": seedSHA, "initial_model_calls": 2, "expected_final_model_calls": 3, "stop_reason": "timeout"}
	journal, err := runner.OpenJournal(filepath.Join(dir, "runner-journal"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = journal.Close() })
	workerCtx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	resultChannel := make(chan taskConversationReplyRuntimeWorkerResult, 1)
	go func() {
		defer close(finished)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		cfg := g.Config
		nextInventory := time.Time{}
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				if !time.Now().Before(nextInventory) {
					// The UI must see current actual CLI/model discovery while idle.
					// Probes never call the model; the persisted report retains its
					// identity until the control plane acknowledges that operation.
					items, e := runner.DescribeTools(workerCtx, cfg)
					if e != nil {
						resultChannel <- taskConversationReplyRuntimeWorkerResult{Error: fmt.Errorf("GUI idle discovery: %w", e)}
						return
					}
					report, e := journal.NewInventory(g.Profile.WorkerID, items)
					if e == nil {
						var observed string
						observed, e = g.Client.ToolInventory(workerCtx, report)
						if e == nil {
							e = journal.AckInventory(report.OperationID, observed)
						}
					}
					if e != nil {
						resultChannel <- taskConversationReplyRuntimeWorkerResult{Error: fmt.Errorf("GUI idle inventory: %w", e)}
						return
					}
					cfg.ToolInventory = items
					nextInventory = time.Now().Add(30 * time.Second)
				}
				assignment, e := g.Client.Claim(workerCtx, g.Profile.WorkerID, []string{"command:reply128"})
				if e != nil {
					if workerCtx.Err() != nil {
						return
					}
					resultChannel <- taskConversationReplyRuntimeWorkerResult{Error: fmt.Errorf("GUI worker claim: %w", e)}
					return
				}
				if assignment == nil {
					continue
				}
				if assignment.NodeID != "reply" || assignment.RunID == g.Replies[0].RunID || assignment.RunID == g.Replies[1].RunID {
					resultChannel <- taskConversationReplyRuntimeWorkerResult{Error: fmt.Errorf("GUI worker received an unexpected execution")}
					return
				}
				result := runner.Execute(workerCtx, *assignment, cfg)
				result.OperationID = newID()
				if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || result.ToolEvidence == nil || result.ToolEvidence.ModelExecution == nil || len(result.WorkspaceEvidence) > 0 {
					resultChannel <- taskConversationReplyRuntimeWorkerResult{Result: result, Error: fmt.Errorf("actual GUI reply execution failed: %s", result.Error)}
					return
				}
				if e = g.Client.Complete(workerCtx, result); e != nil {
					resultChannel <- taskConversationReplyRuntimeWorkerResult{Result: result, Error: fmt.Errorf("GUI reply original completion: %w", e)}
					return
				}
				var replyID string
				if e = f.s.DB.QueryRow(`SELECT id FROM suma_task_conversation_replies WHERE tenant_id=? AND project_id=? AND run_id=?`, f.user.TenantID, f.project, assignment.RunID).Scan(&replyID); e != nil {
					resultChannel <- taskConversationReplyRuntimeWorkerResult{Result: result, Error: e}
					return
				}
				reply, e := f.s.GetTaskConversationReply(workerCtx, f.e, f.actor, f.project, replyID)
				resultChannel <- taskConversationReplyRuntimeWorkerResult{Reply: reply, Result: result, Error: e}
				return
			}
		}
	}()
	t.Cleanup(func() {
		cancel()
		<-finished
		server.Close()
		final, finalSHA := workbenchRetainedProtected(t, f)
		state["final_counts"], state["final_protected_sha256"] = final, finalSHA
		observation := g.Snapshot()
		state["final_model_calls"], state["callback_ack_lost_after_commit"], state["original_callback_replays_after_loss"] = observation.Calls, observation.Dropped, observation.Replayed
		// Only the third DAG/model accounting records may change in this window.
		mutable := map[string]bool{"dag_runs": true, "dag_nodes": true, "dag_attempts": true, "dag_events": true, "suma_usage_attempts": true, "suma_usage_facts": true}
		for table, before := range seed {
			if !mutable[table] && final[table] != before {
				state["status"] = "unexpected_business_write"
				t.Error("GUI changed unrelated business facts", table)
			}
		}
		task, e := f.s.Get(context.Background(), f.actor, f.project, "tasks", g.Task.ID)
		if e != nil || task.Version != g.Task.Version || task.RunID != g.Task.RunID || task.Status != g.Task.Status || task.RequirementID != g.Task.RequirementID {
			state["status"] = "task_changed"
			t.Error("GUI reply changed task execution/version/acceptance", e)
		}
		var work taskConversationReplyRuntimeWorkerResult
		select {
		case work = <-resultChannel:
		default:
			work.Error = fmt.Errorf("the product UI did not create and complete its one explicit follow-up")
		}
		state["worker_error"] = ""
		if work.Error != nil {
			state["worker_error"] = work.Error.Error()
			state["status"] = "ui_followup_not_verified"
			t.Error(work.Error)
		}
		page, e := f.s.ListTaskConversation(context.Background(), f.actor, f.project, g.Task.ID, 20, 0)
		state["conversation"] = page
		state["third_reply"] = work.Reply
		if e != nil || page.Total != 6 || len(page.Items) != 6 || page.Items[4].ActorKind != "human" || page.Items[4].Body != "T128 browser follow-up" || page.Items[5].ActorKind != "assistant" || page.Items[5].Body != g.ExpectedThird || page.Items[5].ReplyID != work.Reply.ID || page.Items[5].ReplyToSequence != 5 || page.Items[5].ExecutionRunID != work.Reply.RunID || work.Reply.State != "completed" || work.Reply.Source.ReplyToSequence != 5 || work.Reply.Source.Coverage.Included != 5 {
			state["status"] = "ui_message_chain_invalid"
			t.Error("GUI follow-up lacks exact continuous message and runtime provenance", e)
		}
		binding := work.Reply.ModelExecution
		if binding == nil || binding.Deployment != g.Deployment || binding.Parameters.MaxSteps != 1 || (binding.Parameters.MaxOutputTokens != 512 && binding.Parameters.MaxOutputTokens != 1024) || binding.Budget == nil || binding.Budget.TokenLimit == nil || *binding.Budget.TokenLimit != 40000 || binding.Budget.CostLimitMicros == nil || *binding.Budget.CostLimitMicros != 40000 || binding.Budget.Currency != "CNY" {
			state["status"] = "ui_model_budget_not_verified"
			t.Error("UI model, output allowance and hard budget were not frozen")
		}
		if observation.Calls != 3 || len(observation.Requests) != 3 || len(observation.Responses) != 3 || !observation.Dropped || observation.Replayed != 1 {
			state["status"] = "unexpected_model_count"
			t.Error("UI chain performed unexpected model requests or callback replay")
		}
		if work.Result.ToolEvidence == nil || work.Result.ToolEvidence.ModelExecution == nil {
			state["status"] = "ui_gateway_evidence_missing"
			t.Error("GUI reply has no actual Go gateway evidence")
		} else {
			model := work.Result.ToolEvidence.ModelExecution
			if !model.GatewayClosed || len(model.Requests) != 1 || len(observation.Requests) != 3 || len(observation.Responses) != 3 || model.Requests[0].RequestSHA256 != observation.Requests[2] || model.Requests[0].ResponseSHA256 != observation.Responses[2] || model.Requests[0].ProviderModel != g.Deployment.Model || model.Requests[0].UsageState != tc.UsageComplete {
				state["status"] = "ui_gateway_receipt_invalid"
				t.Error("UI result differs from actual third model HTTP")
			}
		}
		attempts, e := f.s.ListUsageAttempts(context.Background(), f.actor, f.project, "", "", 100, "")
		settled := 0
		if e != nil {
			t.Error(e)
		} else {
			for _, a := range attempts.Items {
				if a.Model == g.Deployment.Model {
					if a.State != "settled" || a.Rate == nil || a.Rate.ID != g.RateID || a.ReportedTokens == nil || *a.ReportedTokens != 10 || a.EstimatedCostMicros == nil || *a.EstimatedCostMicros != 13 || a.HeldTokens != 0 {
						state["status"] = "usage_not_settled_once"
						t.Error("GUI turn usage did not settle at the original rate")
					}
					settled++
				}
			}
		}
		state["settled_usage_attempts"], state["reported_tokens"], state["estimated_cost_micros"] = settled, settled*10, settled*13
		if settled != 3 {
			state["status"] = "usage_count_invalid"
			t.Error("three explicit turns require exactly three original usage attempts")
		}
		if raw, e := os.ReadFile(filepath.Join(code, "sum.cjs")); e != nil || !bytes.Equal(raw, []byte(source)) {
			state["status"] = "source_changed"
			t.Error("GUI modified original code", e)
		}
		snapshot := filepath.Join(dir, "fixture.sqlite")
		if _, e := f.s.DB.Exec("VACUUM INTO '" + strings.ReplaceAll(snapshot, "'", "''") + "'"); e != nil {
			t.Error(e)
		} else if raw, e := os.ReadFile(snapshot); e != nil {
			t.Error(e)
		} else {
			state["database_bytes"], state["database_sha256"] = len(raw), dispatchRecommendationRuntimeHash(raw)
		}
		if e := os.WriteFile(filepath.Join(dir, "sum.cjs"), []byte(source), 0600); e != nil {
			t.Error(e)
		}
		modelSelectionGUIJSON(t, filepath.Join(dir, "model-http.public.json"), observation.Transcripts)
		modelSelectionGUIJSON(t, filepath.Join(dir, "result.public.json"), state)
	})
	modelSelectionGUIJSON(t, filepath.Join(dir, "model-http.public.json"), g.Snapshot().Transcripts)
	modelSelectionGUIJSON(t, filepath.Join(dir, "web.files.public.json"), modelSelectionGUIWebFiles(t, web))
	instructions := map[string]any{"origin": origin, "email": "owner@example.test", "password": "test-password-strong-42", "project_id": f.project, "task_id": g.Task.ID, "receipt": receipt, "allowed_actions": []string{"read", "post_exact_followup_once", "select_model_and_budget", "acknowledge_and_request_one_reply", "refresh"}, "followup": "T128 browser follow-up", "tool_profile_id": g.Profile.ID, "tool_profile_version": g.Profile.Version, "deployment": g.Deployment, "context_message_limit": 20, "max_steps": 1, "max_output_tokens_options": []int{512, 1024}, "token_budget": 40000, "cost_budget_micros": 40000, "currency": "CNY"}
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.local.json"), instructions)
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.ready"), map[string]any{"status": "ready", "origin": origin, "port": port, "project_id": f.project, "task_id": g.Task.ID, "model_calls": 2, "followup": "T128 browser follow-up", "expected_final_model_calls": 3})
	t.Logf("actual conversation UI ready at %s; explicitly post one follow-up, select model/budget and request its reply", origin)
	deadline, stop := time.NewTimer(7*time.Minute), time.NewTicker(200*time.Millisecond)
	defer deadline.Stop()
	defer stop.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("actual UI follow-up must close explicitly before timeout")
		case <-stop.C:
			if _, e := os.Lstat(filepath.Join(dir, "stop.request")); e == nil {
				state["stop_reason"], state["status"] = "explicit_stop", "closed_after_actual_ui_followup"
				return
			} else if !os.IsNotExist(e) {
				t.Fatal(e)
			}
		}
	}
}
