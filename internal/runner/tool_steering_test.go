package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	sc "github.com/ai-mada/suma/internal/steeringcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func steeringFixture188(t *testing.T) (Assignment, tc.Profile, cc.CodexSteeringSource, sc.SteeringSession, sc.SteeringInstruction) {
	t.Helper()
	a, p, request, permission := codexPermissionFixture174(t)
	source := cc.CodexSteeringSource{Protocol: cc.SteeringProtocol, ThreadID: request.Codex.ThreadID, TurnID: request.Codex.TurnID, BindingSHA256: request.Codex.BindingSHA256}
	session := sc.SteeringSession{ID: "session188", ProjectID: permission.ProjectID, TaskID: "task188", RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, DispatchID: a.DispatchID, ProfileSHA256: p.ProfileSHA256, Source: source, Version: 1, State: "active", OpenedAt: "2026-11-01T00:00:00Z", ExpiresAt: "2099-11-01T00:15:00Z", AllowedActions: []string{"send"}}
	body := "Keep the existing interface.\n保留原始文本与换行。"
	instruction := sc.SteeringInstruction{ID: "instruction188", SessionID: session.ID, ProjectID: session.ProjectID, TaskID: session.TaskID, RunID: session.RunID, NodeID: session.NodeID, Attempt: session.Attempt, Fence: session.Fence, Version: 1, Message: sc.SteeringMessage{ID: "message188", Sequence: 1, AuthorID: "human188", Body: body, BodySHA256: sc.MessageDigest(body)}, ActorID: "human188", CreatedAt: "2026-11-01T00:00:01Z", ExpiresAt: session.ExpiresAt}
	if sc.ValidateSession(session) != nil || sc.ValidateInstruction(instruction) != nil {
		t.Fatal("invalid owned steering fixture")
	}
	return a, p, source, session, instruction
}

func savedSteering188(t *testing.T, a Assignment, p tc.Profile, source cc.CodexSteeringSource, session *sc.SteeringSession) (*Journal, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "journal")
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	if err = j.prepareSteering(a, p, source); err != nil {
		t.Fatal(err)
	}
	if session != nil {
		if err = j.bindSteeringSession(a, *session); err != nil {
			t.Fatal(err)
		}
	}
	return j, dir
}

func steeringObservation188(t *testing.T, source cc.CodexSteeringSource, instruction sc.SteeringInstruction, requestID string) cc.CodexSteeringObservation {
	t.Helper()
	sha, err := cc.SteeringParamsDigest(source, instruction.ID, instruction.Message.Body)
	if err != nil {
		t.Fatal(err)
	}
	return cc.CodexSteeringObservation{Sequence: 1, RequestID: cc.CodexRPCRequestID{Kind: "number", Value: requestID}, ParamsSHA256: sha, Observed: true}
}

