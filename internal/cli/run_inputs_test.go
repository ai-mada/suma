package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/inputdelivery"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

const (
	inputCLIProject  = "project-one"
	inputCLIRun      = "run-one"
	inputCLINode     = "node-one"
	inputCLIPath     = "/api/v1/runs/run-one/nodes/node-one/tool-inputs"
	inputCLIQuestion = "private question text for this test"
	inputCLIAnswer   = "private answer text for this test"
	inputCLIToken    = "isolated-session-secret"
)

func runInputsArgs() []string {
	return []string{"run", "inputs", inputCLIRun, "--node", inputCLINode, "--attempt", "2", "--fence", "9"}
}

func invokeRunInputs(control string, args []string) (int, string, string) {
	var out, diagnostic bytes.Buffer
	base := []string{"--control", control, "--project", inputCLIProject, "--token", inputCLIToken,
		"--expected-user", strings.Repeat("a", 32), "--expected-organization", strings.Repeat("b", 32), "--language", "en-US"}
	code := Execute(context.Background(), append(base, args...), nil, &out, &diagnostic)
	return code, out.String(), diagnostic.String()
}

func runInputRecord(t *testing.T, id string, saved bool) ic.ToolInput {
	t.Helper()
	item := ic.ToolInput{
		ID: id, ProjectID: inputCLIProject, RunID: inputCLIRun, NodeID: inputCLINode, Attempt: 2, Fence: 9,
		DispatchID: "dispatch-one", WorkerID: "worker-one", ProfileSHA256: strings.Repeat("1", 64),
		Version: 1, State: "pending", CreatedAt: "2026-11-06T01:02:03Z", ExpiresAt: "2026-11-06T01:08:03Z",
		AllowedActions: []string{"answer"},
		Request: ic.ToolInputRequest{
			IdempotencyKey: "request-operation", Protocol: cc.Protocol,
			Codex: &cc.CodexUserInputRequest{
				RequestID: cc.CodexRPCRequestID{Kind: "string", Value: "private-native-request"}, Method: cc.UserInputMethod,
				ThreadID: "thread", TurnID: "turn", ItemID: "item", BindingSHA256: strings.Repeat("2", 64),
				ParamsSHA256: strings.Repeat("3", 64), IsBlocking: true,
				Questions: []cc.CodexUserInputQuestion{{ID: "design", Header: "Design", Question: inputCLIQuestion, IsOther: true, Options: []cc.CodexUserInputOption{}}},
			},
		},
	}
	if saved {
		answers := []cc.CodexUserInputSelection{{QuestionID: "design", Kind: "text", Text: inputCLIAnswer}}
		digest, err := ic.AnswerDigest(item.Request, answers, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		item.Answer = &ic.ToolInputAnswer{Answers: answers, AnswerSHA256: digest, ActorID: "private-answer-actor", AnsweredAt: "2026-11-06T01:03:03Z"}
		item.Version, item.State, item.AllowedActions = 2, "answered", []string{}
	}
	if err := ic.ValidateInput(item); err != nil {
		t.Fatal(err)
	}
	return item
}

func TestRunInputsReadsExactMetadataOnly(t *testing.T) {
	pending := runInputRecord(t, "input-pending", false)
	saved := runInputRecord(t, "input-saved", true)
	written := runInputRecord(t, "input-written", true)
	written.State = "closed"
	written.Observation = &inputdelivery.Observation{
		Sequence: 1, ParamsSHA256: written.Request.Codex.ParamsSHA256, AnswerSHA256: written.Answer.AnswerSHA256,
		AnswerObserved: true, ReplyStarted: true, ReplyWritten: true, RequestResolved: true, ExecutionClosed: true,
		FailureCode: "private_failure_code",
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(req.Body)
		query := url.Values{"project_id": {inputCLIProject}, "attempt": {"2"}, "fence": {"9"}}
		if req.Method != http.MethodGet || req.URL.Path != inputCLIPath || !reflect.DeepEqual(req.URL.Query(), query) || err != nil || len(body) != 0 {
			t.Errorf("invalid scoped read: %s %s body bytes=%d (%v)", req.Method, req.URL, len(body), err)
		}
		if req.Header.Get("Authorization") != "Bearer "+inputCLIToken ||
			req.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) ||
			req.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
			t.Error("original browser identity was not preserved")
		}
		_ = json.NewEncoder(w).Encode(ic.ToolInputList{Protocol: cc.UserInputProtocol, Items: []ic.ToolInput{pending, saved, written}})
	}))
	defer server.Close()
	code, out, diagnostic := invokeRunInputs(server.URL, append(runInputsArgs(), "--workbench-origin", "https://workbench.example"))
	if code != 0 || diagnostic != "" || calls.Load() != 1 {
		t.Fatalf("exit=%d reads=%d stdout=%q stderr=%q", code, calls.Load(), out, diagnostic)
	}
	for _, secret := range []string{inputCLIQuestion, inputCLIAnswer, inputCLIToken, "private-answer-actor", "private-native-request", "request-operation", "worker-one", saved.Answer.AnswerSHA256, "private_failure_code"} {
		if strings.Contains(out, secret) {
			t.Fatal("terminal printed private request or answer data")
		}
	}
	var result struct {
		Protocol     string `json:"protocol"`
		WorkbenchURL string `json:"workbench_url"`
		Items        []struct {
			ID             string   `json:"id"`
			State          string   `json:"state"`
			AnswerSaved    bool     `json:"answer_saved"`
			AllowedActions []string `json:"allowed_actions"`
			Delivery       *struct {
				AnswerObserved  bool   `json:"answer_observed"`
				WriteState      string `json:"write_state"`
				RequestResolved bool   `json:"request_resolved"`
				FailureReported bool   `json:"failure_reported"`
			} `json:"delivery"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || result.Protocol != cc.UserInputProtocol || len(result.Items) != 3 {
		t.Fatal("expected scoped input summary", err, out)
	}
	first, second, third := result.Items[0], result.Items[1], result.Items[2]
	if first.ID != pending.ID || first.State != "pending" || first.AnswerSaved || first.Delivery != nil ||
		!reflect.DeepEqual(first.AllowedActions, []string{"answer"}) ||
		second.ID != saved.ID || !second.AnswerSaved || second.State != "answered" || second.Delivery != nil ||
		third.ID != written.ID || !third.AnswerSaved || third.State != "closed" || third.Delivery == nil ||
		!third.Delivery.AnswerObserved || !third.Delivery.RequestResolved || third.Delivery.WriteState != "written" || !third.Delivery.FailureReported {
		t.Fatalf("saved answer and runtime delivery were conflated: %s", out)
	}
	workbench, err := url.Parse(result.WorkbenchURL)
	query := url.Values{"project": {inputCLIProject}, "view": {"development"}, "sub": {"runs"}, "run": {inputCLIRun}, "node": {inputCLINode}, "attempt": {"2"}}
	if err != nil || workbench.Scheme != "https" || workbench.Host != "workbench.example" || !reflect.DeepEqual(workbench.Query(), query) {
		t.Fatalf("workbench link lost exact scope: %q (%v)", result.WorkbenchURL, err)
	}
}

func TestRunInputsRejectsMismatchedOrMalformedResponse(t *testing.T) {
	record := runInputRecord(t, "input-one", false)
	encoded, err := json.Marshal(ic.ToolInputList{Protocol: cc.UserInputProtocol, Items: []ic.ToolInput{record}})
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := json.Marshal(ic.ToolInputList{Protocol: cc.UserInputProtocol, Items: []ic.ToolInput{record, record}})
	if err != nil {
		t.Fatal(err)
	}
	body := string(encoded)
	for name, malformed := range map[string]string{
		"project":              strings.Replace(body, `"project_id":"project-one"`, `"project_id":"other"`, 1),
		"attempt":              strings.Replace(body, `"attempt":2`, `"attempt":3`, 1),
		"fence":                strings.Replace(body, `"fence":9`, `"fence":10`, 1),
		"duplicate input":      string(duplicate),
		"null page":            `{"protocol":"","items":null}`,
		"missing page":         `{"protocol":""}`,
		"unsupported protocol": strings.Replace(body, cc.UserInputProtocol, "unknown-protocol", 1),
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.Method != http.MethodGet {
					t.Error("unexpected write")
				}
				_, _ = io.WriteString(w, malformed)
			}))
			defer server.Close()
			code, out, diagnostic := invokeRunInputs(server.URL, runInputsArgs())
			if code != 1 || out != "" || !strings.Contains(diagnostic, `"code": "run_inputs_response_invalid"`) {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, out, diagnostic)
			}
		})
	}
}

func TestRunInputsAllowsExactHistoryWithoutCurrentCapability(t *testing.T) {
	record := runInputRecord(t, "input-historical", true)
	record.State = "closed"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewEncoder(w).Encode(ic.ToolInputList{Items: []ic.ToolInput{record}})
	}))
	defer server.Close()
	code, out, diagnostic := invokeRunInputs(server.URL, runInputsArgs())
	if code != 0 || diagnostic != "" || !strings.Contains(out, record.ID) || !strings.Contains(out, `"allowed_actions": []`) {
		t.Fatalf("exact readable history rejected: exit=%d stdout=%q stderr=%q", code, out, diagnostic)
	}
}

func TestRunInputsRejectsBadFlagsAndPreservesAuthorityRefusal(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"code":"forbidden","message":"browser session required"}}`)
	}))
	defer server.Close()
	for _, args := range [][]string{
		{"run", "inputs", inputCLIRun, "--node", inputCLINode, "--attempt", "2"},
		append(runInputsArgs(), "--fence", "10"),
		append(runInputsArgs(), "--workbench-origin", "https://user:credential@workbench.example"),
		append(runInputsArgs(), "--workbench-origin", "https://workbench.example/some-path"),
	} {
		code, out, diagnostic := invokeRunInputs(server.URL, args)
		if code != 2 || out != "" || !strings.Contains(diagnostic, `"code": "usage"`) || calls.Load() != 0 {
			t.Fatalf("invalid flags reached control plane: exit=%d reads=%d stdout=%q stderr=%q", code, calls.Load(), out, diagnostic)
		}
	}
	code, out, diagnostic := invokeRunInputs(server.URL, runInputsArgs())
	if code != 1 || out != "" || !strings.Contains(diagnostic, `"code": "forbidden"`) || calls.Load() != 1 {
		t.Fatalf("authority refusal changed: exit=%d reads=%d stdout=%q stderr=%q", code, calls.Load(), out, diagnostic)
	}
}
