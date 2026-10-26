package platform

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type Record struct {
	// Read-only provenance from the diagnosis adoption table; never from Data.
	FailureDiagnosisAdoption *FailureDiagnosisAdoption `json:"failure_diagnosis_adoption,omitempty"`
	ID                       string                    `json:"id"`
	TenantID                 string                    `json:"tenant_id"`
	ProjectID                string                    `json:"project_id"`
	Version                  int                       `json:"version"`
	Title                    string                    `json:"title"`
	Name                     string                    `json:"name,omitempty"`
	Description              string                    `json:"description"`
	Status                   string                    `json:"status"`
	Priority                 string                    `json:"priority"`
	OwnerID                  string                    `json:"owner_id"`
	RequirementID            string                    `json:"requirement_id"`
	RunID                    string                    `json:"run_id,omitempty"`
	DependsOn                []string                  `json:"depends_on"`
	AcceptanceCriteria       []string                  `json:"acceptance_criteria"`
	Kind                     string                    `json:"kind,omitempty"`
	CreatedAt                string                    `json:"created_at"`
	UpdatedAt                string                    `json:"updated_at"`
	Data                     map[string]any            `json:"data"`
}

type Principal struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"`
}

type APIError struct {
	Status        int
	Code, Message string
	Details       any
}

func (e *APIError) Error() string   { return e.Message }
func invalid(message string) error  { return &APIError{400, "invalid_input", message, nil} }
func conflict(message string) error { return &APIError{409, "version_conflict", message, nil} }
func forbidden() error              { return &APIError{403, "forbidden", "当前身份没有此操作权限", nil} }
func missing() error                { return &APIError{404, "not_found", "记录不存在或不可访问", nil} }
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func now() string                           { return time.Now().UTC().Format(time.RFC3339Nano) }
func stringify(v any) string                { b, _ := json.Marshal(v); return string(b) }
func str(m map[string]any, k string) string { v, _ := m[k].(string); return v }
func number(m map[string]any, k string) int {
	switch v := m[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}
func stringsAt(m map[string]any, k string) []string {
	out := []string{}
	if a, ok := m[k].([]any); ok {
		for _, v := range a {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}
func requireTitle(s string) error {
	if len(s) == 0 || len(s) > 500 {
		return invalid("标题长度应为 1–500 个字节")
	}
	return nil
}
func errf(format string, args ...any) error { return invalid(fmt.Sprintf(format, args...)) }
