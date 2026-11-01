package codexcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const UserInputMethod = "item/tool/requestUserInput"
const UserInputProtocol = "codex-user-input-v1"

var userInputFailureCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)

type CodexUserInputOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// Questions are normalized from the pinned native schema. Absent native bools
// use its declared false default; options are always an array in this contract.
type CodexUserInputQuestion struct {
	ID       string                 `json:"id"`
	Header   string                 `json:"header"`
	Question string                 `json:"question"`
	IsOther  bool                   `json:"is_other"`
	IsSecret bool                   `json:"is_secret"`
	Options  []CodexUserInputOption `json:"options"`
}

type CodexUserInputRequest struct {
	RequestID        CodexRPCRequestID        `json:"request_id"`
	Method           string                   `json:"method"`
	ThreadID         string                   `json:"thread_id"`
	TurnID           string                   `json:"turn_id"`
	ItemID           string                   `json:"item_id"`
	BindingSHA256    string                   `json:"binding_sha256"`
	ParamsSHA256     string                   `json:"params_sha256"`
	IsBlocking       bool                     `json:"is_blocking"`
	AutoResolutionMS *int64                   `json:"auto_resolution_ms,omitempty"`
	Questions        []CodexUserInputQuestion `json:"questions"`
}

// A selection is human input, never an approval choice. The control plane
// reconstructs the native response rather than accepting arbitrary RPC JSON.
type CodexUserInputSelection struct {
	QuestionID string `json:"question_id"`
	Kind       string `json:"kind"`
	Option     string `json:"option,omitempty"`
	Text       string `json:"text,omitempty"`
}

type CodexUserInputNativeAnswer struct {
	Answers []string `json:"answers"`
}

type CodexUserInputResponse struct {
	Answers map[string]CodexUserInputNativeAnswer `json:"answers"`
}

type CodexUserInputObservation struct {
	Sequence        int64  `json:"sequence"`
	ParamsSHA256    string `json:"params_sha256"`
	AnswerSHA256    string `json:"answer_sha256,omitempty"`
	AnswerObserved  bool   `json:"answer_observed"`
	ReplyStarted    bool   `json:"reply_started"`
	ReplyWritten    bool   `json:"reply_written"`
	RequestResolved bool   `json:"request_resolved"`
	TurnClosed      bool   `json:"turn_closed"`
	FailureCode     string `json:"failure_code,omitempty"`
}

// Evidence carries only exact identities and delivery facts. Question and
// answer bodies stay in the separately authorized tool-input domain.
type CodexUserInputEvidence struct {
	InputID       string                     `json:"input_id"`
	RequestID     CodexRPCRequestID          `json:"request_id"`
	Method        string                     `json:"method"`
	ThreadID      string                     `json:"thread_id"`
	TurnID        string                     `json:"turn_id"`
	ItemID        string                     `json:"item_id"`
	BindingSHA256 string                     `json:"binding_sha256"`
	ParamsSHA256  string                     `json:"params_sha256"`
	QuestionCount int                        `json:"question_count"`
	IsBlocking    bool                       `json:"is_blocking"`
	Observation   *CodexUserInputObservation `json:"observation,omitempty"`
}

func ValidateUserInputRequest(r CodexUserInputRequest) error {
	if ValidateRPCRequestID(r.RequestID) != nil || r.Method != UserInputMethod || !validID(r.ThreadID, false) || !validID(r.TurnID, false) || !validID(r.ItemID, false) || !hash.MatchString(r.BindingSHA256) || !hash.MatchString(r.ParamsSHA256) || len(r.Questions) < 1 || len(r.Questions) > 3 || r.AutoResolutionMS != nil && (*r.AutoResolutionMS < 0 || *r.AutoResolutionMS > MaxSafeInteger) {
		return fmt.Errorf("invalid native Codex user input request")
	}
	seen := map[string]bool{}
	for _, q := range r.Questions {
		if !approvalText(q.ID, 100, true, false) || seen[q.ID] || !approvalText(q.Header, 80, true, false) || !approvalText(q.Question, 4000, true, true) || q.IsSecret || q.Options == nil || len(q.Options) > 12 {
			return fmt.Errorf("unsupported or invalid native Codex user input question")
		}
		seen[q.ID] = true
		labels := map[string]bool{}
		for _, option := range q.Options {
			if !approvalText(option.Label, 200, true, false) || labels[option.Label] || !approvalText(option.Description, 1000, false, true) {
				return fmt.Errorf("invalid native Codex user input options")
			}
			labels[option.Label] = true
		}
	}
	return nil
}

