package runner

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
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
	sc "github.com/ai-mada/suma/internal/steeringcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// One opt-in installed-CLI journey. Both HTTP authorities are owned fixtures;
// this does not exercise browser/SQLite authority or a commercial model.
func TestCodexSteeringActualInstalledCLIWithOwnedResponses(t *testing.T) {
	if os.Getenv("SUMA_TURN_STEERING_ACTUAL") != "1" {
		t.Skip("explicit isolated installed CLI check required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(root, ".cache", "turn-steering-188", "actual")
	dir := filepath.Join(stage, "run-"+time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name string, value any) {
		raw, _ := json.MarshalIndent(value, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, name), append(raw, '\n'), 0600); err != nil {
			t.Error(err)
		}
	}
	state := map[string]any{"stage": "T-TURN-STEERING-188", "status": "preparing", "commercial_calls": 0, "user_credentials": false, "existing_home_used": false, "version_features_schema_repeated": false, "browser_sqlite_repeated": false, "code_quality_chain_repeated": false}
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
	raw, err := os.ReadFile(filepath.Join(root, ".cache", "tool-inputs-187", "protocol", "summary.json"))
	if err != nil || json.Unmarshal(raw, &pin) != nil {
		t.Fatal("reuse original offline pin", err)
	}
	executable := os.Getenv("SUMA_CODEX_EXECUTABLE")
	if !filepath.IsAbs(executable) {
		t.Fatal("SUMA_CODEX_EXECUTABLE must be an explicit absolute path")
	}
	binary, err := os.ReadFile(executable)
	if err != nil || digest(binary) != pin.SHA {
		t.Fatal("installed CLI differs from offline pinned source", err)
	}
	binary = nil
	state["executable_sha256"], state["version"] = pin.SHA, pin.Version
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	// The original saved human message exists before a session or instruction.
	const messageBody = "Preserve the original scope and acknowledge STEERING_188_OWNED_MESSAGE."
	message := sc.SteeringMessage{ID: "saved-human-message188", Sequence: 1, AuthorID: "owned-human188", Body: messageBody, BodySHA256: sc.MessageDigest(messageBody)}
	write("saved-discussion-message.local.json", message)
	held, releaseFirst := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseFirst) })
	var modelMu sync.Mutex
	model := ""
	modelCalls := 0
	firstCancelled := false
	continuedWithMessage := false
	providerFailure := ""
	var modelRequestSHA, modelResponseSHA []string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "" {
			modelMu.Lock()
			providerFailure = "unexpected_provider_route_or_credential"
			modelMu.Unlock()
			http.Error(w, "owned route", 400)
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
		body, e := io.ReadAll(io.LimitReader(reader, 4<<20))
		var request map[string]any
		modelMu.Lock()
		selected := model
		modelMu.Unlock()
		if e != nil || len(body) >= 4<<20 || json.Unmarshal(body, &request) != nil || request["model"] != selected || request["stream"] != true {
			http.Error(w, "owned model selection", 400)
			return
		}
		modelMu.Lock()
		modelCalls++
		call := modelCalls
		modelRequestSHA = append(modelRequestSHA, digest(body))
		modelMu.Unlock()
		_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("model-request-%d.local.json", call)), body, 0600)
		if call > 2 {
			modelMu.Lock()
			providerFailure = "unexpected_model_replay"
			modelMu.Unlock()
			http.Error(w, "unexpected model replay", 400)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		if call == 1 {
			// Keep the first real request active without holding a fixture lock.
			first := map[string]any{"id": "resp_owned187_1", "object": "response", "created_at": 1, "status": "in_progress", "model": selected, "output": []any{}}
			payload, _ := json.Marshal(map[string]any{"type": "response.created", "sequence_number": 0, "response": first})
			_, _ = fmt.Fprintf(w, "event: response.created\ndata: %s\n\n", payload)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			close(held)
			select {
			case <-releaseFirst:
			case <-r.Context().Done():
				modelMu.Lock()
				firstCancelled = true
				modelMu.Unlock()
				return
			case <-ctx.Done():
				return
			}
		} else {
			if !bytes.Contains(body, []byte(messageBody)) {
				modelMu.Lock()
				providerFailure = "original_steering_message_absent"
				modelMu.Unlock()
				http.Error(w, "missing original message", 400)
				return
			}
			modelMu.Lock()
			continuedWithMessage = true
			modelMu.Unlock()
		}
		text := "Initial owned response; no tools or code effects."
		if call == 2 {
			text = "Received the exact supplemental human message in the original turn. No code or acceptance is claimed."
		}
		item := map[string]any{"type": "message", "id": fmt.Sprintf("msg_owned188_%d", call), "status": "completed", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": text, "annotations": []any{}}}}
		response := actualInputSSE187(call, selected, item)
		if call == 1 {
			if end := bytes.Index(response, []byte("\n\n")); end >= 0 {
				response = response[end+2:]
			}
		}
		modelMu.Lock()
		modelResponseSHA = append(modelResponseSHA, digest(response))
		modelMu.Unlock()
		_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("model-response-%d.local.sse", call)), response, 0600)
		_, _ = w.Write(response)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer func() { cancel(); releaseOnce.Do(func() { close(releaseFirst) }); provider.Close() }()
	workspace := filepath.Join(dir, "workspace")
	if err = os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	spec := CommandSpec{Adapter: "codex", Protocol: cc.Protocol, Executable: executable, Args: []string{"app-server", "--listen", "stdio://"}, Input: "codex-app-server", CodexAppServer: &CodexAppServerSpec{ProviderID: "owned188", BaseURL: provider.URL + "/v1", WindowsSandbox: "unelevated"}}
	state["failing_stage"] = "one_current_config_catalog_handshake"
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
		t.Fatal("native catalog has no explicit reasoning model")
	}
	binding, err := cc.NewBinding(*catalog, choice)
	if err != nil {
		t.Fatal(err)
	}
	modelMu.Lock()
	model = choice.Model
	modelMu.Unlock()
	profileSHA, err := commandProfileDigest(spec, pin.SHA)
	if err != nil {
		t.Fatal(err)
	}
	profile := tc.Profile{CommandID: "codex-steering-actual188", Adapter: "codex", Protocol: cc.Protocol, ProfileSHA256: profileSHA, ExecutableSHA256: pin.SHA, ToolVersion: pin.Version}
	var bindingWire map[string]any
	bindingRaw, _ := json.Marshal(binding)
	_ = json.Unmarshal(bindingRaw, &bindingWire)
	a := Assignment{TenantID: "tenant188", ProjectID: "project188", RunID: "run188", NodeID: "code188", Attempt: 1, Fence: 1, WorkerID: "worker188", DispatchID: "dispatch188", Type: "agent", Command: []string{profile.CommandID}, Config: map[string]any{"goal": "Respond to the owned model fixture; do not run tools or edit files. The current turn may receive an explicit supplemental human message.", "tool_profile_sha256": profileSHA, "codex_execution": bindingWire}}
	j, err := OpenJournal(filepath.Join(dir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	var controlMu sync.Mutex
	var session sc.SteeringSession
	var instruction sc.SteeringInstruction
	var order []string
	counts := map[string]int{}
	receipts := map[string]json.RawMessage{}
	requestSHA := map[string]string{}
	resultReceived := false
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		controlMu.Lock()
		defer controlMu.Unlock()
		body, e := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var q struct {
			ProjectID     string                      `json:"project_id"`
			Assignment    Assignment                  `json:"assignment"`
			Source        cc.CodexSteeringSource      `json:"source"`
			SessionID     string                      `json:"session_id"`
			InstructionID string                      `json:"instruction_id"`
			Key           string                      `json:"idempotency_key"`
			Observation   cc.CodexSteeringObservation `json:"observation"`
			Result        json.RawMessage             `json:"result"`
		}
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer owned-fixture-188" || e != nil || json.Unmarshal(body, &q) != nil || q.ProjectID != a.ProjectID {
			http.Error(w, "scope", 400)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/runner/")
		counts[path]++
		order = append(order, path)
		if path == "results" {
			var result Result
			if json.Unmarshal(q.Result, &result) != nil || session.State != "closed" || instruction.Observation == nil || !instruction.Observation.NativeAccepted || !instruction.Observation.WriteWritten || !instruction.Observation.TurnClosed || result.ToolEvidence == nil || result.ToolEvidence.CodexExecution == nil || result.ToolEvidence.CodexExecution.Steering == nil || !result.ToolEvidence.CodexExecution.Steering.Closed {
				http.Error(w, "result before closed original receipts", 409)
				return
			}
			resultReceived = true
			_ = json.NewEncoder(w).Encode(map[string]bool{"acknowledged": true})
			return
		}
		if !samePermissionValue(q.Assignment, a) {
			http.Error(w, "assignment", 409)
			return
		}
		if q.Key != "" {
			if old, ok := receipts[q.Key]; ok {
				if requestSHA[q.Key] != digest(body) {
					http.Error(w, "changed operation", 409)
					return
				}
				_, _ = w.Write(old)
				return
			}
		}
		var out any
		switch path {
		case "steering/sessions":
			if cc.ValidateSteeringSource(q.Source) != nil || q.Source.BindingSHA256 != binding.BindingSHA256 || session.ID != "" {
				http.Error(w, "source", 409)
				return
			}
			now := time.Now().UTC()
			session = sc.SteeringSession{ID: "actual-session188", ProjectID: a.ProjectID, TaskID: "owned-task188", RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, DispatchID: a.DispatchID, ProfileSHA256: profileSHA, Source: q.Source, Version: 1, State: "active", OpenedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(10 * time.Minute).Format(time.RFC3339Nano), AllowedActions: []string{}}
			out = sc.SteeringOpenResult{Session: &session}
		case "steering/poll":
			if session.ID == "" || q.SessionID != session.ID {
				http.Error(w, "session", 409)
				return
			}
			if session.State == "active" && instruction.ID == "" {
				select {
				case <-held:
					now := time.Now().UTC()
					instruction = sc.SteeringInstruction{ID: "actual-instruction188", SessionID: session.ID, ProjectID: a.ProjectID, TaskID: session.TaskID, RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, Version: 1, Message: message, ActorID: message.AuthorID, CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: session.ExpiresAt}
				default:
				}
			}
			items := []sc.SteeringInstruction{}
			if instruction.ID != "" {
				items = append(items, instruction)
			}
			out = sc.SteeringPoll{Session: session, Instructions: items}
		case "steering/observations":
			sha, e := cc.SteeringParamsDigest(session.Source, instruction.ID, message.Body)
			if q.SessionID != session.ID || q.InstructionID != instruction.ID || cc.ValidateSteeringObservation(q.Observation) != nil || e != nil || sha != q.Observation.ParamsSHA256 || instruction.Observation != nil && !steeringObservationAdvances(*instruction.Observation, q.Observation) {
				http.Error(w, "observation", 409)
				return
			}
			instruction.Version++
			copy := q.Observation
			instruction.Observation = &copy
			out = instruction
			if q.Observation.NativeAccepted {
				releaseOnce.Do(func() { close(releaseFirst) })
			}
		case "steering/close":
			if q.SessionID != session.ID || instruction.Observation == nil || !instruction.Observation.TurnClosed || !instruction.Observation.NativeAccepted || !instruction.Observation.WriteWritten {
				http.Error(w, "close before original observations", 409)
				return
			}
			session.State = "closed"
			session.Version++
			out = session
		default:
			http.Error(w, "owned route", 404)
			return
		}
		encoded, _ := json.Marshal(out)
		if q.Key != "" {
			receipts[q.Key] = append(json.RawMessage(nil), encoded...)
			requestSHA[q.Key] = digest(body)
		}
		_, _ = w.Write(encoded)
	}))
	defer control.Close()
	client := &Client{BaseURL: control.URL, Token: "owned-fixture-188", ProjectID: a.ProjectID}
	home := filepath.Join(dir, "codex-home")
	if err = prepareCodexHome(spec, home, workspace); err != nil {
		t.Fatal(err)
	}
	stream := newToolStream(profile, nil, nil)
	driver := newCodexAppServer(profile, spec, home, workspace, a.Config["goal"].(string), &binding, stream)
	defer driver.Close()
	if err = driver.configureSteering(ctx, codexSteeringTransport(a, ExecConfig{Journal: j, SteeringClient: client}, profile)); err != nil {
		t.Fatal(err)
	}
	// Match the original Worker sender: original HTTP observations are drained
	// during the live turn as well as before its final Result.
	senderCtx, stopSender := context.WithCancel(ctx)
	senderDone := make(chan struct{})
	go func() {
		defer close(senderDone)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-senderCtx.Done():
				return
			case <-ticker.C:
				_ = ReplaySteering(senderCtx, client, j)
			}
		}
	}()
	defer func() { stopSender(); <-senderDone }()
	cmd := exec.Command(executable, spec.Args...)
	cmd.Dir = home
	cmd.Env, _, err = codexEnvironment(spec, home)
	if err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(dir, "tmp")
	if err = os.Mkdir(tmp, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TEMP", "TMP", "TMPDIR"} {
		cmd.Env = append(cmd.Env, name+"="+tmp)
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
	state["failing_stage"] = "actual_live_turn"
	ctl, err := startProcess(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer ctl.Close()
	state["native_process_pid"] = cmd.Process.Pid
	state["native_turn_processes"] = 1
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
	state["failing_stage"] = "original_close_and_observations_before_result"
	result := Result{Assignment: a, ExitCode: 0}
	// Use the production stream evidence, including its native terminal/usage.
	var streamError error
	result.ToolEvidence, streamError = stream.finish()
	result.OperationID = operationID()
	if forced || err != nil || finishErr != nil || driver.bad != nil || stream.terminal != "completed" || streamError != nil {
		t.Fatal("actual native transport failed; original evidence retained")
	}
	if err = j.Finish(result); err != nil {
		t.Fatal(err)
	}
	replayErr := Replay(ctx, client, j)
	state["replay_error"] = fmt.Sprint(replayErr)
	_ = stdout.Sync()
	_ = stdin.Sync()
	nativeIn, _ := os.ReadFile(filepath.Join(dir, "native-stdin.local.jsonl"))
	nativeOut, _ := os.ReadFile(filepath.Join(dir, "native-stdout.local.jsonl"))
	requestCounts := map[string]int{}
	var steerID any
	var steerParams map[string]any
	scanner := bufio.NewScanner(bytes.NewReader(nativeIn))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		frame, e := decodeToolObject(scanner.Bytes())
		if e == nil {
			method := textAt(frame, "method")
			if method != "" {
				requestCounts[method]++
			}
			if method == "turn/steer" {
				steerID = frame["id"]
				steerParams = objectAt(frame, "params")
			}
		}
	}
	ackCount := 0
	scanner = bufio.NewScanner(bytes.NewReader(nativeOut))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		frame, e := decodeToolObject(scanner.Bytes())
		if e == nil && steerID != nil && samePermissionValue(frame["id"], steerID) && textAt(objectAt(frame, "result"), "turnId") == driver.turn {
			ackCount++
		}
	}
	state["native_host_requests"], state["same_turn_ack_count"] = requestCounts, ackCount
	controlMu.Lock()
	write("control-receipt.local.json", map[string]any{"session": session, "instruction": instruction, "calls": counts, "order": order, "receipt_keys": requestSHA, "result_received": resultReceived})
	resultAck := resultReceived
	controlMu.Unlock()
	modelMu.Lock()
	write("model-receipt.local.json", map[string]any{"calls": modelCalls, "first_request_cancelled": firstCancelled, "continued_with_original_message": continuedWithMessage, "request_sha256": modelRequestSHA, "response_sha256": modelResponseSHA, "failure": providerFailure})
	calls, continued, failure := modelCalls, continuedWithMessage, providerFailure
	modelMu.Unlock()
	proof := driver.evidence.Steering
	if replayErr != nil || !resultAck || proof == nil || !proof.Closed || len(proof.Instructions) != 1 || proof.Instructions[0].Observation == nil || !proof.Instructions[0].Observation.WriteWritten || !proof.Instructions[0].Observation.NativeAccepted || !proof.Instructions[0].Observation.TurnClosed || proof.Instructions[0].Observation.FailureCode != "" || requestCounts["thread/start"] != 1 || requestCounts["turn/start"] != 1 || requestCounts["turn/steer"] != 1 || ackCount != 1 || textAt(steerParams, "expectedTurnId") != driver.turn || textAt(steerParams, "threadId") != driver.thread || calls != 2 || !continued || failure != "" {
		t.Fatal("actual same-turn steering facts did not close exactly; evidence retained")
	}
	state["status"], state["failing_stage"] = "passed_actual_cli_same_turn_owned_message", ""
}
