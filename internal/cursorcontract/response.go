package cursorcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type CursorInputNativeAnswer struct {
	QuestionID        string   `json:"questionId"`
	SelectedOptionIDs []string `json:"selectedOptionIds"`
}
type CursorInputNativeOutcome struct {
	Outcome string                    `json:"outcome"`
	Answers []CursorInputNativeAnswer `json:"answers,omitempty"`
	Reason  string                    `json:"reason,omitempty"`
}
type CursorInputNativeResult struct {
	Outcome CursorInputNativeOutcome `json:"outcome"`
}

// BuildResponse is the only native response encoder. Browser data cannot
// supply RPC JSON, change a plan URI or turn a plan decision into a permission.
func BuildResponse(r CursorInputRequest, d CursorInputDecision) (CursorInputNativeResult, error) {
	var out CursorInputNativeResult
	if ValidateRequest(r) != nil || !fixedText(d.Reason, 4000, false, true) {
		return out, fmt.Errorf("invalid original Cursor input response")
	}
	out.Outcome.Outcome = d.Outcome
	if d.Outcome == "cancelled" && len(d.Answers) == 0 && d.Reason == "" {
		return out, nil
	}
	if r.Plan != nil {
		if len(d.Answers) != 0 || d.Outcome != "accepted" && d.Outcome != "rejected" || d.Outcome == "accepted" && d.Reason != "" {
			return out, fmt.Errorf("invalid explicit plan decision")
		}
		out.Outcome.Reason = d.Reason
		return out, nil
	}
	if d.Outcome == "skipped" && len(d.Answers) == 0 {
		out.Outcome.Reason = d.Reason
		return out, nil
	}
	if d.Outcome != "answered" || d.Reason != "" || len(d.Answers) != len(r.Questions.Questions) {
		return out, fmt.Errorf("Cursor questions require a complete original answer")
	}
	answers := map[string][]string{}
	for _, selection := range d.Answers {
		if _, duplicate := answers[selection.QuestionID]; duplicate || selection.OptionIDs == nil {
			return out, fmt.Errorf("duplicate or missing question answer")
		}
		answers[selection.QuestionID] = selection.OptionIDs
	}
	for _, q := range r.Questions.Questions {
		chosen, exists := answers[q.ID]
		if !exists || len(chosen) < 1 || !q.AllowMultiple && len(chosen) != 1 || len(chosen) > len(q.Options) {
			return out, fmt.Errorf("Cursor selection differs from its original question")
		}
		selected := map[string]bool{}
		for _, id := range chosen {
			if selected[id] {
				return out, fmt.Errorf("duplicate Cursor option")
			}
			selected[id] = true
		}
		native := CursorInputNativeAnswer{QuestionID: q.ID, SelectedOptionIDs: []string{}}
		for _, option := range q.Options {
			if selected[option.ID] {
				native.SelectedOptionIDs = append(native.SelectedOptionIDs, option.ID)
				delete(selected, option.ID)
			}
		}
		if len(selected) != 0 {
			return out, fmt.Errorf("unknown Cursor option ID")
		}
		out.Outcome.Answers = append(out.Outcome.Answers, native)
	}
	return out, nil
}
func AnswerDigest(r CursorInputRequest, d CursorInputDecision) (string, error) {
	response, err := BuildResponse(r, d)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(struct {
		ParamsSHA256 string                  `json:"params_sha256"`
		Response     CursorInputNativeResult `json:"response"`
	}{r.ParamsSHA256, response})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
