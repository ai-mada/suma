// Package developmentcontract defines an explicitly reviewed same-task code
// input. It carries no acceptance, filesystem, model or execution authority.
package developmentcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

const ContinuationProtocol = "suma-task-development-continuation-v1"

// DevelopmentContinuationSource is an intent, resolved by the control plane.
// Workers, transfer IDs and evidence are never caller-selected authority.
type DevelopmentContinuationSource struct {
	RunID              string `json:"run_id"`
	NodeID             string `json:"node_id"`
	Attempt            int    `json:"attempt"`
	Fence              int64  `json:"fence"`
	ExpectedRunVersion int    `json:"expected_run_version"`
}

type DevelopmentContinuationBinding struct {
	Protocol          string     `json:"protocol"`
	ID                string     `json:"id"`
	ProjectID         string     `json:"project_id"`
	TaskID            string     `json:"task_id"`
	TaskVersion       int        `json:"task_version"`
	SourceTaskRunID   string     `json:"source_task_run_id"`
	SourceTaskVersion int        `json:"source_task_version"`
	SourceRunVersion  int        `json:"source_run_version"`
	Source            cc.Binding `json:"source"`
	BindingSHA256     string     `json:"binding_sha256"`
}

func validID(value string, limit int) bool {
	return value != "" && len(value) <= limit && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "/\\") && strings.IndexFunc(value, unicode.IsControl) < 0
}

func ValidateSource(s DevelopmentContinuationSource) error {
	if !validID(s.RunID, 100) || !validID(s.NodeID, 150) || s.Attempt < 1 || s.Fence < 1 || s.ExpectedRunVersion < 1 {
		return fmt.Errorf("invalid development continuation source")
	}
	return nil
}

func validateBindingFields(b DevelopmentContinuationBinding) error {
	if b.Protocol != ContinuationProtocol || !validID(b.ID, 64) || !validID(b.ProjectID, 64) || !validID(b.TaskID, 64) || b.TaskVersion < 1 || b.SourceTaskVersion < 1 || b.SourceTaskVersion > b.TaskVersion || b.SourceRunVersion < 1 || !b.Source.ValidSealed() || b.ProjectID != b.Source.ProjectID || b.SourceTaskRunID != b.Source.Source.RunID {
		return fmt.Errorf("invalid development continuation binding")
	}
	return nil
}

// BindingDigest fixes every field except the digest itself. It does not prove
// the current task relationship; the control plane rechecks that separately.
func BindingDigest(b DevelopmentContinuationBinding) (string, error) {
	if err := validateBindingFields(b); err != nil {
		return "", err
	}
	b.BindingSHA256 = ""
	raw, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func ValidateBinding(b DevelopmentContinuationBinding) error {
	expected, err := BindingDigest(b)
	if err != nil || !cc.ValidHash(b.BindingSHA256) || b.BindingSHA256 != expected {
		return fmt.Errorf("invalid development continuation digest")
	}
	return nil
}
