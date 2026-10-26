package runner

import (
	"context"
	nwc "github.com/ai-mada/suma/internal/nativeworkspacecontract"
)

const NativeWorkspaceVersion = nwc.Version

// NativeWorkspacePolicy is local administrator configuration. A workflow may
// select a profile, but cannot inject endpoints, filesystem roots or commands.
type NativeWorkspacePolicy struct {
	Checks          []string `json:"checks"`
	RuntimeManifest string   `json:"runtime_manifest"`
}

// NativeWorkspaceBridge is carried only on the child's stdin. It must never
// appear in model messages, stdout facts, platform metadata or saved evidence.
type NativeWorkspaceBridge struct {
	Version      int    `json:"version"`
	Endpoint     string `json:"endpoint"`
	Token        string `json:"token"`
	PolicySHA256 string `json:"policy_sha256"`
	RequestID    string `json:"request_id"`
}

type NativeWorkspaceFileEffect = nwc.NativeWorkspaceFileEffect
type NativeWorkspaceCheckResult = nwc.NativeWorkspaceCheckResult
type NativeWorkspaceEvidence = nwc.NativeWorkspaceEvidence
type NativeWorkspaceSeal = nwc.NativeWorkspaceSeal

// The check callback accepts only a previously pinned local command ID. It
// reuses Runner process control without recursively creating another attempt.
type nativeWorkspaceCheck func(context.Context, string) (NativeWorkspaceCheckResult, error)
