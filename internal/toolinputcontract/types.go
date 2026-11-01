// Package toolinputcontract owns reviewed human input during an exact live
// execution. These records cannot grant command, file or session permissions.
package toolinputcontract

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	cl "github.com/ai-mada/suma/internal/claudecontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
)

type ToolInputRequest struct {
	IdempotencyKey string                    `json:"idempotency_key"`
	Protocol       string                    `json:"protocol"`
	Codex          *cc.CodexUserInputRequest `json:"codex,omitempty" wire:"nonnull"`
	Cursor         *cu.CursorInputRequest    `json:"cursor,omitempty" wire:"nonnull"`
	Claude         *cl.InputRequest          `json:"claude,omitempty" wire:"nonnull"`
}

type ToolInputAnswer struct {
	Answers      []cc.CodexUserInputSelection `json:"answers,omitempty"`
	Cursor       *cu.CursorInputDecision      `json:"cursor,omitempty" wire:"nonnull"`
	Claude       *cl.InputDecision            `json:"claude,omitempty" wire:"nonnull"`
	AnswerSHA256 string                       `json:"answer_sha256"`
	ActorID      string                       `json:"actor_id"`
	AnsweredAt   string                       `json:"answered_at"`
}

type ToolInput struct {
	ID             string                `json:"id"`
	ProjectID      string                `json:"project_id"`
	RunID          string                `json:"run_id"`
	NodeID         string                `json:"node_id"`
	Attempt        int                   `json:"attempt"`
	Fence          int64                 `json:"fence"`
	DispatchID     string                `json:"dispatch_id"`
	WorkerID       string                `json:"worker_id"`
	ProfileSHA256  string                `json:"profile_sha256"`
	Version        int64                 `json:"version"`
	State          string                `json:"state"`
	Request        ToolInputRequest      `json:"request"`
	Answer         *ToolInputAnswer      `json:"answer,omitempty"`
	Observation    *delivery.Observation `json:"observation,omitempty"`
	CreatedAt      string                `json:"created_at"`
	ExpiresAt      string                `json:"expires_at"`
	AllowedActions []string              `json:"allowed_actions"`
}

type ToolInputList struct {
	// Protocol is the current protected node/profile capability, not liveness.
	// Empty means no current capability; authorized history may still be read.
	Protocol string      `json:"protocol"`
	Items    []ToolInput `json:"items"`
}

type ToolInputAnswerRequest struct {
	ProjectID       string                       `json:"project_id"`
	ExpectedVersion int64                        `json:"expected_version"`
	IdempotencyKey  string                       `json:"idempotency_key"`
	Answers         []cc.CodexUserInputSelection `json:"answers,omitempty"`
	Cursor          *cu.CursorInputDecision      `json:"cursor,omitempty" wire:"nonnull"`
	Claude          *cl.InputDecision            `json:"claude,omitempty" wire:"nonnull"`
}

func textID(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && utf8.ValidString(value) && strings.TrimSpace(value) == value && strings.IndexFunc(value, unicode.IsControl) < 0
}

func ValidateRequest(r ToolInputRequest) error {
	validNative := r.Protocol == cc.Protocol && r.Codex != nil && r.Cursor == nil && r.Claude == nil && cc.ValidateUserInputRequest(*r.Codex) == nil || r.Protocol == ac.Protocol && r.Cursor != nil && r.Codex == nil && r.Claude == nil && cu.ValidateRequest(*r.Cursor) == nil || r.Protocol == cl.Protocol && r.Claude != nil && r.Codex == nil && r.Cursor == nil && cl.ValidateInputRequest(*r.Claude) == nil
	if !textID(r.IdempotencyKey, 128) || len(r.IdempotencyKey) < 8 || !validNative {
		return fmt.Errorf("invalid tool input request")
	}
	return nil
}

func ValidateAnswer(r ToolInputRequest, a ToolInputAnswer) error {
	expected, err := AnswerDigest(r, a.Answers, a.Cursor, a.Claude)
	if ValidateRequest(r) != nil || err != nil || expected != a.AnswerSHA256 || !textID(a.ActorID, 100) {
		return fmt.Errorf("invalid original tool input answer")
	}
	if _, err = time.Parse(time.RFC3339Nano, a.AnsweredAt); err != nil {
		return fmt.Errorf("invalid tool input answer time")
	}
	return nil
}

