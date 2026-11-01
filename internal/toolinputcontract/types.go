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

	cc "github.com/ai-mada/suma/internal/codexcontract"
)

type ToolInputRequest struct {
	IdempotencyKey string                   `json:"idempotency_key"`
	Protocol       string                   `json:"protocol"`
	Codex          cc.CodexUserInputRequest `json:"codex"`
}

type ToolInputAnswer struct {
	Answers      []cc.CodexUserInputSelection `json:"answers"`
	AnswerSHA256 string                       `json:"answer_sha256"`
	ActorID      string                       `json:"actor_id"`
	AnsweredAt   string                       `json:"answered_at"`
}

type ToolInput struct {
	ID             string                        `json:"id"`
	ProjectID      string                        `json:"project_id"`
	RunID          string                        `json:"run_id"`
	NodeID         string                        `json:"node_id"`
	Attempt        int                           `json:"attempt"`
	Fence          int64                         `json:"fence"`
	DispatchID     string                        `json:"dispatch_id"`
	WorkerID       string                        `json:"worker_id"`
	ProfileSHA256  string                        `json:"profile_sha256"`
	Version        int64                         `json:"version"`
	State          string                        `json:"state"`
	Request        ToolInputRequest              `json:"request"`
	Answer         *ToolInputAnswer              `json:"answer,omitempty"`
	Observation    *cc.CodexUserInputObservation `json:"observation,omitempty"`
	CreatedAt      string                        `json:"created_at"`
	ExpiresAt      string                        `json:"expires_at"`
	AllowedActions []string                      `json:"allowed_actions"`
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
	Answers         []cc.CodexUserInputSelection `json:"answers"`
}

func textID(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && utf8.ValidString(value) && strings.TrimSpace(value) == value && strings.IndexFunc(value, unicode.IsControl) < 0
}

func ValidateRequest(r ToolInputRequest) error {
	if !textID(r.IdempotencyKey, 128) || len(r.IdempotencyKey) < 8 || r.Protocol != cc.Protocol || cc.ValidateUserInputRequest(r.Codex) != nil {
		return fmt.Errorf("invalid tool input request")
	}
	return nil
}

func ValidateAnswer(r ToolInputRequest, a ToolInputAnswer) error {
	expected, err := cc.UserInputAnswerDigest(r.Codex, a.Answers)
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
		if cc.ValidateUserInputObservation(*o) != nil || o.ParamsSHA256 != v.Request.Codex.ParamsSHA256 || o.AnswerObserved && (v.Answer == nil || o.AnswerSHA256 != v.Answer.AnswerSHA256) {
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
