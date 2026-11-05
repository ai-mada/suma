package releasecontract

import (
	"encoding/json"
	"errors"
	"strings"
)

// ApplicationBuildConfigKey is a protected source expectation, not a command or
// an authorization grant. The control plane verifies it against registration.
const ApplicationBuildConfigKey = "application_build"

// ApplicationBuildBasis binds a build to a single registered application source.
// A workspace root proves the working directory and its Git tree, not isolation
// from sibling paths that the administrator's build program may read.
type ApplicationBuildBasis struct {
	ApplicationID string `json:"application_id"`
	VersionID     string `json:"version_id"`
	VersionSHA256 string `json:"version_sha256"`
	ScopeID       string `json:"scope_id"`
	ScopeRoot     string `json:"scope_root"`
	ScopeTreeOID  string `json:"scope_tree_oid"`
	SourceCommit  string `json:"source_commit"`
	ReceiptSHA256 string `json:"receipt_sha256"`
}

// DecodeApplicationBuild rejects any incomplete or extra declared fields. An
// absent field selects an ordinary project build and confers no application proof.
func DecodeApplicationBuild(config map[string]any) (*ApplicationBuildBasis, error) {
	value, declared := config[ApplicationBuildConfigKey]
	if !declared {
		return nil, nil
	}
	invalid := errors.New("invalid application build basis")
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 2048 {
		return nil, invalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 8 {
		return nil, invalid
	}
	var basis ApplicationBuildBasis
	names := map[string]*string{
		"application_id": &basis.ApplicationID, "version_id": &basis.VersionID,
		"version_sha256": &basis.VersionSHA256, "scope_id": &basis.ScopeID,
		"scope_root": &basis.ScopeRoot, "scope_tree_oid": &basis.ScopeTreeOID,
		"source_commit": &basis.SourceCommit, "receipt_sha256": &basis.ReceiptSHA256,
	}
	for key, destination := range names {
		data := fields[key]
		if len(data) == 0 || data[0] != '"' || json.Unmarshal(data, destination) != nil {
			return nil, invalid
		}
	}
	if !ValidProfileID(basis.ApplicationID) || !ValidProfileID(basis.VersionID) || !ValidProfileID(basis.ScopeID) ||
		!ValidDigest(basis.VersionSHA256) || !ValidDigest(basis.ReceiptSHA256) || !ValidCommit(basis.SourceCommit) ||
		!ValidCommit(basis.ScopeTreeOID) || len(basis.ScopeTreeOID) != len(basis.SourceCommit) {
		return nil, invalid
	}
	if basis.ScopeRoot != "" {
		if !ValidPath(basis.ScopeRoot) {
			return nil, invalid
		}
		for _, segment := range strings.Split(basis.ScopeRoot, "/") {
			if strings.EqualFold(segment, ".git") {
				return nil, invalid
			}
		}
	}
	return &basis, nil
}
