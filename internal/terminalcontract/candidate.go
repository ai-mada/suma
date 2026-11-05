package terminalcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	nwc "github.com/ai-mada/suma/internal/nativeworkspacecontract"
)

const CandidateConfigKey = "terminal_candidate_source"

var candidateOriginID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

// TerminalCandidateWorkBuddyOrigin fixes the cloud attempt that prompted the
// local continuation. Candidate bytes still come from Original's terminal.
type TerminalCandidateWorkBuddyOrigin struct {
	RunID            string `json:"run_id"`
	RunVersion       int    `json:"run_version"`
	NodeID           string `json:"node_id"`
	Attempt          int    `json:"attempt"`
	Fence            int64  `json:"fence"`
	AssignmentSHA256 string `json:"assignment_sha256"`
}

func (o TerminalCandidateWorkBuddyOrigin) Valid() bool {
	return candidateOriginID.MatchString(o.RunID) && o.NodeID == "workbuddy-cloud" &&
		o.RunVersion > 0 && o.Attempt > 0 && o.Fence > 0 && cc.ValidHash(o.AssignmentSHA256)
}

// TerminalCandidateSource names a stopped original attempt. Filesystem paths
// are deliberately absent: only the original administrator cache may resolve it.
type TerminalCandidateSource struct {
	SessionID       string                            `json:"session_id"`
	SessionVersion  int                               `json:"session_version"`
	Original        cc.AssignmentRef                  `json:"original"`
	BaseCommit      string                            `json:"base_commit"`
	AdminBaseCommit string                            `json:"admin_base_commit"`
	CodeBaseline    *ccmp.CodeBaselineBinding         `json:"code_baseline,omitempty"`
	SnapshotSHA256  string                            `json:"snapshot_sha256"`
	PlanID          string                            `json:"plan_id"`
	WorkBuddyOrigin *TerminalCandidateWorkBuddyOrigin `json:"workbuddy_origin,omitempty" wire:"nonnull"`
}

func (s TerminalCandidateSource) Valid() bool {
	if s.SessionID == "" || len(s.SessionID) > 64 || s.SessionVersion < 1 || !s.Original.Valid() || s.Original.NodeID != "terminal" || s.Original.Attempt != 1 || !cc.ValidCommit(s.BaseCommit) || !cc.ValidCommit(s.AdminBaseCommit) || !cc.ValidHash(s.SnapshotSHA256) || s.PlanID == "" || len(s.PlanID) > 64 {
		return false
	}
	if s.WorkBuddyOrigin != nil && (!s.WorkBuddyOrigin.Valid() || s.WorkBuddyOrigin.RunID == s.Original.RunID) {
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