func ValidateInput(v ToolInput) error {
	_, digestError := hex.DecodeString(v.ProfileSHA256)
	if !textID(v.ID, 100) || !textID(v.ProjectID, 64) || !textID(v.RunID, 100) || !textID(v.NodeID, 150) || !textID(v.WorkerID, 100) || v.Attempt < 1 || v.Fence < 1 || v.Version < 1 || v.Version > cc.MaxSafeInteger || len(v.ProfileSHA256) != 64 || digestError != nil || strings.ToLower(v.ProfileSHA256) != v.ProfileSHA256 || ValidateRequest(v.Request) != nil || v.AllowedActions == nil || len(v.AllowedActions) > 1 {
		return fmt.Errorf("invalid scoped tool input record")
	}
	switch v.State {
	case "pending", "answered", "closed", "expired":
	default:
		return fmt.Errorf("invalid tool input state")
	}
	for _, stamp := range []string{v.CreatedAt, v.ExpiresAt} {
		if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
			return fmt.Errorf("invalid tool input record time")
		}
	}
	if v.Answer != nil && ValidateAnswer(v.Request, *v.Answer) != nil || v.State == "answered" && v.Answer == nil || v.State == "pending" && v.Answer != nil {
		return fmt.Errorf("tool input answer differs from its original request")
	}
	if v.Observation != nil {
		o := v.Observation
		if delivery.Validate(*o) != nil || o.ParamsSHA256 != ParamsSHA256(v.Request) || (v.Request.Cursor != nil || v.Request.Claude != nil) && o.RequestResolved || o.AnswerObserved && (v.Answer == nil || o.AnswerSHA256 != v.Answer.AnswerSHA256) {
			return fmt.Errorf("tool input observation differs from its original answer")
		}
	}
	for _, action := range v.AllowedActions {
		if action != "answer" || v.State != "pending" {
			return fmt.Errorf("tool input action cannot grant authority")
		}
	}
	return nil
}

// AnswerDigest dispatches only to the exact source runtime. The Cursor native
// response additionally binds the complete original question or plan digest.
func AnswerDigest(r ToolInputRequest, answers []cc.CodexUserInputSelection, cursor *cu.CursorInputDecision, claude *cl.InputDecision) (string, error) {
	if ValidateRequest(r) != nil {
		return "", fmt.Errorf("invalid tool input source")
	}
	if r.Codex != nil && cursor == nil && claude == nil {
		return cc.UserInputAnswerDigest(*r.Codex, answers)
	}
	if r.Cursor != nil && cursor != nil && claude == nil && len(answers) == 0 {
		return cu.AnswerDigest(*r.Cursor, *cursor)
	}
	if r.Claude != nil && claude != nil && cursor == nil && len(answers) == 0 {
		return cl.InputDecisionDigest(*r.Claude, *claude)
	}
	return "", fmt.Errorf("tool input answer uses another runtime")
}
func ParamsSHA256(r ToolInputRequest) string {
	if r.Codex != nil && r.Cursor == nil && r.Claude == nil {
		return r.Codex.ParamsSHA256
	}
	if r.Cursor != nil && r.Codex == nil && r.Claude == nil {
		return r.Cursor.ParamsSHA256
	}
	if r.Claude != nil && r.Codex == nil && r.Cursor == nil {
		return r.Claude.ParamsSHA256
	}
	return ""
}

// These projections translate real protocol facts. Cursor has no Codex turn
// or request-resolved acknowledgement, and does not use the Codex projection.
func FromCodexObservation(o cc.CodexUserInputObservation) delivery.Observation {
	return delivery.Observation{Sequence: o.Sequence, ParamsSHA256: o.ParamsSHA256, AnswerSHA256: o.AnswerSHA256, AnswerObserved: o.AnswerObserved, ReplyStarted: o.ReplyStarted, ReplyWritten: o.ReplyWritten, RequestResolved: o.RequestResolved, ExecutionClosed: o.TurnClosed, FailureCode: o.FailureCode}
}
func ToCodexObservation(o delivery.Observation) cc.CodexUserInputObservation {
	return cc.CodexUserInputObservation{Sequence: o.Sequence, ParamsSHA256: o.ParamsSHA256, AnswerSHA256: o.AnswerSHA256, AnswerObserved: o.AnswerObserved, ReplyStarted: o.ReplyStarted, ReplyWritten: o.ReplyWritten, RequestResolved: o.RequestResolved, TurnClosed: o.ExecutionClosed, FailureCode: o.FailureCode}
}
