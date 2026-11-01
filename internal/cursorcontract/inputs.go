// Package cursorcontract owns Cursor's exact native question/plan vocabulary.
// It reconstructs typed responses and never grants execution permissions.
package cursorcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	delivery "github.com/ai-mada/suma/internal/inputdelivery"
)

const InputProtocol = "cursor-acp-input-v1"
const AskQuestionMethod = "cursor/ask_question"
const CreatePlanMethod = "cursor/create_plan"
const MaxQuestions = 16
const MaxOptions = 32
const MaxPlanBytes = 128 << 10
const MaxTodos = 256
const MaxPhases = 32
const MaxPayloadBytes = 256 << 10

// Numeric RPC IDs remain canonical decimal strings, without float conversion.
// Prompt IDs and server request IDs have separate protocol directions.
type CursorRPCRequestID struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type CursorQuestionOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type CursorQuestion struct {
	ID            string                 `json:"id"`
	Prompt        string                 `json:"prompt"`
	Options       []CursorQuestionOption `json:"options"`
	AllowMultiple bool                   `json:"allow_multiple"`
}
type CursorAskQuestion struct {
	ToolCallID string           `json:"tool_call_id"`
	Title      string           `json:"title,omitempty"`
	Questions  []CursorQuestion `json:"questions"`
}
type CursorTodo struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"`
}
type CursorPlanPhase struct {
	Name  string       `json:"name"`
	Todos []CursorTodo `json:"todos"`
}
type CursorCreatePlan struct {
	ToolCallID string            `json:"tool_call_id"`
	Name       string            `json:"name,omitempty"`
	Overview   string            `json:"overview,omitempty"`
	Plan       string            `json:"plan"`
	Todos      []CursorTodo      `json:"todos"`
	IsProject  bool              `json:"is_project"`
	Phases     []CursorPlanPhase `json:"phases,omitempty"`
}

type CursorInputRequest struct {
	RequestID           CursorRPCRequestID `json:"request_id"`
	SessionID           string             `json:"session_id"`
	PromptID            CursorRPCRequestID `json:"prompt_id"`
	Method              string             `json:"method"`
	ConfigurationSHA256 string             `json:"configuration_sha256"`
	ParamsSHA256        string             `json:"params_sha256"`
	Questions           *CursorAskQuestion `json:"questions,omitempty"`
	Plan                *CursorCreatePlan  `json:"plan,omitempty"`
}
type CursorInputSelection struct {
	QuestionID string   `json:"question_id"`
	OptionIDs  []string `json:"option_ids"`
}
type CursorInputDecision struct {
	Outcome string                 `json:"outcome"`
	Answers []CursorInputSelection `json:"answers,omitempty"`
	Reason  string                 `json:"reason,omitempty"`
}

// Evidence excludes the separately authorized question, answer and plan body.
type CursorInputEvidence struct {
	InputID             string                `json:"input_id"`
	RequestID           CursorRPCRequestID    `json:"request_id"`
	PromptID            CursorRPCRequestID    `json:"prompt_id"`
	SessionID           string                `json:"session_id"`
	Method              string                `json:"method"`
	ConfigurationSHA256 string                `json:"configuration_sha256"`
	ParamsSHA256        string                `json:"params_sha256"`
	Kind                string                `json:"kind"`
	QuestionCount       int                   `json:"question_count"`
	Observation         *delivery.Observation `json:"observation,omitempty"`
}

var hash = regexp.MustCompile(`^[a-f0-9]{64}$`)

