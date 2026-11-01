package codexcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func userInputFixture() CodexUserInputRequest {
	return CodexUserInputRequest{RequestID: CodexRPCRequestID{Kind: "number", Value: "17"}, Method: UserInputMethod, ThreadID: "thread", TurnID: "turn", ItemID: "item", BindingSHA256: strings.Repeat("a", 64), ParamsSHA256: strings.Repeat("b", 64), Questions: []CodexUserInputQuestion{
		{ID: "choice", Header: "Choice", Question: "Choose", IsOther: true, Options: []CodexUserInputOption{{Label: "A", Description: "First"}, {Label: "B", Description: "Second"}}},
		{ID: "text", Header: "Details", Question: "Describe", Options: []CodexUserInputOption{}},
		{ID: "skip", Header: "Optional", Question: "Optional", Options: []CodexUserInputOption{}},
	}}
}

func TestUserInputAnswersPreserveNativeMeaningAndDigest(t *testing.T) {
	r := userInputFixture()
	s := []CodexUserInputSelection{{QuestionID: "choice", Kind: "option", Option: "B", Text: " why "}, {QuestionID: "text", Kind: "text", Text: " details "}, {QuestionID: "skip", Kind: "skip"}}
	response, err := BuildUserInputResponse(r, s)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(response)
	if string(raw) != `{"answers":{"choice":{"answers":["B","user_note: why"]},"skip":{"answers":[]},"text":{"answers":["user_note: details"]}}}` {
		t.Fatal(string(raw))
	}
	digest, err := UserInputAnswerDigest(r, s)
	if err != nil {
		t.Fatal(err)
	}
	s[0], s[2] = s[2], s[0]
	if other, _ := UserInputAnswerDigest(r, s); other != digest {
		t.Fatal("same native response has an order-dependent digest")
	}
	s[2] = CodexUserInputSelection{QuestionID: "choice", Kind: "other", Text: " custom "}
	response, err = BuildUserInputResponse(r, s)
	if err != nil || response.Answers["choice"].Answers[0] != "None of the above" {
		t.Fatal("other answer was converted into an approval", err)
	}
	s[2].Kind = "accept"
	if _, err = BuildUserInputResponse(r, s); err == nil {
		t.Fatal("permission choice accepted as an answer")
	}
	s[2] = s[1]
	if _, err = BuildUserInputResponse(r, s); err == nil {
		t.Fatal("duplicate question replaced exact coverage")
	}
	r.Questions[0].IsSecret = true
	if ValidateUserInputRequest(r) == nil {
		t.Fatal("secret question accepted for ordinary persistence")
	}
}

func TestUserInputDeliveryAndEvidenceKeepOriginalScope(t *testing.T) {
	r := userInputFixture()
	o := CodexUserInputObservation{Sequence: 1, ParamsSHA256: r.ParamsSHA256, RequestResolved: true}
	if ValidateUserInputObservation(o) != nil {
		t.Fatal("unanswered native cleanup must remain valid")
	}
	o.ReplyWritten = true
	if ValidateUserInputObservation(o) == nil {
		t.Fatal("native closure invented a response write")
	}
	o.ReplyWritten = false
	o.FailureCode = "C:/private/error"
	if ValidateUserInputObservation(o) == nil {
		t.Fatal("raw path admitted as safe failure code")
	}
	o.FailureCode = ""
	x := CodexUserInputEvidence{InputID: "input", RequestID: r.RequestID, Method: r.Method, ThreadID: r.ThreadID, TurnID: r.TurnID, ItemID: r.ItemID, BindingSHA256: r.BindingSHA256, ParamsSHA256: r.ParamsSHA256, QuestionCount: 3, Observation: &o}
	e := Evidence{BindingSHA256: r.BindingSHA256, ThreadID: r.ThreadID, TurnID: r.TurnID, RequestedModel: "fixture", AcknowledgedModel: "fixture", AcknowledgedProvider: "owned", ConfigurationAcknowledged: true, UserInputs: []CodexUserInputEvidence{x}}
	if err := ValidateEvidence(e); err != nil {
		t.Fatal(err)
	}
	copy := CloneEvidence(&e)
	copy.UserInputs[0].Observation.Sequence = 2
	if e.UserInputs[0].Observation.Sequence != 1 {
		t.Fatal("journal clone aliases delivery facts")
	}
	e.UserInputs[0].BindingSHA256 = strings.Repeat("c", 64)
	if ValidateEvidence(e) == nil {
		t.Fatal("question evidence escaped original configuration")
	}
}
