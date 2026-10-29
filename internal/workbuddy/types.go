// Package workbuddy implements only the documented cloud task HTTP protocol.
// It does not authorize a submission, retry an uncertain effect, or interpret a
// vendor status as Suma test, candidate, acceptance, or deployment evidence.
package workbuddy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	defaultOrigin    = "https://www.workbuddy.cn"
	tasksPath        = "/openapi/v2/tasks"
	maxResponseBytes = 1 << 20
	maxPromptBytes   = 256 << 10
)

type Config struct {
	BaseURL    string       `json:"base_url"`
	Token      string       `json:"-"`
	HTTPClient *http.Client `json:"-"`
}

// Format prevents Go's %#v fallback from exposing credentials, including when
// a Config is accidentally attached to a diagnostic.
func (Config) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddy.Config{credentials:redacted}")
}

type CreateRequest struct {
	Prompt string `json:"prompt"`
	Name   string `json:"name,omitempty"`
}

type Task struct {
	ID        string `json:"task_id"`
	Status    string `json:"status"`
	Name      string `json:"name,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}
type Pagination struct {
	Page  int `json:"page"`
	Size  int `json:"size"`
	Total int `json:"total"`
}
type TaskPage struct {
	Tasks      []Task     `json:"tasks"`
	Total      int        `json:"total"`
	Pagination Pagination `json:"pagination"`
}

// Error contains only local classifications and an HTTP status code. No vendor
// body, transport error, request URL, prompt or token is retained or wrapped.
type Error struct {
	Code           string `json:"code"`
	StatusCode     int    `json:"status_code,omitempty"`
	OutcomeUnknown bool   `json:"outcome_unknown"`
}

func (e *Error) Error() string {
	switch e.Code {
	case "invalid_config":
		return "WorkBuddy client configuration is invalid"
	case "invalid_request":
		return "WorkBuddy task request is invalid"
	case "cancelled":
		return "WorkBuddy request was cancelled before submission"
	case "transport":
		return "WorkBuddy request did not return a verified response"
	case "http_status":
		return "WorkBuddy returned an unexpected HTTP status"
	default:
		return "WorkBuddy response is invalid"
	}
}
func localError(code string) error { return &Error{Code: code} }

func validText(s string, max int, multiline bool) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t')) {
			return false
		}
	}
	return true
}
func validTaskID(id string) bool {
	if len(id) == 0 || len(id) > 200 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
func encodeCreateRequest(req CreateRequest) ([]byte, error) {
	if len(req.Prompt) > maxPromptBytes || strings.TrimSpace(req.Prompt) == "" || !validText(req.Prompt, maxPromptBytes, true) || !validText(req.Name, 512, false) || req.Name != "" && strings.TrimSpace(req.Name) == "" {
		return nil, localError("invalid_request")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, localError("invalid_request")
	}
	return raw, nil
}

// CreateRequestDigest hashes exactly the validated bytes used by CreateTask.
// It is a local checkpoint binding, not a provider idempotency key.
func CreateRequestDigest(req CreateRequest) (string, error) {
	raw, err := encodeCreateRequest(req)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
