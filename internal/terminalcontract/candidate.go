package terminalcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	nwc "github.com/ai-mada/suma/internal/nativeworkspacecontract"
)

const CandidateConfigKey = "terminal_candidate_source"

// TerminalCandidateSource names a stopped original attempt. Filesystem paths
// are deliberately absent: only the original administrator cache may resolve it.
type TerminalCandidateSource struct {
	SessionID       string                    `json:"session_id"`
	SessionVersion  int                       `json:"session_version"`
	Original        cc.AssignmentRef          `json:"original"`
	BaseCommit      string                    `json:"base_commit"`
	AdminBaseCommit string                    `json:"admin_base_commit"`
	CodeBaseline    *ccmp.CodeBaselineBinding `json:"code_baseline,omitempty"`
	SnapshotSHA256  string                    `json:"snapshot_sha256"`
	PlanID          string                    `json:"plan_id"`
}

func (s TerminalCandidateSource) Valid() bool {
	if s.SessionID == "" || len(s.SessionID) > 64 || s.SessionVersion < 1 || !s.Original.Valid() || s.Original.NodeID != "terminal" || s.Original.Attempt != 1 || !cc.ValidCommit(s.BaseCommit) || !cc.ValidCommit(s.AdminBaseCommit) || !cc.ValidHash(s.SnapshotSHA256) || s.PlanID == "" || len(s.PlanID) > 64 {
		return false
	}
	if s.CodeBaseline == nil {
		return s.BaseCommit == s.AdminBaseCommit
	}
	return ccmp.ValidateCodeBaselineBinding(*s.CodeBaseline) == nil && s.CodeBaseline.Source.HeadCommit == s.BaseCommit && s.CodeBaseline.Source.BundleBase() == s.AdminBaseCommit
}

func DecodeCandidateSource(value any) (*TerminalCandidateSource, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 8192 {
		return nil, fmt.Errorf("Invalid stopped terminal candidate source")
	}
	var source TerminalCandidateSource
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&source) != nil || !source.Valid() {
		return nil, fmt.Errorf("Invalid stopped terminal candidate source")
	}
	return &source, nil
}

// The check remains ordinary fixed-command evidence. Seal records the actual
// parent host operation; it is not a terminal-output or human acceptance claim.
type TerminalCandidateEvidence struct {
	SchemaVersion  int                     `json:"schema_version"`
	Source         TerminalCandidateSource `json:"source"`
	SnapshotSHA256 string                  `json:"snapshot_sha256"`
	FileCount      int                     `json:"file_count"`
	Seal           nwc.NativeWorkspaceSeal `json:"seal"`
}
