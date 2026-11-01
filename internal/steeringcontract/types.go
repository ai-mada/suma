// Package steeringcontract describes explicit human discussion messages sent
// into one attested live turn. It never creates tasks or grants tool access.
package steeringcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type SteeringSession struct {
	ID             string                 `json:"id"`
	ProjectID      string                 `json:"project_id"`
	TaskID         string                 `json:"task_id"`
	RunID          string                 `json:"run_id"`
	NodeID         string                 `json:"node_id"`
	Attempt        int                    `json:"attempt"`
	Fence          int64                  `json:"fence"`
	WorkerID       string                 `json:"worker_id"`
	DispatchID     string                 `json:"dispatch_id"`
	ProfileSHA256  string                 `json:"profile_sha256"`
	Source         cc.CodexSteeringSource `json:"source"`
	Version        int64                  `json:"version"`
	State          string                 `json:"state"`
	OpenedAt       string                 `json:"opened_at"`
	ExpiresAt      string                 `json:"expires_at"`
	AllowedActions []string               `json:"allowed_actions"`
}

type SteeringMessage struct {
	ID         string `json:"id"`
	Sequence   int64  `json:"sequence"`
	AuthorID   string `json:"author_id"`
	Body       string `json:"body"`
	BodySHA256 string `json:"body_sha256"`
}

type SteeringInstruction struct {
	ID          string                       `json:"id"`
	SessionID   string                       `json:"session_id"`
	ProjectID   string                       `json:"project_id"`
	TaskID      string                       `json:"task_id"`
	RunID       string                       `json:"run_id"`
	NodeID      string                       `json:"node_id"`
	Attempt     int                          `json:"attempt"`
	Fence       int64                        `json:"fence"`
	Version     int64                        `json:"version"`
	Message     SteeringMessage              `json:"message"`
	ActorID     string                       `json:"actor_id"`
	CreatedAt   string                       `json:"created_at"`
	ExpiresAt   string                       `json:"expires_at"`
	Observation *cc.CodexSteeringObservation `json:"observation,omitempty"`
}

type SteeringContext struct {
	TaskID       string                `json:"task_id"`
	TaskVersion  int64                 `json:"task_version"`
	Message      SteeringMessage       `json:"message"`
	Targets      []SteeringSession     `json:"targets"`
	Instructions []SteeringInstruction `json:"instructions"`
	CanSend      bool                  `json:"can_send"`
	Reason       string                `json:"reason,omitempty"`
}

type SteeringCreateRequest struct {
	ProjectID              string `json:"project_id"`
	TaskID                 string `json:"task_id"`
	MessageID              string `json:"message_id"`
	ExpectedTaskVersion    int64  `json:"expected_task_version"`
	SessionID              string `json:"session_id"`
	ExpectedSessionVersion int64  `json:"expected_session_version"`
	IdempotencyKey         string `json:"idempotency_key"`
}

type SteeringPoll struct {
	Session      SteeringSession       `json:"session"`
	Instructions []SteeringInstruction `json:"instructions"`
}

// A regular workflow need not belong to a managed task. This explicit outcome
// disables this optional collaboration channel without hiding auth failures.
type SteeringOpenResult struct {
	Session *SteeringSession `json:"session,omitempty" wire:"nonnull"`
	Reason  string           `json:"reason,omitempty"`
}

func validID(v string, max int) bool {
	return v != "" && len(v) <= max && utf8.ValidString(v) && strings.TrimSpace(v) == v && strings.IndexFunc(v, unicode.IsControl) < 0
}
func validHash(v string) bool {
	_, err := hex.DecodeString(v)
	return err == nil && len(v) == 64 && strings.ToLower(v) == v
}
func validTime(v string) bool { _, err := time.Parse(time.RFC3339Nano, v); return err == nil }
func MessageDigest(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func ValidateSession(v SteeringSession) error {
	if !validID(v.ID, 100) || !validID(v.ProjectID, 64) || !validID(v.TaskID, 64) || !validID(v.RunID, 100) || !validID(v.NodeID, 150) || !validID(v.WorkerID, 100) || v.Attempt < 1 || v.Fence < 1 || v.Version < 1 || v.Version > cc.MaxSafeInteger || !validHash(v.ProfileSHA256) || cc.ValidateSteeringSource(v.Source) != nil || (v.State != "active" && v.State != "closed") || !validTime(v.OpenedAt) || !validTime(v.ExpiresAt) || v.AllowedActions == nil || len(v.AllowedActions) > 1 {
		return fmt.Errorf("invalid exact live steering session")
	}
	for _, a := range v.AllowedActions {
		if a != "send" || v.State != "active" {
			return fmt.Errorf("invalid live steering action")
		}
	}
	return nil
}

func ValidateMessage(v SteeringMessage) error {
	if !validID(v.ID, 100) || v.Sequence < 1 || v.Sequence > cc.MaxSafeInteger || !validID(v.AuthorID, 100) || !cc.ValidSteeringText(v.Body) || !validHash(v.BodySHA256) || v.BodySHA256 != MessageDigest(v.Body) {
		return fmt.Errorf("invalid original discussion steering message")
	}
	return nil
}

func ValidateInstruction(v SteeringInstruction) error {
	if !validID(v.ID, 100) || !validID(v.SessionID, 100) || !validID(v.ProjectID, 64) || !validID(v.TaskID, 64) || !validID(v.RunID, 100) || !validID(v.NodeID, 150) || v.Attempt < 1 || v.Fence < 1 || v.Version < 1 || v.Version > cc.MaxSafeInteger || ValidateMessage(v.Message) != nil || v.ActorID != v.Message.AuthorID || !validTime(v.CreatedAt) || !validTime(v.ExpiresAt) || v.Observation != nil && cc.ValidateSteeringObservation(*v.Observation) != nil {
		return fmt.Errorf("invalid exact steering instruction")
	}
	return nil
}