func TestSteeringJournalBindsSourceAndPreservesCompleteMessages(t *testing.T) {
	t.Run("exact_source_session_instruction", func(t *testing.T) {
		a, p, source, session, instruction := steeringFixture188(t)
		j, _ := savedSteering188(t, a, p, source, &session)
		changedSource := source
		changedSource.TurnID = "another-turn188"
		if j.prepareSteering(a, p, changedSource) == nil {
			t.Fatal("original attempt was rebound to another native turn")
		}
		changedSession := session
		changedSession.Source = changedSource
		if j.bindSteeringSession(a, changedSession) == nil {
			t.Fatal("session accepted another native source")
		}
		wrong := instruction
		wrong.TaskID = "another-task188"
		if j.recordSteeringInstructions(a, sc.SteeringPoll{Session: session, Instructions: []sc.SteeringInstruction{wrong}}, nil) == nil {
			t.Fatal("another task's instruction entered the original session")
		}
		if err := j.recordSteeringInstructions(a, sc.SteeringPoll{Session: session, Instructions: []sc.SteeringInstruction{instruction}}, nil); err != nil {
			t.Fatal(err)
		}
		changed := instruction
		changed.Message.Body = "Replace the old interface."
		changed.Message.BodySHA256 = sc.MessageDigest(changed.Message.Body)
		if j.recordSteeringInstructions(a, sc.SteeringPoll{Session: session, Instructions: []sc.SteeringInstruction{changed}}, nil) == nil {
			t.Fatal("existing human message was rewritten")
		}
		otherFence := a
		otherFence.Fence++
		if _, err := j.steeringCheckpoint(otherFence); err == nil {
			t.Fatal("another fence read the private steering checkpoint")
		}
		cp, err := j.steeringCheckpoint(a)
		if err != nil || cp.Source != source || !sameSteeringInstruction(cp.Instructions[instruction.ID].Instruction, instruction) {
			t.Fatal("rejected source changed original durable content", err)
		}
	})
	t.Run("maximum_32000_bytes_8000_runes", func(t *testing.T) {
		a, p, source, session, instruction := steeringFixture188(t)
		j, _ := savedSteering188(t, a, p, source, &session)
		instruction.Message.Body = strings.Repeat("🚀", 8000)
		instruction.Message.BodySHA256 = sc.MessageDigest(instruction.Message.Body)
		if len(instruction.Message.Body) != 32000 || utf8.RuneCountInString(instruction.Message.Body) != 8000 {
			t.Fatal("maximum content fixture is not the declared boundary")
		}
		if err := j.recordSteeringInstructions(a, sc.SteeringPoll{Session: session, Instructions: []sc.SteeringInstruction{instruction}}, nil); err != nil {
			t.Fatal("valid complete discussion message was rejected", err)
		}
		cp, err := j.steeringCheckpoint(a)
		if err != nil || cp.Instructions[instruction.ID].Instruction.Message.Body != instruction.Message.Body {
			t.Fatal("maximum message was silently truncated or rewritten", err)
		}
	})
	t.Run("credential_does_not_reach_journal", func(t *testing.T) {
		a, p, source, session, instruction := steeringFixture188(t)
		j, _ := savedSteering188(t, a, p, source, &session)
		before, err := os.ReadFile(j.path)
		if err != nil {
			t.Fatal(err)
		}
		secret := "owned-private-steering188"
		instruction.Message.Body = "Do not persist this credential: " + secret
		instruction.Message.BodySHA256 = sc.MessageDigest(instruction.Message.Body)
		if j.recordSteeringInstructions(a, sc.SteeringPoll{Session: session, Instructions: []sc.SteeringInstruction{instruction}}, []string{secret}) == nil {
			t.Fatal("credential message entered the durable instruction inbox")
		}
		after, err := os.ReadFile(j.path)
		if err != nil || string(before) != string(after) || strings.Contains(string(after), secret) {
			t.Fatal("rejected credential modified durable storage", err)
		}
	})
	t.Run("same_message_once_and_32_per_session", func(t *testing.T) {
		a, p, source, session, template := steeringFixture188(t)
		j, _ := savedSteering188(t, a, p, source, &session)
		instructions := []sc.SteeringInstruction{}
		for i := 1; i <= 32; i++ {
			instruction := template
			instruction.ID = fmt.Sprintf("instruction188-%02d", i)
			instruction.Message.ID = fmt.Sprintf("message188-%02d", i)
			instruction.Message.Sequence = int64(i)
			instructions = append(instructions, instruction)
		}
		if err := j.recordSteeringInstructions(a, sc.SteeringPoll{Session: session, Instructions: instructions[:1]}, nil); err != nil {
			t.Fatal(err)
		}
		duplicate := instructions[0]
		duplicate.ID = "duplicate-message188"
		if j.recordSteeringInstructions(a, sc.SteeringPoll{Session: session, Instructions: []sc.SteeringInstruction{duplicate}}, nil) == nil {
			t.Fatal("same human message was registered twice below the session cap")
		}
		poll := sc.SteeringPoll{Session: session, Instructions: instructions}
		if err := j.recordSteeringInstructions(a, poll, nil); err != nil {
			t.Fatal("bounded complete session was rejected", err)
		}
		if err := j.recordSteeringInstructions(a, poll, nil); err != nil {
			t.Fatal("exact instruction delivery retry rejected", err)
		}
		extra := template
		extra.ID, extra.Message.ID, extra.Message.Sequence = "instruction188-33", "message188-33", 33
		if j.recordSteeringInstructions(a, sc.SteeringPoll{Session: session, Instructions: []sc.SteeringInstruction{extra}}, nil) == nil {
			t.Fatal("session accepted a 33rd instruction")
		}
		cp, err := j.steeringCheckpoint(a)
		if err != nil || len(cp.Instructions) != 32 {
			t.Fatal("rejected duplicate/cap changed the original instruction set", err)
		}
	})
}

