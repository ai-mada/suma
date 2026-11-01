package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cl "github.com/ai-mada/suma/internal/claudecontract"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

// These files exercise configuration identity only. They are not runnable
// executables, official packages, account evidence, or model calls.
func claudeSDKConfigFixture192(t *testing.T) CommandSpec {
	t.Helper()
	root := t.TempDir()
	runtimeRoot := filepath.Join(root, "runtime")
	if err := os.Mkdir(runtimeRoot, 0700); err != nil {
		t.Fatal(err)
	}
	node, native := filepath.Join(root, "node.exe"), filepath.Join(root, "claude.exe")
	for _, file := range []string{node, native} {
		if err := os.WriteFile(file, []byte("owned identity fixture "+filepath.Base(file)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	entry := filepath.Join(runtimeRoot, "suma-claude.bundle.mjs")
	if err := os.WriteFile(entry, []byte("// owned bundled fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	refreshClaudeSDKManifest192(t, entry)
	spec, err := BuildClaudeSDKCommand(filepath.ToSlash(node), filepath.ToSlash(entry), ClaudeSDKSpec{NativeCLIExecutable: filepath.ToSlash(native), Model: "claude-sonnet-5", CredentialEnvRef: "OWNED_CLAUDE_KEY", ReviewedCLIVersion: tc.ClaudeSDKCLIVersion})
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func refreshClaudeSDKManifest192(t *testing.T, entry string) {
	t.Helper()
	data, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	m := claudeSDKManifest{SchemaVersion: 1, Protocol: cl.Protocol, SDKVersion: cl.SDKVersion, CLIVersion: cl.CLIVersion, Entry: filepath.Base(entry), Assets: []claudeSDKAsset{{Path: filepath.Base(entry), SHA256: digest(data), SizeBytes: int64(len(data))}}}
	raw, _ := json.Marshal(m)
	if err = os.WriteFile(filepath.Join(filepath.Dir(entry), "suma-claude-runtime.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func claudeSDKOwnedExecution192(t *testing.T) (Assignment, tc.Profile, pc.Request, pc.Permission) {
	t.Helper()
	spec := claudeSDKConfigFixture192(t)
	cap, err := localClaudeSDKCapability(spec)
	if err != nil {
		t.Fatal(err)
	}
	_, nodeSHA, err := commandExecutable(spec)
	if err != nil {
		t.Fatal(err)
	}
	profileSHA, err := commandProfileDigest(spec, nodeSHA)
	if err != nil {
		t.Fatal(err)
	}
	p := tc.Profile{CommandID: ClaudeSDKCommandID, Adapter: "claude", Protocol: cl.Protocol, ToolVersion: cl.CLIVersion, ExecutableSHA256: nodeSHA, ProfileSHA256: profileSHA, UserInputProtocol: cl.InputProtocol, ClaudeSDKExecution: cap}
	a := Assignment{RunID: "run-sdk192", NodeID: "node-sdk192", Attempt: 1, Fence: 4, DispatchID: "dispatch-sdk192", WorkerID: "worker-sdk192", Type: "agent", Command: []string{p.CommandID}, Config: map[string]any{"claude_sdk_execution": cap, "tool_profile_sha256": p.ProfileSHA256, "goal": "owned SDK operation"}}
	raw := `{"command":"git status --short"}`
	source := cl.PermissionSource{Basis: cl.Basis{SessionID: "session-sdk192", ToolUseID: "tool-permission192", RequestID: "native-request192", CallbackID: "callback-permission192", ConfigurationSHA256: cap.ConfigurationSHA256, ParamsSHA256: cl.PayloadDigest(raw)}, ToolName: "Bash", InputJSON: raw}
	r := pc.Request{IdempotencyKey: "request-sdk192-original", Protocol: cl.Protocol, Claude: &source}
	decision := pc.Decision{Choice: pc.Choice{Protocol: cl.Protocol, Claude: &cl.PermissionChoice{Outcome: "allow_once"}}, ActorID: "human-sdk192", DecidedAt: "2026-11-01T00:00:01Z"}
	v := pc.Permission{ID: "permission-sdk192", ProjectID: "project-sdk192", RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, DispatchID: a.DispatchID, WorkerID: a.WorkerID, ProfileSHA256: p.ProfileSHA256, Version: 2, State: "decided", Request: r, Decision: &decision, CreatedAt: "2026-11-01T00:00:00Z", ExpiresAt: "2026-11-01T00:10:00Z", AllowedActions: []string{}}
	if tc.ValidateProfile(p, true) != nil || pc.ValidateRequest(r) != nil || !pc.DecisionMatchesRequest(r, decision) {
		t.Fatal("invalid owned SDK fixture")
	}
	return a, p, r, v
}

func TestClaudeSDKConfigurationClosesAssetsAndCredentialScope(t *testing.T) {
	t.Run("closure_and_source_changes", func(t *testing.T) {
		spec := claudeSDKConfigFixture192(t)
		original, err := ClaudeSDKConfigurationDigest(spec)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(spec.ClaudeSDK.NativeCLIExecutable, []byte("replaced native executable"), 0700); err != nil {
			t.Fatal(err)
		}
		changed, err := ClaudeSDKConfigurationDigest(spec)
		if err != nil || changed == original {
			t.Fatal("native replacement retained configuration", err)
		}
		if err = os.WriteFile(spec.Args[0], []byte("changed unreviewed bundle"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = ClaudeSDKConfigurationDigest(spec); err == nil {
			t.Fatal("changed unpinned asset accepted")
		}
		refreshClaudeSDKManifest192(t, spec.Args[0])
		updated, err := ClaudeSDKConfigurationDigest(spec)
		if err != nil || updated == changed {
			t.Fatal("new reviewed bundle retained old identity", err)
		}
		if err = os.WriteFile(filepath.Join(filepath.Dir(spec.Args[0]), "shadow.mjs"), []byte("unlisted"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = ClaudeSDKConfigurationDigest(spec); err == nil {
			t.Fatal("unlisted loadable asset accepted")
		}
	})
	t.Run("fixed_host_and_no_inherited_credentials", func(t *testing.T) {
		spec := claudeSDKConfigFixture192(t)
		for _, mutate := range []func(*CommandSpec){func(s *CommandSpec) { s.Args = append(s.Args, "--dangerously-skip-permissions") }, func(s *CommandSpec) { s.EnvFrom = []string{"ANTHROPIC_AUTH_TOKEN"} }, func(s *CommandSpec) { s.Input = "goal" }, func(s *CommandSpec) { c := *s.ClaudeSDK; c.CredentialEnvRef = "SUMA_RUNNER_TOKEN"; s.ClaudeSDK = &c }, func(s *CommandSpec) { c := *s.ClaudeSDK; c.CredentialEnvRef = "CLAUDE_API_KEY"; s.ClaudeSDK = &c }, func(s *CommandSpec) { c := *s.ClaudeSDK; c.CredentialEnvRef = "ANTHROPIC_AUTH_TOKEN"; s.ClaudeSDK = &c }, func(s *CommandSpec) { c := *s.ClaudeSDK; c.CredentialEnvRef = "_CLAUDE_KEY"; s.ClaudeSDK = &c }} {
			copy := spec
			mutate(&copy)
			if validateClaudeSDKSpec(copy) == nil {
				t.Fatal("unsafe alternate host accepted")
			}
		}
		t.Setenv("OWNED_CLAUDE_KEY", "owned-local-api-key")
		t.Setenv("SUMA_RUNNER_TOKEN", "must-not-inherit")
		t.Setenv("ANTHROPIC_AUTH_TOKEN", "must-not-inherit")
		t.Setenv("NODE_OPTIONS", "must-not-inherit")
		root := t.TempDir()
		source, home := filepath.Join(root, "source"), filepath.Join(root, "home")
		if err := os.Mkdir(source, 0700); err != nil {
			t.Fatal(err)
		}
		env, secrets, err := claudeSDKEnvironment(spec, home, source, true)
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(env, "\n")
		if strings.Contains(joined, "must-not-inherit") || !strings.Contains(joined, "OWNED_CLAUDE_KEY=owned-local-api-key") || len(secrets) != 1 {
			t.Fatal("credential scope widened")
		}
		if !strings.Contains(joined, "SUMA_CLAUDE_SDK_CONFIG=") || !strings.Contains(joined, "HOME="+home) {
			t.Fatal("private configuration missing")
		}
	})
}

func TestClaudeSDKJournalRetainsUnknownIntentAndNativeIdentity(t *testing.T) {
	a, p, r, permission := claudeSDKOwnedExecution192(t)
	dir := filepath.Join(t.TempDir(), "journal")
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	if err = j.beginToolPermissionExecution(a, p); err != nil {
		t.Fatal(err)
	}
	if err = j.beginToolInputExecution(a, p); err != nil {
		t.Fatal(err)
	}
	if err = j.recordToolPermission(a, r); err != nil {
		t.Fatal(err)
	}
	if err = j.observeToolPermission(a, permission); err != nil {
		t.Fatal(err)
	}
	sha, _ := cl.PermissionDecisionDigest(*r.Claude, *permission.Decision.Claude)
	o := delivery.Observation{Sequence: 1, ParamsSHA256: r.Claude.ParamsSHA256, AnswerSHA256: sha, AnswerObserved: true}
	if err = j.recordClaudeToolPermissionObservation(a, r, permission.ID, o, false); err != nil {
		t.Fatal(err)
	}
	o.Sequence++
	o.ReplyStarted = true
	if err = j.recordClaudeToolPermissionObservation(a, r, permission.ID, o, true); err != nil {
		t.Fatal(err)
	}
	if j.recordClaudeToolPermissionObservation(a, r, permission.ID, o, true) == nil {
		t.Fatal("same native response was authorized twice")
	}
	bad := o
	bad.Sequence++
	bad.RequestResolved = true
	if j.recordClaudeToolPermissionObservation(a, r, permission.ID, bad, false) == nil {
		t.Fatal("invented native ACK accepted")
	}
	inputRaw := `{"questions":[{"question":"Which scope?","header":"Scope","options":[{"label":"CLI","description":"Local execution"},{"label":"HTTP","description":"Control API"}],"multiSelect":true}]}`
	basis := r.Claude.Basis
	basis.CallbackID = "callback-question192"
	basis.ParamsSHA256 = cl.PayloadDigest(inputRaw)
	question, err := cl.NewInputRequest(basis, inputRaw)
	if err != nil {
		t.Fatal(err)
	}
	if j.recordToolInput(a, ic.ToolInputRequest{IdempotencyKey: "request-question192-collision", Protocol: cl.Protocol, Claude: &question}) == nil {
		t.Fatal("same native request moved to another domain")
	}
	basis.RequestID = "native-question192"
	basis.SessionID = "other-session192"
	question, err = cl.NewInputRequest(basis, inputRaw)
	if err != nil {
		t.Fatal(err)
	}
	if j.recordToolInput(a, ic.ToolInputRequest{IdempotencyKey: "request-question192-session", Protocol: cl.Protocol, Claude: &question}) == nil {
		t.Fatal("one attempt moved to another SDK session")
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.Pending()) != 1 || !reopened.Pending()[0].OutcomeUnknown {
		t.Fatal("restart fabricated a completed native operation")
	}
	cp, err := reopened.toolPermission(a, r)
	if err != nil || cp.ClaudeObservation == nil || !cp.ClaudeObservation.ReplyStarted || cp.ClaudeObservation.ReplyWritten || cp.ClaudeObservation.RequestResolved {
		t.Fatal("original unknown intent changed", err)
	}
	if reopened.recordClaudeToolPermissionObservation(a, r, permission.ID, o, true) == nil {
		t.Fatal("restart reconstructed a native write permit")
	}
}

type claudeSDKFrameWriter192 struct{ frames chan map[string]any }

func (w claudeSDKFrameWriter192) Write(raw []byte) (int, error) {
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, err
	}
	w.frames <- value
	return len(raw), nil
}

func TestClaudeSDKDriverQuestionsPermissionsAndTerminalRemainDistinct(t *testing.T) {
	t.Run("large_source_reply", func(t *testing.T) {
		for _, html := range []bool{false, true} {
			a, p, original, _ := claudeSDKOwnedExecution192(t)
			j, err := OpenJournal(filepath.Join(t.TempDir(), "journal"))
			if err != nil {
				t.Fatal(err)
			}
			defer j.Close()
			if err = j.Accept(a); err != nil {
				t.Fatal(err)
			}
			d, err := newClaudeSDKDriver(context.Background(), a, ExecConfig{Journal: j, ToolInputClient: &Client{}, ToolPermission: func(context.Context, pc.Request) (pc.Decision, error) { return pc.Decision{}, nil }, ToolPermissionPoll: func(context.Context, string) (pc.Permission, error) { return pc.Permission{}, nil }}, p, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			questions := []any{}
			for i := 0; i < 2; i++ {
				options := []any{}
				for o := 0; o < 4; o++ {
					label, description := fmt.Sprintf("Option%d", o)+strings.Repeat("<", 90), strings.Repeat("\\", 3500)
					if html {
						label = fmt.Sprintf("Option%d", o) + strings.Repeat("<", 490)
						description = strings.Repeat("<", 4000)
					}
					options = append(options, map[string]any{"label": label, "description": description})
				}
				questions = append(questions, map[string]any{"question": fmt.Sprintf("Scope%d", i) + strings.Repeat("<", 3940), "header": "Scope", "options": options, "multiSelect": true})
			}
			var input bytes.Buffer
			encoder := json.NewEncoder(&input)
			encoder.SetEscapeHTML(false)
			if err = encoder.Encode(map[string]any{"questions": questions}); err != nil {
				t.Fatal(err)
			}
			raw := strings.TrimSuffix(input.String(), "\n")
			if len(raw) > cl.MaxPayloadBytes {
				t.Fatal("owned native input exceeds its declared bound", len(raw))
			}
			basis := original.Claude.Basis
			basis.ParamsSHA256 = cl.PayloadDigest(raw)
			source, err := cl.NewInputRequest(basis, raw)
			if err != nil {
				t.Fatal(err)
			}
			answer := cl.InputDecision{Outcome: "answered", Answers: []cl.InputSelection{{QuestionID: "q1", OptionIDs: []string{"o1"}}, {QuestionID: "q2", OptionIDs: []string{"o2"}}}}
			if err = cl.ValidateInputDecision(source, answer); err != nil {
				t.Fatal(err)
			}
			if err = d.queue.enqueueReply(map[string]any{"v": 1, "type": "input_reply", "request_id": d.requestID, "source": source, "answer": answer}, nil, nil); err != nil {
				t.Fatal("legal bounded question cannot reach the SDK host", err)
			}
			d.queue.close()
			var written bytes.Buffer
			n, err := d.CopyInput(&written)
			if err != nil {
				t.Fatal(err)
			}
			if !html && n <= 128<<10 {
				t.Fatal("fixture did not exercise the original queue limit", n)
			}
			lines := bytes.Split(bytes.TrimSpace(written.Bytes()), []byte("\n"))
			if len(lines) != 2 {
				t.Fatal("carrier did not write the original goal and exact reply")
			}
			var reply struct {
				Source cl.InputRequest `json:"source"`
			}
			if json.Unmarshal(lines[1], &reply) != nil || reply.Source.InputJSON != raw {
				t.Fatal("large native source was truncated or replaced")
			}
		}
	})
	a, p, permissionRequest, permission := claudeSDKOwnedExecution192(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	j, err := OpenJournal(filepath.Join(t.TempDir(), "journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	var input ic.ToolInput
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body struct {
			Request ic.ToolInputRequest `json:"request"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/tool-inputs") {
			decision := &cl.InputDecision{Outcome: "answered", Answers: []cl.InputSelection{{QuestionID: "q1", OptionIDs: []string{"o2", "o1"}}}}
			answer := &ic.ToolInputAnswer{Claude: decision, ActorID: "human-sdk192", AnsweredAt: "2026-11-01T00:00:01Z"}
			answer.AnswerSHA256, _ = ic.AnswerDigest(body.Request, nil, nil, decision)
			input = ic.ToolInput{ID: "input-sdk192", ProjectID: permission.ProjectID, RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, DispatchID: a.DispatchID, WorkerID: a.WorkerID, ProfileSHA256: p.ProfileSHA256, Version: 2, State: "answered", Request: body.Request, Answer: answer, CreatedAt: "2026-11-01T00:00:00Z", ExpiresAt: "2026-11-01T00:10:00Z", AllowedActions: []string{}}
		}
		if err := json.NewEncoder(w).Encode(input); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, ProjectID: permission.ProjectID, Token: "owned-http-credential"}
	var progress []tc.Event
	cfg := ExecConfig{Journal: j, ToolInputClient: client, ToolInputProfile: p.ProfileSHA256, ToolProgress: func(event tc.Event) error { progress = append(progress, event); return nil }, ToolPermission: func(ctx context.Context, r pc.Request) (pc.Decision, error) {
		permission.Request = r
		if err := j.observeToolPermission(a, permission); err != nil {
			return pc.Decision{}, err
		}
		return *permission.Decision, nil
	}, ToolPermissionPoll: func(context.Context, string) (pc.Permission, error) { return permission, nil }}
	d, err := newClaudeSDKDriver(ctx, a, cfg, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	frames := make(chan map[string]any, 8)
	done := make(chan struct{})
	go func() { defer close(done); _, _ = d.CopyInput(claudeSDKFrameWriter192{frames}) }()
	next := func() map[string]any {
		select {
		case frame := <-frames:
			return frame
		case <-ctx.Done():
			t.Fatal("SDK reply timed out")
			return nil
		}
	}
	if next()["type"] != "execute" {
		t.Fatal("missing original goal")
	}
	write := func(frame map[string]any) {
		frame["v"] = 1
		frame["request_id"] = d.requestID
		raw, _ := json.Marshal(frame)
		if _, err := d.Write(append(raw, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	write(map[string]any{"type": "initialized", "session_id": permissionRequest.Claude.SessionID, "model": p.ClaudeSDKExecution.Model, "configuration_sha256": p.ClaudeSDKExecution.ConfigurationSHA256, "sdk_version": cl.SDKVersion, "cli_version": cl.CLIVersion})
	raw := `{"questions":[{"question":"Which scope?","header":"Scope","options":[{"label":"CLI","description":"Local execution"},{"label":"HTTP","description":"Control API"}],"multiSelect":true}]}`
	basis := permissionRequest.Claude.Basis
	basis.RequestID = "native-question192"
	basis.CallbackID = "callback-question192"
	basis.ToolUseID = "tool-question192"
	basis.ParamsSHA256 = cl.PayloadDigest(raw)
	question, err := cl.NewInputRequest(basis, raw)
	if err != nil {
		t.Fatal(err)
	}
	write(map[string]any{"type": "activity_started", "session_id": question.SessionID, "tool_use_id": question.ToolUseID, "tool_name": "AskUserQuestion"})
	write(map[string]any{"type": "input_request", "source": question})
	reply := next()
	if reply["type"] != "input_reply" || reply["request_id"] != d.requestID {
		t.Fatal("question answered on wrong carrier")
	}
	// These are the SDK host's actual emitted fields. Tool results contain the
	// original tool-use ID and completed/error status, not another tool name.
	write(map[string]any{"type": "activity_completed", "session_id": question.SessionID, "tool_use_id": question.ToolUseID, "status": "completed"})
	write(map[string]any{"type": "activity_started", "session_id": permissionRequest.Claude.SessionID, "tool_use_id": permissionRequest.Claude.ToolUseID, "tool_name": "Bash"})
	write(map[string]any{"type": "permission_request", "source": permissionRequest.Claude})
	reply = next()
	if reply["type"] != "permission_reply" || reply["decision"].(map[string]any)["outcome"] != "allow_once" {
		t.Fatal("one-call permission was replaced")
	}
	write(map[string]any{"type": "activity_completed", "session_id": permissionRequest.Claude.SessionID, "tool_use_id": permissionRequest.Claude.ToolUseID, "status": "completed"})
	write(map[string]any{"type": "activity_started", "session_id": question.SessionID, "tool_use_id": "read-error192", "tool_name": "Read"})
	write(map[string]any{"type": "activity_completed", "session_id": question.SessionID, "tool_use_id": "read-error192", "status": "error"})
	write(map[string]any{"type": "completed", "session_id": permissionRequest.Claude.SessionID, "evidence": map[string]any{"result_subtype": "success", "result_is_error": false, "turn_ended": true, "tools_completed": true, "callback_count": 2, "returned_count": 2, "pending_callbacks": 0, "callback_denial_count": 0, "native_permission_denial_count": 0}})
	<-done
	proof := d.finish(true, true)
	if proof.Terminal != "completed" || proof.ClaudeSDKExecution == nil || len(proof.ClaudeSDKExecution.Permissions) != 1 || len(proof.ClaudeSDKExecution.UserInputs) != 1 {
		t.Fatalf("foreground proof missing: %+v bad=%v", proof, d.bad)
	}
	for _, o := range []*delivery.Observation{proof.ClaudeSDKExecution.Permissions[0].Observation, proof.ClaudeSDKExecution.UserInputs[0].Observation} {
		if o == nil || !o.AnswerObserved || !o.ReplyWritten || !o.ExecutionClosed || o.RequestResolved || o.FailureCode != "" {
			t.Fatalf("write invented a vendor ACK or lost closure: %+v", o)
		}
	}
	if err := tc.ValidateEvidence(*proof); err != nil {
		t.Fatal(err)
	}
	foundError := false
	for _, event := range progress {
		if event.Kind == "activity_completed" && event.ItemID == "read-error192" && event.Status == "failed" {
			foundError = true
		}
	}
	if !foundError {
		t.Fatal("native error tool result was hidden or promoted to success")
	}
	for index, variant := range []struct{ id, status string }{{"unpaired-tool192", "completed"}, {"paired-tool192", "failed"}, {"paired-tool192", "unknown"}} {
		rejectedAssignment := a
		rejectedAssignment.RunID = fmt.Sprintf("reject-activity192-%d", index)
		if err := j.Accept(rejectedAssignment); err != nil {
			t.Fatal(err)
		}
		rejected, err := newClaudeSDKDriver(ctx, rejectedAssignment, cfg, p, nil)
		if err != nil {
			t.Fatal(err)
		}
		writeRejected := func(frame map[string]any) error {
			frame["v"], frame["request_id"] = 1, rejected.requestID
			raw, _ := json.Marshal(frame)
			_, err := rejected.Write(append(raw, '\n'))
			return err
		}
		if err := writeRejected(map[string]any{"type": "initialized", "session_id": permissionRequest.Claude.SessionID, "model": p.ClaudeSDKExecution.Model, "configuration_sha256": p.ClaudeSDKExecution.ConfigurationSHA256, "sdk_version": cl.SDKVersion, "cli_version": cl.CLIVersion}); err != nil {
			t.Fatal(err)
		}
		if err := writeRejected(map[string]any{"type": "activity_started", "session_id": permissionRequest.Claude.SessionID, "tool_use_id": "paired-tool192", "tool_name": "Read"}); err != nil {
			t.Fatal(err)
		}
		if err := writeRejected(map[string]any{"type": "activity_completed", "session_id": permissionRequest.Claude.SessionID, "tool_use_id": variant.id, "status": variant.status}); err == nil {
			t.Fatal("unpaired or unsupported SDK tool result accepted", variant)
		}
		_ = rejected.Close()
	}
	if _, err := d.Write([]byte("{}\n")); err == nil && d.bad == nil {
		t.Fatal("post-terminal frame accepted")
	}
}
