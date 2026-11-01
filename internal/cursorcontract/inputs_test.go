package cursorcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func testQuestion(t *testing.T) CursorInputRequest {
	t.Helper()
	r := CursorInputRequest{RequestID: CursorRPCRequestID{Kind: "number", Value: "17"}, PromptID: CursorRPCRequestID{Kind: "number", Value: "3"}, SessionID: "session-190", Method: AskQuestionMethod, ConfigurationSHA256: strings.Repeat("a", 64), Questions: &CursorAskQuestion{ToolCallID: "call-190", Questions: []CursorQuestion{{ID: "q", Prompt: "Choose scope", AllowMultiple: true, Options: []CursorQuestionOption{{ID: "api", Label: "API"}, {ID: "ui", Label: "UI"}}}}}}
	var err error
	r.ParamsSHA256, err = ParamsDigest(r)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCursorAnswersKeepExactIDsAndOriginalPlanDigest(t *testing.T) {
	r := testQuestion(t)
	d := CursorInputDecision{Outcome: "answered", Answers: []CursorInputSelection{{QuestionID: "q", OptionIDs: []string{"ui", "api"}}}}
	response, err := BuildResponse(r, d)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(response)
	if string(raw) != `{"outcome":{"outcome":"answered","answers":[{"questionId":"q","selectedOptionIds":["api","ui"]}]}}` {
		t.Fatal("response lost native IDs/order", string(raw))
	}
	d.Answers[0].OptionIDs = []string{"api", "api"}
	if _, err = BuildResponse(r, d); err == nil {
		t.Fatal("duplicate selection accepted")
	}
	d.Answers[0].OptionIDs = []string{"API"}
	if _, err = BuildResponse(r, d); err == nil {
		t.Fatal("display label replaced original ID")
	}
	r.Method, r.Questions, r.Plan = CreatePlanMethod, nil, &CursorCreatePlan{ToolCallID: "plan-190", Plan: "Review exact plan\nThen implement", Todos: []CursorTodo{}}
	r.ParamsSHA256, _ = ParamsDigest(r)
	first, err := AnswerDigest(r, CursorInputDecision{Outcome: "accepted"})
	if err != nil {
		t.Fatal(err)
	}
	r.Plan.Plan += "\nChanged scope"
	if _, err = BuildResponse(r, CursorInputDecision{Outcome: "accepted"}); err == nil {
		t.Fatal("changed plan kept prior authority")
	}
	r.ParamsSHA256, _ = ParamsDigest(r)
	second, _ := AnswerDigest(r, CursorInputDecision{Outcome: "accepted"})
	if first == second {
		t.Fatal("accepted digest did not bind exact plan")
	}
	if _, err = BuildResponse(r, CursorInputDecision{Outcome: "answered", Answers: []CursorInputSelection{}}); err == nil {
		t.Fatal("plan answered as a question")
	}
	for _, outcome := range []string{"rejected", "cancelled"} {
		v, err := BuildResponse(r, CursorInputDecision{Outcome: outcome})
		if err != nil || v.Outcome.Outcome != outcome {
			t.Fatal("decline became a protocol error", outcome, err)
		}
	}
}

func TestCursorInputLimitsPreserveBoundedCompleteMarkdown(t *testing.T) {
	r := testQuestion(t)
	r.Method, r.Questions, r.Plan = CreatePlanMethod, nil, &CursorCreatePlan{ToolCallID: "plan", Plan: "# Exact plan\r\n\tReview first", Todos: []CursorTodo{}}
	r.ParamsSHA256, _ = ParamsDigest(r)
	if ValidateRequest(r) != nil {
		t.Fatal("complete CRLF Markdown rejected")
	}
	r.Plan.Plan = strings.Repeat("p", MaxPlanBytes+1)
	if _, err := ParamsDigest(r); err == nil {
		t.Fatal("oversized plan accepted")
	}
	r.Plan.Plan = "Plan"
	for i := 0; i < 80; i++ {
		r.Plan.Todos = append(r.Plan.Todos, CursorTodo{ID: strings.Repeat("x", i+1), Content: strings.Repeat("c", 4000), Status: "pending"})
	}
	if _, err := ParamsDigest(r); err == nil {
		t.Fatal("individually valid fields exceeded aggregate payload limit")
	}
}
