package runner

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	cc "github.com/ai-mada/suma/internal/codexcontract"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

// Opt-in actual native CLI transport; the model endpoint and human answer are
// owned deterministic fixtures. This is not a commercial-model or platform
// browser authorization test (those HTTP/SQL boundaries have separate cases).
func TestCodexUserInputActualInstalledCLIWithOwnedResponses(t *testing.T) {
	if os.Getenv("SUMA_TOOL_INPUT_ACTUAL") != "1" {
		t.Skip("explicit isolated installed CLI check required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(root, ".cache", "tool-inputs-187", "actual")
	if err = os.MkdirAll(stage, 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(stage, "run-"+time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	hash := func(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
	write := func(name string, v any) {
		raw, _ := json.MarshalIndent(v, "", "  ")
		if e := os.WriteFile(filepath.Join(dir, name), append(raw, '\n'), 0600); e != nil {
			t.Error(e)
		}
	}
	state := map[string]any{"stage": "T-TOOL-INPUTS-187", "status": "preparing", "commercial_calls": 0, "user_credentials": false, "existing_home_used": false, "model": "owned Responses fixture", "platform_http_sql_repeated": false, "version_features_schema_repeated": false}
	defer func() {
		if t.Failed() {
			state["status"] = "failed_original_attempt_preserved"
		}
		write("summary.json", state)
		t.Log("actual evidence:", dir)
	}()
	var pin struct {
		SHA     string `json:"executable_sha256"`
		Version string `json:"version"`
	}
	protocol, err := os.ReadFile(filepath.Join(root, ".cache", "tool-inputs-187", "protocol", "summary.json"))
	if err != nil || json.Unmarshal(protocol, &pin) != nil {
		t.Fatal("reuse current offline protocol receipt", err)
	}
	executable := os.Getenv("SUMA_CODEX_EXECUTABLE")
	if !filepath.IsAbs(executable) {
		t.Fatal("SUMA_CODEX_EXECUTABLE must explicitly select an absolute installed executable path")
	}
	binary, err := os.ReadFile(executable)
	if err != nil || hash(binary) != pin.SHA {
		t.Fatal("pinned installed executable differs", err)
	}
	binary = nil
	state["executable_sha256"], state["version"] = pin.SHA, pin.Version
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	var mu sync.Mutex
	providerCalls := 0
	model := ""
	var providerRequests, providerResponses []string
	var providerPaths []string
	var declaredTools any
	var originalToolOutput any
	var providerFailure string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		providerPaths = append(providerPaths, r.Method+" "+r.URL.Path)
		if r.Method != "POST" || r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "" {
			providerFailure = "unexpected_route_or_credential"
			http.Error(w, providerFailure, 400)
			return
		}
		var reader io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			z, e := gzip.NewReader(r.Body)
			if e != nil {
				http.Error(w, "gzip", 400)
				return
			}
			defer z.Close()
			reader = z
		}
		raw, e := io.ReadAll(io.LimitReader(reader, 4<<20))
		var body map[string]any
		if e != nil || len(raw) >= 4<<20 || json.Unmarshal(raw, &body) != nil || body["stream"] != true || body["model"] != model {
			providerFailure = "unexpected_model_request"
			http.Error(w, providerFailure, 400)
			return
		}
		providerCalls++
		providerRequests = append(providerRequests, hash(raw))
		_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("model-request-%d.local.json", providerCalls)), raw, 0600)
		var item map[string]any
		if providerCalls == 1 {
			declaredTools = body["tools"]
			name := ""
			var find func(any)
			find = func(v any) {
				switch x := v.(type) {
				case []any:
					for _, child := range x {
						find(child)
					}
				case map[string]any:
					if n, _ := x["name"].(string); strings.HasSuffix(n, "request_user_input") && x["parameters"] != nil {
						name = n
					}
					for _, child := range x {
						find(child)
					}
				}
			}
			find(declaredTools)
			if name == "" {
				providerFailure = "native_request_user_input_not_declared"
				http.Error(w, providerFailure, 400)
				return
			}
			args := map[string]any{"questions": []any{map[string]any{"id": "scope187", "header": "Scope", "question": "Which reviewed scope should remain fixed?", "isOther": false, "isSecret": false, "options": []any{map[string]any{"label": "Keep current scope", "description": "Preserve the reviewed engineering objective"}, map[string]any{"label": "Discuss another scope", "description": "Ask for a separate reviewed iteration"}}}}}
			encoded, _ := json.Marshal(args)
			item = map[string]any{"type": "function_call", "id": "fc_actual187", "call_id": "call_actual187", "name": name, "arguments": string(encoded)}
		} else if providerCalls == 2 {
			var find func(any)
			find = func(v any) {
				switch x := v.(type) {
				case []any:
					for _, child := range x {
						find(child)
					}
				case map[string]any:
					if x["type"] == "function_call_output" && x["call_id"] == "call_actual187" {
						originalToolOutput = x
					}
					for _, child := range x {
						find(child)
					}
				}
			}
			find(body["input"])
			encoded, _ := json.Marshal(originalToolOutput)
			if originalToolOutput == nil || !bytes.Contains(encoded, []byte("Keep current scope")) {
				providerFailure = "native_tool_output_missing_original_answer"
				http.Error(w, providerFailure, 400)
				return
			}
			item = map[string]any{"type": "message", "id": "msg_actual187", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "The explicitly reviewed scope was received. No file or command permission was granted.", "annotations": []any{}}}}
		} else {
			providerFailure = "additional_model_request_refused"
			http.Error(w, providerFailure, 400)
			return
		}
		response := actualInputSSE187(providerCalls, model, item)
		providerResponses = append(providerResponses, hash(response))
		_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("model-response-%d.local.sse", providerCalls)), response, 0600)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(response)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer provider.Close()
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		state["controlled_model_requests"] = providerCalls
		state["provider_request_sha256"] = providerRequests
		state["provider_response_sha256"] = providerResponses
		state["provider_paths"] = providerPaths
		state["provider_failure"] = providerFailure
		write("model-receipt.local.json", map[string]any{"declared_tools": declaredTools, "original_tool_output": originalToolOutput})
	}()
	workspace := filepath.Join(dir, "workspace")
	if err = os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	spec := CommandSpec{Adapter: "codex", Protocol: cc.Protocol, Executable: executable, Args: []string{"app-server", "--listen", "stdio://"}, Input: "codex-app-server", CodexAppServer: &CodexAppServerSpec{ProviderID: "owned187", BaseURL: provider.URL + "/v1", WindowsSandbox: "unelevated", EnableUserInput: true}}
	state["failing_stage"] = "current_local_catalog"
	catalog, err := probeCodexCatalog(ctx, executable, spec, ExecConfig{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	write("current-catalog.json", catalog)
	state["catalog_only_processes"] = 1
	choice := cc.Selection{CatalogSHA256: catalog.CatalogSHA256}
	for _, option := range catalog.Models {
		if len(option.ReasoningEfforts) > 0 {
			choice.Model, choice.ReasoningEffort = option.Model, option.DefaultReasoningEffort
			break
		}
	}
	if choice.Model == "" {
		t.Fatal("current native catalog has no explicit reasoning model")
	}
	binding, err := cc.NewBinding(*catalog, choice)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	model = choice.Model
	mu.Unlock()
	profileSHA, err := commandProfileDigest(spec, pin.SHA)
	if err != nil {
		t.Fatal(err)
	}
	profile := tc.Profile{CommandID: "codex-input-actual187", Adapter: "codex", Protocol: cc.Protocol, UserInputProtocol: cc.UserInputProtocol, ProfileSHA256: profileSHA, ExecutableSHA256: pin.SHA, ToolVersion: pin.Version}
	// Match the actual assignment's decoded wire map, rather than comparing a
	// struct's field-order JSON with the receiver's canonical map ordering.
	var bindingWire map[string]any
	bindingJSON, _ := json.Marshal(binding)
	if err = json.Unmarshal(bindingJSON, &bindingWire); err != nil {
		t.Fatal(err)
	}
	a := Assignment{TenantID: "tenant187", ProjectID: "project187", RunID: "run187", NodeID: "work", Attempt: 1, Fence: 1, WorkerID: "worker187", DispatchID: "dispatch187", Type: "agent", Command: []string{profile.CommandID}, Config: map[string]any{"goal": "Use request_user_input once for the reviewed scope. Do not run shell commands or edit files.", "tool_profile_sha256": profileSHA, "codex_execution": bindingWire}}
	j, err := OpenJournal(filepath.Join(dir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	if err = j.beginToolInputExecution(a, profile); err != nil {
		t.Fatal(err)
	}
	var controlMu sync.Mutex
	var record ic.ToolInput
	controlCalls := map[string]int{}
	receipts := map[string]ic.ToolInput{}
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		controlMu.Lock()
		defer controlMu.Unlock()
		controlCalls[r.URL.Path]++
		var body struct {
			ProjectID   string               `json:"project_id"`
			Assignment  Assignment           `json:"assignment"`
			Request     ic.ToolInputRequest  `json:"request"`
			InputID     string               `json:"input_id"`
			Key         string               `json:"idempotency_key"`
			Observation delivery.Observation `json:"observation"`
		}
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer owned-fixture-187" || json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body) != nil || body.ProjectID != a.ProjectID || !samePermissionValue(body.Assignment, a) {
			http.Error(w, "scope", 400)
			return
		}
		switch r.URL.Path {
		case "/api/v1/runner/tool-inputs":
			if ic.ValidateRequest(body.Request) != nil {
				http.Error(w, "request", 400)
				return
			}
			if record.ID == "" {
				now := time.Now().UTC()
				record = ic.ToolInput{ID: "input-actual187", ProjectID: a.ProjectID, RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, DispatchID: a.DispatchID, ProfileSHA256: profileSHA, Version: 1, State: "pending", Request: body.Request, CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(15 * time.Minute).Format(time.RFC3339Nano), AllowedActions: []string{}}
			} else if !samePermissionValue(body.Request, record.Request) {
				http.Error(w, "changed", 409)
				return
			}
		case "/api/v1/runner/tool-inputs/poll":
			if body.InputID != record.ID {
				http.Error(w, "locator", 409)
				return
			}
			if record.Answer == nil {
				selections := []cc.CodexUserInputSelection{{QuestionID: "scope187", Kind: "option", Option: "Keep current scope"}}
				sha, e := cc.UserInputAnswerDigest(*record.Request.Codex, selections)
				if e != nil {
					http.Error(w, "answer", 400)
					return
				}
				record.Answer = &ic.ToolInputAnswer{Answers: selections, AnswerSHA256: sha, ActorID: "fixture-reviewer187", AnsweredAt: time.Now().UTC().Format(time.RFC3339Nano)}
				record.Version++
				record.State = "answered"
			}
		case "/api/v1/runner/tool-inputs/observations":
			if body.InputID != record.ID || delivery.Validate(body.Observation) != nil || body.Observation.ParamsSHA256 != record.Request.Codex.ParamsSHA256 || body.Observation.AnswerObserved && (record.Answer == nil || body.Observation.AnswerSHA256 != record.Answer.AnswerSHA256) {
				http.Error(w, "observation", 409)
				return
			}
			if old, ok := receipts[body.Key]; ok {
				_ = json.NewEncoder(w).Encode(old)
				return
			}
			record.Version++
			record.Observation = &body.Observation
			if body.Observation.RequestResolved || body.Observation.ExecutionClosed {
				record.State = "closed"
			}
			raw, _ := json.Marshal(record)
			var snapshot ic.ToolInput
			_ = json.Unmarshal(raw, &snapshot)
			receipts[body.Key] = snapshot
		default:
			http.Error(w, "route", 404)
			return
		}
		if ic.ValidateInput(record) != nil {
			http.Error(w, "invalid fixture record", 500)
			return
		}
		_ = json.NewEncoder(w).Encode(record)
	}))
	defer control.Close()
	client := &Client{BaseURL: control.URL, Token: "owned-fixture-187", ProjectID: a.ProjectID}
	home := filepath.Join(dir, "codex-home")
	if err = prepareCodexHome(spec, home, workspace); err != nil {
		t.Fatal(err)
	}
	stream := newToolStream(profile, nil, nil)
	driver := newCodexAppServer(profile, spec, home, workspace, a.Config["goal"].(string), &binding, stream)
	defer driver.Close()
	if err = driver.configureUserInputs(ctx, codexToolInputHandler(a, ExecConfig{Journal: j, ToolInputClient: client, ToolInputProfile: profileSHA})); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, spec.Args...)
	cmd.Dir = home
	cmd.Env, _, err = codexEnvironment(spec, home)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TEMP", "TMP", "TMPDIR"} {
		cmd.Env = append(cmd.Env, name+"="+filepath.Join(dir, "tmp"))
	}
	if err = os.MkdirAll(filepath.Join(dir, "tmp"), 0700); err != nil {
		t.Fatal(err)
	}
	stdout, err := os.OpenFile(filepath.Join(dir, "native-stdout.local.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	stdin, err := os.OpenFile(filepath.Join(dir, "native-stdin.local.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	pipe, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	cmd.Stdout = io.MultiWriter(stdout, driver)
	stderr := newOutput(32<<10, nil)
	cmd.Stderr = stderr
	state["failing_stage"] = "actual_native_turn"
	ctl, err := startProcess(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer ctl.Close()
	state["question_process_pid"] = cmd.Process.Pid
	state["question_processes"] = 1
	copied := make(chan struct{})
	go func() { defer close(copied); _, _ = driver.CopyInput(io.MultiWriter(pipe, stdin)); _ = pipe.Close() }()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	forced := false
	select {
	case err = <-done:
	case <-ctx.Done():
		forced = true
		_ = driver.Close()
		_ = ctl.Stop()
		err = <-done
	}
	_ = driver.Close()
	<-copied
	driver.Finish()
	finishErr := ctl.Finish()
	state["forced_timeout"], state["process_error"], state["process_exit_code"], state["job_finish_error"] = forced, fmt.Sprint(err), cmd.ProcessState.ExitCode(), fmt.Sprint(finishErr)
	state["driver_error"], state["native_terminal"] = fmt.Sprint(driver.bad), stream.terminal
	write("stderr.local.json", map[string]any{"stderr": stderr.String()})
	write("native-evidence.json", driver.evidence)
	state["failing_stage"] = "original_observation_replay"
	replayErr := ReplayToolInputObservations(context.Background(), client, j)
	state["observation_replay_error"] = fmt.Sprint(replayErr)
	controlMu.Lock()
	write("control-receipt.local.json", map[string]any{"calls": controlCalls, "input": record, "observation_receipts": receipts})
	controlMu.Unlock()
	_ = stdin.Sync()
	_ = stdout.Sync()
	rawStdin, _ := os.ReadFile(filepath.Join(dir, "native-stdin.local.jsonl"))
	writes := 0
	scanner := bufio.NewScanner(bytes.NewReader(rawStdin))
	for scanner.Scan() {
		var frame map[string]any
		if json.Unmarshal(scanner.Bytes(), &frame) == nil {
			if result, ok := frame["result"].(map[string]any); ok && result["answers"] != nil {
				writes++
			}
		}
	}
	state["native_answer_writes"] = writes
	if forced || err != nil || finishErr != nil || driver.bad != nil || stream.terminal != "completed" || replayErr != nil {
		t.Fatal("actual CLI transport did not close successfully; original evidence retained")
	}
	if len(driver.evidence.UserInputs) != 1 || writes != 1 {
		t.Fatal("actual CLI did not produce exactly one native question and answer")
	}
	o := driver.evidence.UserInputs[0].Observation
	if o == nil || !o.ReplyStarted || !o.ReplyWritten || !o.RequestResolved || !o.TurnClosed || o.FailureCode != "" {
		t.Fatal("actual native answer closure is incomplete", o)
	}
	mu.Lock()
	calls, failure := providerCalls, providerFailure
	mu.Unlock()
	if calls != 2 || failure != "" {
		t.Fatal("actual native tool output did not reach the owned Responses continuation", calls, failure)
	}
	state["status"], state["failing_stage"] = "passed_actual_cli_owned_model_question", ""
}

func actualInputSSE187(call int, model string, item map[string]any) []byte {
	var out bytes.Buffer
	sequence := 0
	event := func(kind string, fields map[string]any) {
		fields["type"], fields["sequence_number"] = kind, sequence
		sequence++
		raw, _ := json.Marshal(fields)
		fmt.Fprintf(&out, "event: %s\ndata: %s\n\n", kind, raw)
	}
	response := map[string]any{"id": fmt.Sprintf("resp_owned187_%d", call), "object": "response", "created_at": 1, "status": "in_progress", "model": model, "output": []any{}}
	event("response.created", map[string]any{"response": response})
	initial := map[string]any{}
	for k, v := range item {
		initial[k] = v
	}
	initial["status"] = "in_progress"
	kind, _ := item["type"].(string)
	if kind == "message" {
		initial["content"] = []any{}
	} else {
		initial["arguments"] = ""
	}
	event("response.output_item.added", map[string]any{"output_index": 0, "item": initial})
	if kind == "message" {
		part := item["content"].([]any)[0].(map[string]any)
		event("response.content_part.added", map[string]any{"item_id": item["id"], "output_index": 0, "content_index": 0, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}})
		event("response.output_text.delta", map[string]any{"item_id": item["id"], "output_index": 0, "content_index": 0, "delta": part["text"]})
		event("response.output_text.done", map[string]any{"item_id": item["id"], "output_index": 0, "content_index": 0, "text": part["text"]})
		event("response.content_part.done", map[string]any{"item_id": item["id"], "output_index": 0, "content_index": 0, "part": part})
	} else {
		event("response.function_call_arguments.delta", map[string]any{"item_id": item["id"], "output_index": 0, "delta": item["arguments"]})
		event("response.function_call_arguments.done", map[string]any{"item_id": item["id"], "output_index": 0, "arguments": item["arguments"]})
	}
	item["status"] = "completed"
	event("response.output_item.done", map[string]any{"output_index": 0, "item": item})
	response["status"], response["output"] = "completed", []any{item}
	response["usage"] = map[string]any{"input_tokens": 23, "output_tokens": 17, "total_tokens": 40, "input_tokens_details": map[string]any{"cached_tokens": 0}, "output_tokens_details": map[string]any{"reasoning_tokens": 0}}
	event("response.completed", map[string]any{"response": response})
	return out.Bytes()
}
