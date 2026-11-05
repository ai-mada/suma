// Package releasecontract contains versioned wire contracts shared by the
// control plane, trusted Runner and release providers. It grants no authority.
package releasecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

const (
	StaticSiteKind         = "static_site_v1"
	MaxFiles               = 128
	MaxFileBytes     int64 = 16 << 20
	MaxArtifactBytes int64 = 64 << 20
)

type AssignmentRef struct {
	RunID    string `json:"run_id"`
	NodeID   string `json:"node_id"`
	Attempt  int    `json:"attempt"`
	Fence    int64  `json:"fence"`
	WorkerID string `json:"worker_id"`
}

func (a AssignmentRef) Valid() bool {
	return len(a.RunID) > 0 && len(a.RunID) <= 100 && len(a.NodeID) > 0 && len(a.NodeID) <= 150 && a.Attempt > 0 && a.Fence > 0 && len(a.WorkerID) > 0 && len(a.WorkerID) <= 100
}

type ArtifactFile struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}
type ArtifactManifest struct {
	SchemaVersion int            `json:"schema_version"`
	Kind          string         `json:"kind"`
	Files         []ArtifactFile `json:"files"`
}
type ArtifactEvidence struct {
	ApplicationBuild *ApplicationBuildBasis `json:"application_build,omitempty"`
	Kind             string                 `json:"kind"`
	ArtifactID       string                 `json:"artifact_id"`
	ManifestSHA256   string                 `json:"manifest_sha256"`
	SourceCommit     string                 `json:"source_commit"`
	ProfileID        string                 `json:"profile_id"`
	ProfileSHA256    string                 `json:"profile_sha256"`
}
type ArtifactBinding struct {
	ArtifactID     string           `json:"artifact_id"`
	ManifestSHA256 string           `json:"manifest_sha256"`
	SourceCommit   string           `json:"source_commit"`
	BuildRunID     string           `json:"build_run_id"`
	BuildNodeID    string           `json:"build_node_id"`
	BuildAttempt   int              `json:"build_attempt"`
	BuildFence     int64            `json:"build_fence"`
	Manifest       ArtifactManifest `json:"manifest"`
}
type ArtifactUploadRequest struct {
	ProjectID     string           `json:"project_id"`
	Assignment    AssignmentRef    `json:"assignment"`
	OperationID   string           `json:"operation_id"`
	ProfileID     string           `json:"profile_id"`
	ProfileSHA256 string           `json:"profile_sha256"`
	Manifest      ArtifactManifest `json:"manifest"`
}
type ArtifactUploadState struct {
	UploadID       string   `json:"upload_id"`
	State          string   `json:"state"`
	ManifestSHA256 string   `json:"manifest_sha256"`
	MissingBlobs   []string `json:"missing_blobs"`
}
type ArtifactSealRequest struct {
	ProjectID   string        `json:"project_id"`
	Assignment  AssignmentRef `json:"assignment"`
	OperationID string        `json:"operation_id"`
}
type ArtifactSealReceipt struct {
	ArtifactID     string `json:"artifact_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
	State          string `json:"state"`
	SizeBytes      int64  `json:"size_bytes"`
	FileCount      int    `json:"file_count"`
}
type ArtifactResolveRequest struct {
	ProjectID    string        `json:"project_id"`
	Assignment   AssignmentRef `json:"assignment"`
	OperationID  string        `json:"operation_id"`
	SourceNodeID string        `json:"source_node_id"`
}

// CommandProfile is an administrator-owned command specification. Profiles
// register only this digest; environment secret values must not be transmitted.
type CommandProfile struct {
	ID             string            `json:"id"`
	Kind           string            `json:"kind"`
	Executable     string            `json:"executable"`
	Args           []string          `json:"args"`
	Env            map[string]string `json:"env,omitempty"`
	EnvFrom        []string          `json:"env_from,omitempty"`
	Input          string            `json:"input,omitempty"`
	ReadOnly       bool              `json:"read_only"`
	TestEvidence   bool              `json:"test_evidence"`
	ArtifactOutput string            `json:"artifact_output,omitempty"`
	ArtifactInput  bool              `json:"artifact_input"`
}

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
var segmentPattern = regexp.MustCompile(`^[A-Za-z0-9._@+-]+$`)
var profilePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)

func ValidDigest(s string) bool    { return hashPattern.MatchString(s) }
func ValidCommit(s string) bool    { return commitPattern.MatchString(s) }
func ValidProfileID(s string) bool { return profilePattern.MatchString(s) }

// ValidPath uses a portable, explicit subset: no links, device names, alternate
// data streams, encoded separators, ambiguous trailing dots or reserved route.
func ValidPath(p string) bool {
	if p == "" || len(p) > 240 || path.Clean(p) != p || strings.HasPrefix(p, "/") {
		return false
	}
	for _, segment := range strings.Split(p, "/") {
		if len(segment) > 128 || !segmentPattern.MatchString(segment) || segment == "." || segment == ".." || strings.HasSuffix(segment, ".") || strings.EqualFold(segment, ".suma-release") {
			return false
		}
		device := strings.ToUpper(strings.SplitN(segment, ".", 2)[0])
		if device == "CON" || device == "PRN" || device == "AUX" || device == "NUL" || len(device) == 4 && (strings.HasPrefix(device, "COM") || strings.HasPrefix(device, "LPT")) && device[3] >= '0' && device[3] <= '9' {
			return false
		}
	}
	return true
}

// CanonicalManifest returns a copy, ordered by path. Its digest is SHA-256 of
// encoding/json compact UTF-8 for exactly these typed fields in declared order.
func CanonicalManifest(m ArtifactManifest) (ArtifactManifest, string, error) {
	if m.SchemaVersion != 1 || m.Kind != StaticSiteKind || len(m.Files) == 0 || len(m.Files) > MaxFiles {
		return m, "", fmt.Errorf("invalid artifact kind, schema or file count")
	}
	m.Files = append([]ArtifactFile(nil), m.Files...)
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	seen := map[string]bool{}
	blobSizes := map[string]int64{}
	var total int64
	for _, f := range m.Files {
		lower := strings.ToLower(f.Path)
		if !ValidPath(f.Path) || !ValidDigest(f.SHA256) || f.SizeBytes < 0 || f.SizeBytes > MaxFileBytes || seen[lower] {
			return m, "", fmt.Errorf("unsafe artifact path, duplicate, digest or file size: %q", f.Path)
		}
		if prior, exists := blobSizes[f.SHA256]; exists && prior != f.SizeBytes {
			return m, "", fmt.Errorf("the same blob digest cannot have two lengths")
		}
		blobSizes[f.SHA256] = f.SizeBytes
		for parent := path.Dir(lower); parent != "."; parent = path.Dir(parent) {
			if seen[parent] {
				return m, "", fmt.Errorf("artifact path is also a file: %q", f.Path)
			}
		}
		for prior := range seen {
			if strings.HasPrefix(prior, lower+"/") {
				return m, "", fmt.Errorf("artifact path is also a directory: %q", f.Path)
			}
		}
		seen[lower] = true
		total += f.SizeBytes
		if total > MaxArtifactBytes {
			return m, "", fmt.Errorf("artifact exceeds total size limit")
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return m, "", err
	}
	h := sha256.Sum256(b)
	return m, hex.EncodeToString(h[:]), nil
}
func ProfileDigest(p CommandProfile) (string, error) {
	if !ValidProfileID(p.ID) || p.Executable == "" || p.ReadOnly {
		return "", fmt.Errorf("release profiles require an ID, executable and isolated workspace")
	}
	switch p.Kind {
	case "build":
		if p.ArtifactOutput == "" || !ValidPath(p.ArtifactOutput) || p.ArtifactInput {
			return "", fmt.Errorf("build profile requires a fixed safe output directory")
		}
	case "package_test":
		if !p.ArtifactInput || !p.TestEvidence || p.ArtifactOutput != "" {
			return "", fmt.Errorf("package-test profile requires test evidence and immutable artifact input")
		}
	default:
		return "", fmt.Errorf("unsupported release profile kind")
	}
	if p.Args == nil {
		p.Args = []string{}
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