func BuildUserInputResponse(r CodexUserInputRequest, selections []CodexUserInputSelection) (CodexUserInputResponse, error) {
	out := CodexUserInputResponse{Answers: map[string]CodexUserInputNativeAnswer{}}
	if ValidateUserInputRequest(r) != nil || len(selections) != len(r.Questions) {
		return out, fmt.Errorf("user input response must cover the exact original questions")
	}
	questions := map[string]CodexUserInputQuestion{}
	for _, q := range r.Questions {
		questions[q.ID] = q
	}
	for _, selection := range selections {
		q, found := questions[selection.QuestionID]
		if !found || !approvalText(selection.Text, 4000, false, true) {
			return out, fmt.Errorf("invalid user input response question")
		}
		delete(questions, selection.QuestionID)
		values := []string{} // An explicit skip is [], never null or an approval.
		note := strings.TrimSpace(selection.Text)
		switch selection.Kind {
		case "skip":
			if selection.Option != "" || selection.Text != "" {
				return out, fmt.Errorf("skipped question cannot carry an answer")
			}
		case "option":
			matched := false
			for _, option := range q.Options {
				matched = matched || option.Label == selection.Option
			}
			if !matched {
				return out, fmt.Errorf("answer option is not in the original question")
			}
			values = append(values, selection.Option)
		case "other":
			if !q.IsOther || len(q.Options) == 0 || selection.Option != "" {
				return out, fmt.Errorf("question does not offer an other option")
			}
			values = append(values, "None of the above")
		case "text":
			if len(q.Options) != 0 || selection.Option != "" || note == "" {
				return out, fmt.Errorf("question requires its original answer format")
			}
		default:
			return out, fmt.Errorf("unknown user input selection kind")
		}
		if selection.Kind != "skip" && note != "" {
			values = append(values, "user_note: "+note)
		}
		out.Answers[q.ID] = CodexUserInputNativeAnswer{Answers: values}
	}
	return out, nil
}

func UserInputAnswerDigest(r CodexUserInputRequest, selections []CodexUserInputSelection) (string, error) {
	response, err := BuildUserInputResponse(r, selections)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(response)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func ValidateUserInputObservation(o CodexUserInputObservation) error {
	if o.Sequence < 1 || o.Sequence > MaxSafeInteger || !hash.MatchString(o.ParamsSHA256) || o.AnswerObserved != (o.AnswerSHA256 != "") || o.AnswerSHA256 != "" && !hash.MatchString(o.AnswerSHA256) || o.ReplyStarted && !o.AnswerObserved || o.ReplyWritten && !o.ReplyStarted || o.FailureCode != "" && !userInputFailureCode.MatchString(o.FailureCode) {
		return fmt.Errorf("invalid Codex user input delivery observation")
	}
	return nil
}

func ValidateUserInputEvidence(e CodexUserInputEvidence) error {
	if !validID(e.InputID, false) || ValidateRPCRequestID(e.RequestID) != nil || e.Method != UserInputMethod || !validID(e.ThreadID, false) || !validID(e.TurnID, false) || !validID(e.ItemID, false) || !hash.MatchString(e.BindingSHA256) || !hash.MatchString(e.ParamsSHA256) || e.QuestionCount < 1 || e.QuestionCount > 3 || e.Observation != nil && (ValidateUserInputObservation(*e.Observation) != nil || e.Observation.ParamsSHA256 != e.ParamsSHA256) {
		return fmt.Errorf("invalid Codex user input evidence")
	}
	return nil
}