func TestSteeringIntentIsMonotoneAndCannotReplayAfterCrash(t *testing.T) {
	a, p, source, session, instruction := steeringFixture188(t)
	j, dir := savedSteering188(t, a, p, source, &session)
	undelivered := instruction
	undelivered.ID, undelivered.Message.ID = "undelivered188", "undelivered-message188"
	if err := j.recordSteeringInstructions(a, sc.SteeringPoll{Session: session, Instructions: []sc.SteeringInstruction{instruction, undelivered}}, nil); err != nil {
		t.Fatal(err)
	}
	o := steeringObservation188(t, source, instruction, "12")
	if err := j.recordSteeringObservation(a, instruction, o, false); err != nil {
		t.Fatal(err)
	}
	o.Sequence, o.WriteStarted = 2, true
	if err := j.recordSteeringObservation(a, instruction, o, true); err != nil {
		t.Fatal(err)
	}
	if j.recordSteeringObservation(a, instruction, o, true) == nil {
		t.Fatal("second native writer acquired the original durable intent")
	}
	if err := j.recordSteeringObservation(a, instruction, o, false); err != nil {
		t.Fatal("exact persisted observation retry rejected", err)
	}
	changed := o
	changed.WriteWritten = true
	if j.recordSteeringObservation(a, instruction, changed, false) == nil {
		t.Fatal("same sequence accepted a new native write fact")
	}
	changed.Sequence, changed.WriteStarted, changed.WriteWritten = 3, false, false
	if j.recordSteeringObservation(a, instruction, changed, false) == nil {
		t.Fatal("native write intent regressed")
	}
	if err := j.requestSteeringClose(a); err != nil {
		t.Fatal(err)
	}
	closed := steeringObservation188(t, source, undelivered, "13")
	closed.TurnClosed = true
	if err := j.recordSteeringObservation(a, undelivered, closed, false); err != nil {
		t.Fatal(err)
	}
	late := closed
	late.Sequence, late.WriteStarted = 2, true
	if j.recordSteeringObservation(a, undelivered, late, true) == nil {
		t.Fatal("closed session issued a new native write intent")
	}
	before, err := j.steeringCheckpoint(a)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	after, err := reopened.steeringCheckpoint(a)
	if err != nil || !samePermissionValue(before, after) || len(reopened.Pending()) != 1 || !reopened.Pending()[0].OutcomeUnknown {
		t.Fatal("restart changed original operation/body or claimed a successful native outcome", err)
	}
	if reopened.prepareSteering(a, p, source) == nil || reopened.recordSteeringObservation(a, instruction, o, true) == nil {
		t.Fatal("restart recreated a live execution/write permit")
	}
	if after.Instructions[instruction.ID].Observation.WriteWritten || after.Instructions[instruction.ID].Observation.NativeAccepted {
		t.Fatal("durable start intent became a native acceptance ACK")
	}
}