func fixedText(s string, maximum int, required, multiline bool) bool {
	return utf8.ValidString(s) && len(s) <= maximum && (!required || strings.TrimSpace(s) != "") && strings.IndexFunc(s, func(r rune) bool {
		return unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t'))
	}) < 0
}
func identifier(s string) bool { return fixedText(s, 256, true, false) && strings.TrimSpace(s) == s }
func ValidateRPCRequestID(v CursorRPCRequestID) error {
	if v.Kind == "string" && identifier(v.Value) {
		return nil
	}
	if v.Kind == "number" {
		n, err := strconv.ParseInt(v.Value, 10, 64)
		if err == nil && strconv.FormatInt(n, 10) == v.Value {
			return nil
		}
	}
	return fmt.Errorf("invalid exact Cursor RPC identity")
}
func validTodos(values []CursorTodo) bool {
	if values == nil || len(values) > MaxTodos {
		return false
	}
	seen := map[string]bool{}
	for _, v := range values {
		if !identifier(v.ID) || seen[v.ID] || !fixedText(v.Content, 4000, true, true) {
			return false
		}
		seen[v.ID] = true
		switch v.Status {
		case "pending", "in_progress", "completed", "cancelled":
		default:
			return false
		}
	}
	return true
}
func validPayload(r CursorInputRequest) bool {
	switch r.Method {
	case AskQuestionMethod:
		q := r.Questions
		if q == nil || r.Plan != nil || !identifier(q.ToolCallID) || !fixedText(q.Title, 256, false, false) || len(q.Questions) < 1 || len(q.Questions) > MaxQuestions {
			return false
		}
		seen := map[string]bool{}
		for _, item := range q.Questions {
			if !identifier(item.ID) || seen[item.ID] || !fixedText(item.Prompt, 4000, true, true) || len(item.Options) < 1 || len(item.Options) > MaxOptions {
				return false
			}
			seen[item.ID] = true
			options := map[string]bool{}
			for _, option := range item.Options {
				if !identifier(option.ID) || options[option.ID] || !fixedText(option.Label, 1000, true, false) {
					return false
				}
				options[option.ID] = true
			}
		}
		return true
	case CreatePlanMethod:
		p := r.Plan
		if p == nil || r.Questions != nil || !identifier(p.ToolCallID) || !fixedText(p.Name, 256, false, false) || !fixedText(p.Overview, 8000, false, true) || !fixedText(p.Plan, MaxPlanBytes, true, true) || !validTodos(p.Todos) || len(p.Phases) > MaxPhases {
			return false
		}
		total := len(p.Todos)
		for _, phase := range p.Phases {
			if !fixedText(phase.Name, 256, true, false) || !validTodos(phase.Todos) {
				return false
			}
			total += len(phase.Todos)
		}
		return total <= MaxTodos*2 // The root list may also be grouped into phases.
	}
	return false
}

// ParamsDigest binds the normalized, closed native payload. It deliberately
// does not claim to hash the original raw JSON whitespace or property order.
func ParamsDigest(r CursorInputRequest) (string, error) {
	if !validPayload(r) {
		return "", fmt.Errorf("invalid Cursor input payload")
	}
	raw, err := json.Marshal(struct {
		Method    string             `json:"method"`
		Questions *CursorAskQuestion `json:"questions,omitempty"`
		Plan      *CursorCreatePlan  `json:"plan,omitempty"`
	}{r.Method, r.Questions, r.Plan})
	if err != nil {
		return "", err
	}
	if len(raw) > MaxPayloadBytes {
		return "", fmt.Errorf("Cursor input payload exceeds its aggregate limit")
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func ValidateRequest(r CursorInputRequest) error {
	expected, err := ParamsDigest(r)
	if err != nil || expected != r.ParamsSHA256 || !identifier(r.SessionID) || ValidateRPCRequestID(r.RequestID) != nil || ValidateRPCRequestID(r.PromptID) != nil || !hash.MatchString(r.ConfigurationSHA256) {
		return fmt.Errorf("invalid original Cursor input source")
	}
	return nil
}
func ValidateEvidence(e CursorInputEvidence) error {
	kindOK := e.Method == AskQuestionMethod && e.Kind == "questions" && e.QuestionCount >= 1 && e.QuestionCount <= MaxQuestions || e.Method == CreatePlanMethod && e.Kind == "plan" && e.QuestionCount == 0
	if !identifier(e.InputID) || !identifier(e.SessionID) || !kindOK || ValidateRPCRequestID(e.RequestID) != nil || ValidateRPCRequestID(e.PromptID) != nil || !hash.MatchString(e.ConfigurationSHA256) || !hash.MatchString(e.ParamsSHA256) {
		return fmt.Errorf("invalid Cursor input evidence")
	}
	if e.Observation != nil && (delivery.Validate(*e.Observation) != nil || e.Observation.ParamsSHA256 != e.ParamsSHA256 || e.Observation.RequestResolved) {
		return fmt.Errorf("invalid Cursor input delivery fact")
	}
	return nil
}
