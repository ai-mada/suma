package claudecontract

import (
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
	"strings"
	"testing"
)

func sdkBasis(raw string) Basis {
	return Basis{SessionID: "sdk-session", ToolUseID: "tool-1", RequestID: "native-request", CallbackID: "callback-1", ConfigurationSHA256: strings.Repeat("a", 64), ParamsSHA256: PayloadDigest(raw)}
}
func TestSDKCallbackDecisionBindsNativeRequestAndOriginalPayload(t *testing.T) {
	raw := `{"command":"git status --short"}`
	source := PermissionSource{Basis: sdkBasis(raw), ToolName: "Bash", InputJSON: raw}
	choice := PermissionChoice{Outcome: "allow_once"}
	original, err := PermissionDecisionDigest(source, choice)
	if err != nil {
		t.Fatal(err)
	}
	moved := source
	moved.RequestID = "different-native-request"
	next, err := PermissionDecisionDigest(moved, choice)
	if err != nil || next == original {
		t.Fatal("native request lost from decision identity")
	}
	moved = source
	moved.InputJSON = `{"command":"git reset --hard"}`
	if ValidatePermissionSource(moved) == nil {
		t.Fatal("changed command borrowed an approved input digest")
	}
	for _, bad := range []string{`{"x":1,"x":2}`, `{"x":"\ud800"}`, `null`} {
		p := PermissionSource{Basis: sdkBasis(bad), ToolName: "Bash", InputJSON: bad}
		if ValidatePermissionSource(p) == nil {
			t.Fatalf("unsafe payload admitted: %s", bad)
		}
	}
	if ValidatePermissionChoice(PermissionChoice{Outcome: "allow_always"}) == nil {
		t.Fatal("persistent rule silently granted")
	}
	o := delivery.Observation{Sequence: 1, ParamsSHA256: source.ParamsSHA256, AnswerSHA256: original, AnswerObserved: true, ReplyStarted: true, ReplyWritten: true, RequestResolved: true}
	if ValidatePermissionObservation(o) == nil {
		t.Fatal("local SDK delivery fabricated native acceptance ACK")
	}
}
func TestSDKQuestionPreservesNativeTextAndRejectsForgedSelections(t *testing.T) {
	raw := `{"questions":[{"question":"选择实现方案？","header":"设计","options":[{"label":"保留 <A>","description":"中文"},{"label":"新建 B","description":""}],"multiSelect":true}],"answers":{}}`
	source, err := NewInputRequest(sdkBasis(raw), raw)
	if err != nil {
		t.Fatal(err)
	}
	answer := InputDecision{Outcome: "answered", Answers: []InputSelection{{QuestionID: "q1", OptionIDs: []string{"o2", "o1"}}}}
	native, err := NativeAnswers(source, answer)
	if err != nil || native["选择实现方案？"] != "保留 <A>, 新建 B" {
		t.Fatalf("native original labels lost: %v %v", native, err)
	}
	digest, _ := InputDecisionDigest(source, answer)
	answer.Answers[0].OptionIDs = []string{"o1", "o2"}
	reordered, _ := InputDecisionDigest(source, answer)
	if digest != reordered {
		t.Fatal("selection order changed equivalent decision")
	}
	forged := source
	forged.Questions = append([]Question(nil), source.Questions...)
	forged.Questions[0].Prompt = "Changed prompt"
	if ValidateInputRequest(forged) == nil {
		t.Fatal("displayed question differs from original payload")
	}
	answer.Answers[0].OptionIDs = []string{"o1", "o1"}
	if ValidateInputDecision(source, answer) == nil {
		t.Fatal("duplicate selection admitted")
	}
	answer.Answers[0].OptionIDs = []string{"injected"}
	if ValidateInputDecision(source, answer) == nil {
		t.Fatal("unknown option admitted")
	}
	bad := strings.Replace(raw, `"answers":{}`, `"answers":{"选择实现方案？":"新建 B"}`, 1)
	if _, err := NewInputRequest(sdkBasis(bad), bad); err == nil {
		t.Fatal("pre-filled native answer bypassed human choice")
	}
	for _, missing := range []string{strings.Replace(raw, `,"multiSelect":true`, "", 1), strings.Replace(raw, `,"description":"中文"`, "", 1)} {
		if _, err := NewInputRequest(sdkBasis(missing), missing); err == nil {
			t.Fatal("required native field silently defaulted")
		}
	}
}
