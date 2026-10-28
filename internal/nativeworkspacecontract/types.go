// Package nativeworkspacecontract holds non-secret execution evidence shared by
// the Runner, control plane and generated clients. It grants no authority.
package nativeworkspacecontract

import wo "github.com/ai-mada/suma/internal/workspaceoperationcontract"

const Version = "attempt-workspace-v1"

type NativeWorkspaceFileEffect struct {
	Path           string `json:"path"`
	OperationID    string `json:"operation_id"`
	PreviousSHA256 string `json:"previous_sha256,omitempty"`
	SHA256         string `json:"sha256,omitempty"`
	Removed        bool   `json:"removed"`
	Generation     int    `json:"generation"`
}

type NativeWorkspaceCheckResult struct {
	OperationID     string `json:"operation_id"`
	CheckID         string `json:"check_id"`
	ProfileSHA256   string `json:"profile_sha256"`
	Generation      int    `json:"generation"`
	WorkspaceSHA256 string `json:"workspace_sha256"`
	ExitCode        int    `json:"exit_code"`
	Stopped         bool   `json:"stopped"`
	OutcomeUnknown  bool   `json:"outcome_unknown"`
	Output          string `json:"output"`
	OutputTruncated bool   `json:"output_truncated"`
}

type NativeWorkspaceEvidence struct {
	SchemaVersion  int                          `json:"schema_version"`
	PolicySHA256   string                       `json:"policy_sha256"`
	Generation     int                          `json:"generation"`
	State          string                       `json:"state"`
	Files          []NativeWorkspaceFileEffect  `json:"files"`
	Checks         []NativeWorkspaceCheckResult `json:"checks"`
	Seal           *NativeWorkspaceSeal         `json:"seal,omitempty"`
	OperationTrace *wo.Coverage                 `json:"operation_trace,omitempty" wire:"nonnull"`
}

// Seal describes a local candidate commit, never a merge, formal test or human
// acceptance. The operation persists before Git effects and is recoverable.
type NativeWorkspaceSeal struct {
	SchemaVersion  int    `json:"schema_version"`
	OperationID    string `json:"operation_id"`
	PolicySHA256   string `json:"policy_sha256"`
	State          string `json:"state"`
	PreviousCommit string `json:"previous_commit"`
	Commit         string `json:"commit,omitempty"`
	Tree           string `json:"tree,omitempty"`
	ChangedPaths   int    `json:"changed_paths"`
}
