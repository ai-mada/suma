package platform

import (
	"strings"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

// These helpers validate public shapes and exact receipts, never authority.
// Current authorization, freshness and CAS remain inside the original domain
// transaction; a well-formed acknowledgement cannot permit another command.
func ValidToolProfileReplacementRequest(req ToolProfileReplacementRequest, originalID string) bool {
	if !tc.ValidCommandID(originalID) || !tc.ValidCommandID(req.ProjectID) || !ingressVersion(req.ExpectedVersion) ||
		!tc.ValidCommandID(req.WorkerID) || tc.ValidateProfile(req.Profile, true) != nil || !tic.Operation(req.IdempotencyKey) {
		return false
	}
	if req.Reuse != nil {
		return req.Name == "" && tc.ValidCommandID(req.Reuse.ProfileID) && req.Reuse.ProfileID != originalID && ingressVersion(req.Reuse.ExpectedVersion)
	}
	return req.Name != "" && strings.TrimSpace(req.Name) == req.Name && ingressText(req.Name, 200)
}

func replacementProfileSnapshotValid(p ToolProfile, project string) bool {
	if !tc.ValidCommandID(p.ID) || p.ProjectID != project || !tc.ValidCommandID(p.WorkerID) || !ingressVersion(p.Version) ||
		p.Name == "" || strings.TrimSpace(p.Name) != p.Name || !ingressText(p.Name, 200) || tc.ValidateProfile(p.Profile, true) != nil ||
		(p.State != "paused" && p.State != "enabled") || (p.State == "paused" && p.Available) {
		return false
	}
	for _, stamp := range []string{p.CreatedAt, p.UpdatedAt} {
		if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
			return false
		}
	}
	return true
}

// ValidToolProfileReplacement checks the immutable relationship snapshot. It
// does not compare it with today's profiles or recompute a historical result.
func ValidToolProfileReplacement(v ToolProfileReplacement) bool {
	if !tc.ValidCommandID(v.ID) || !tc.ValidCommandID(v.ProjectID) || !tc.ValidCommandID(v.CreatedBy) ||
		!replacementProfileSnapshotValid(v.Original, v.ProjectID) || !replacementProfileSnapshotValid(v.Replacement, v.ProjectID) ||
		v.Original.ID == v.Replacement.ID || (v.PauseOriginal && v.Original.State != "paused") {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, v.CreatedAt); err != nil {
		return false
	}
	switch v.Disposition {
	case "created":
		return v.Replacement.State == "paused" && v.Replacement.Version == 1 && v.Replacement.EnabledBy == ""
	case "reused":
		return true
	}
	return false
}

// ToolProfileReplacementMatches binds a successful response to the original
// submitted intent. The operator's file/body/key, not a newly read version, is
// the recovery basis when delivery of that response is uncertain.
func ToolProfileReplacementMatches(v ToolProfileReplacement, req ToolProfileReplacementRequest, originalID string) bool {
	if !ValidToolProfileReplacementRequest(req, originalID) || !ValidToolProfileReplacement(v) ||
		v.ProjectID != req.ProjectID || v.Original.ID != originalID || v.PauseOriginal != req.PauseOriginal ||
		v.Replacement.WorkerID != req.WorkerID || !tc.EqualProfile(v.Replacement.Profile, req.Profile) {
		return false
	}
	if req.PauseOriginal {
		if v.Original.Version != req.ExpectedVersion && v.Original.Version != req.ExpectedVersion+1 {
			return false
		}
	} else if v.Original.Version != req.ExpectedVersion {
		return false
	}
	if req.Reuse == nil {
		return v.Disposition == "created" && v.Replacement.Name == req.Name
	}
	return v.Disposition == "reused" && v.Replacement.ID == req.Reuse.ProfileID && v.Replacement.Version == req.Reuse.ExpectedVersion
}
