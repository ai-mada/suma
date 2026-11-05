package repositoryintegrationcontract

import (
	"encoding/json"
	"fmt"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

const RepositorySourceExecutionKey = "repository_source_intake"

// RepositorySourceExecution pins an approved bundle in a managed producer.
// The server's plan and live lease grant access; these fields alone do not.
type RepositorySourceExecution struct {
	IntakeID     string `json:"intake_id"`
	PlanID       string `json:"plan_id"`
	TaskID       string `json:"task_id"`
	TaskVersion  int    `json:"task_version"`
	BaseCommit   string `json:"base_commit"`
	HeadCommit   string `json:"head_commit"`
	BundleSHA256 string `json:"bundle_sha256"`
	SizeBytes    int64  `json:"size_bytes"`
}

func (s RepositorySourceExecution) Valid() bool {
	return sourceIdentity.MatchString(s.IntakeID) && sourceIdentity.MatchString(s.PlanID) && sourceIdentity.MatchString(s.TaskID) &&
		s.TaskVersion > 0 && s.TaskVersion <= 2147483647 && cc.ValidCommit(s.BaseCommit) && cc.ValidCommit(s.HeadCommit) &&
		len(s.BaseCommit) == len(s.HeadCommit) && s.BaseCommit != s.HeadCommit && cc.ValidHash(s.BundleSHA256) && s.SizeBytes > 0 && s.SizeBytes <= cc.MaxBundleBytes
}

func DecodeRepositorySourceExecution(v any) (*RepositorySourceExecution, error) {
	raw, err := json.Marshal(v)
	if err != nil || len(raw) > 8192 {
		return nil, fmt.Errorf("invalid managed repository source")
	}
	var s RepositorySourceExecution
	keys := []string{"intake_id", "plan_id", "task_id", "task_version", "base_commit", "head_commit", "bundle_sha256", "size_bytes"}
	if exactRepositorySourceFields(raw, keys) != nil || json.Unmarshal(raw, &s) != nil || !s.Valid() {
		return nil, fmt.Errorf("invalid managed repository source")
	}
	return &s, nil
}