func TestSteeringRecoveryRestoresOriginalOpenObservationAndCloseBeforeResult(t *testing.T) {
	for _, pool := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "parent_pool"}[pool], func(t *testing.T) {
			a, p, source, session, instruction := steeringFixture188(t)
			var scope *PoolContext
			namespace := "runner"
			if pool {
				a.TenantID, a.ProjectID, a.PoolID, a.PoolRunnerID, a.PoolSlotID = "tenant188", session.ProjectID, "pool188", "machine188", "0"
				a.PoolGrantID, a.PoolGrantVersion, a.PoolCredentialID, a.PoolCredentialVersion = "grant188", 1, "credential188", 1
				a.DispatchState, a.LeaseUntil = "active", session.ExpiresAt
				scope = &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID}
				namespace = "pool-runner"
			}
			j, _ := savedSteering188(t, a, p, source, nil)
			o := steeringObservation188(t, source, instruction, "12")
			o.WriteStarted = true
			var mu sync.Mutex
			paths := []string{}
			bodies := map[string]string{}
			counts := map[string]int{}
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				paths = append(paths, request.URL.Path)
				route := strings.TrimPrefix(request.URL.Path, "/api/v1/"+namespace+"/")
				if request.Header.Get("Authorization") != "Bearer private188" || route == request.URL.Path {
					t.Error("steering changed machine credentials or namespace")
				}
				raw, _ := io.ReadAll(request.Body)
				var body map[string]json.RawMessage
				if json.Unmarshal(raw, &body) != nil {
					t.Error("invalid steering HTTP envelope")
				}
				var identity Assignment
				_ = json.Unmarshal(body["assignment"], &identity)
				if route != "results" && (identity.RunID != a.RunID || identity.Fence != a.Fence || identity.DispatchID != a.DispatchID) {
					t.Error("steering changed the original execution tuple")
				}
				if pool && (len(body["tenant_id"]) == 0 || len(body["runner_id"]) == 0 || identity.Config != nil || identity.Command != nil) {
					t.Error("Pool recovery lost the original scope or copied private command configuration")
				}
				w.Header().Set("Content-Type", "application/json")
				if route == "results" {
					cp, err := j.steeringCheckpoint(a)
					if err != nil || !cp.CloseAcked || !cp.Instructions[instruction.ID].Events[0].Acked {
						t.Error("final result overtook original steering observations/close receipts")
					}
					var result Result
					if json.Unmarshal(body["result"], &result) != nil || result.RunID != a.RunID || result.Fence != a.Fence || !result.OutcomeUnknown {
						t.Error("final result changed the original unknown execution outcome")
					}
					return
				}
				if route != "steering/sessions" && route != "steering/observations" && route != "steering/close" {
					t.Error("unexpected recovery operation", route)
					w.WriteHeader(400)
					return
				}
				if len(body["idempotency_key"]) == 0 {
					t.Error("steering receipt has no original operation key")
				}
				if bodies[route] == "" {
					bodies[route] = string(raw)
				} else if bodies[route] != string(raw) {
					t.Error("ACK recovery changed the accepted original body/key", route)
				}
				counts[route]++
				if counts[route] == 1 {
					// Each mutation is accepted once; its first ACK is truncated.
					_, _ = io.WriteString(w, `{"id":`)
					return
				}
				switch route {
				case "steering/sessions":
					_ = json.NewEncoder(w).Encode(sc.SteeringOpenResult{Session: &session})
				case "steering/observations":
					var actual cc.CodexSteeringObservation
					_ = json.Unmarshal(body["observation"], &actual)
					if !samePermissionValue(actual, o) {
						t.Error("recovery changed original native write facts")
					}
					result := instruction
					result.Observation = &actual
					_ = json.NewEncoder(w).Encode(result)
				case "steering/close":
					closed := session
					closed.Version, closed.State, closed.AllowedActions = 2, "closed", []string{}
					_ = json.NewEncoder(w).Encode(closed)
				}
			}))
			defer s.Close()
			client := &Client{BaseURL: s.URL, ProjectID: session.ProjectID, Token: "private188", Pool: scope, HTTP: s.Client()}
			if pool {
				client.ProjectID = "" // A restarted slot starts with its parent client.
			}
			count := func() int { mu.Lock(); defer mu.Unlock(); return len(paths) }
			if err := ReplaySteering(context.Background(), client, j); err == nil || count() != 1 {
				t.Fatal("lost session-open ACK was silently skipped", err)
			}
			cp, _ := j.steeringCheckpoint(a)
			if cp.Session != nil || cp.NotTaskExecution {
				t.Fatal("unknown open became an unavailable or invented live session")
			}
			if err := ReplaySteering(context.Background(), client, j); err != nil || count() != 2 {
				t.Fatal("original session-open receipt was not recovered", err)
			}
			if err := j.recordSteeringInstructions(a, sc.SteeringPoll{Session: session, Instructions: []sc.SteeringInstruction{instruction}}, nil); err != nil {
				t.Fatal(err)
			}
			if err := j.recordSteeringObservation(a, instruction, o, true); err != nil {
				t.Fatal(err)
			}
			if err := j.requestSteeringClose(a); err != nil {
				t.Fatal(err)
			}
			if err := j.Finish(Result{Assignment: a, ExitCode: -1, OutcomeUnknown: true}); err != nil {
				t.Fatal(err)
			}
			replay := func() error {
				if pool {
					return replayPool(context.Background(), client, j)
				}
				return Replay(context.Background(), client, j)
			}
			if err := replay(); err == nil || count() != 3 || len(j.Pending()) != 1 {
				t.Fatal("lost observation ACK allowed close/result to overtake", err)
			}
			if err := replay(); err == nil || count() != 5 || len(j.Pending()) != 1 {
				t.Fatal("lost close ACK allowed final result to overtake", err)
			}
			if err := replay(); err != nil || count() != 7 || len(j.Pending()) != 0 {
				t.Fatal("exact open/observation/close receipts did not precede result", err)
			}
		})
	}
}

func TestSteeringSessionOpenOnlyExplicitNonTaskResultIsUnavailable(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		skip   bool
	}{
		{"not_task_execution", 200, `{"reason":"not_task_execution"}`, true},
		{"authorization_rejected", 403, `{}`, false},
		{"database_unavailable", 500, `{}`, false},
		{"unknown_result", 200, `{}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, p, source, session, _ := steeringFixture188(t)
			j, _ := savedSteering188(t, a, p, source, nil)
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if !strings.HasSuffix(request.URL.Path, "/steering/sessions") {
					t.Error("failed open created another recovery operation")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer s.Close()
			client := &Client{BaseURL: s.URL, ProjectID: session.ProjectID, Token: "private188", HTTP: s.Client()}
			err := ReplaySteering(context.Background(), client, j)
			cp, readErr := j.steeringCheckpoint(a)
			if readErr != nil || cp.Session != nil || cp.NotTaskExecution != test.skip || (err == nil) != test.skip {
				t.Fatal("transport/auth/unknown response became a normal non-task skip", err, readErr)
			}
			if test.status == 403 && (!cp.Blocked || !errors.Is(err, errSteeringReconciliation)) {
				t.Fatal("rejected authority did not retain original reconciliation state", err)
			}
		})
	}
}
