package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	pc "github.com/ai-mada/suma/internal/poolconnectioncontract"
	"github.com/ai-mada/suma/internal/strictjson"
)

// ReadPoolConfigFile is shared by one-shot checking and foreground startup.
// Resolve a normal relative CLI filename once, then use the original bounded,
// rooted ordinary-file/parent identity checks. No paths enter its safe error.
func ReadPoolConfigFile(path string) (PoolLocalConfig, string, error) {
	var config PoolLocalConfig
	invalid := func() (PoolLocalConfig, string, error) {
		return config, "", fmt.Errorf("Pool local configuration file unavailable or invalid")
	}
	if !localToolSafeText(path, 4096) || strings.TrimSpace(path) != path {
		return invalid()
	}
	if runtime.GOOS == "windows" {
		volume := filepath.VolumeName(path)
		if cursorDiscoveryWindowsNamespace(path) || volume != "" && !cursorDiscoveryWindowsDrive(path) || volume == "" && (strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`)) {
			return invalid()
		}
		for _, part := range strings.FieldsFunc(strings.TrimPrefix(path, volume), func(r rune) bool { return r == '/' || r == '\\' }) {
			if part != "." && part != ".." && strings.TrimRight(part, ". ") != part || strings.ContainsRune(part, ':') {
				return invalid()
			}
		}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return invalid()
	}
	source, err := inspectPoolPreparationFile(absolute, poolPreparationFileLimit)
	if err != nil {
		return invalid()
	}
	config, err = DecodePoolConfig(source.raw)
	if err != nil || !cursorPackageParentsStable(source.parents) {
		return invalid()
	}
	return config, digest(source.raw), nil
}

// CheckPoolConnection observes exactly one original authenticated PoolSession.
// It does not start a Worker, open a Journal, reserve slots, run a command or
// derive an execution permission from these historical connection facts.
func CheckPoolConnection(ctx context.Context, client *Client, config PoolLocalConfig, identity PoolRunnerIdentity, configurationFileSHA256 string) (pc.PoolConnectionCheck, error) {
	var out pc.PoolConnectionCheck
	invalid := func() (pc.PoolConnectionCheck, error) {
		return out, fmt.Errorf("Pool connection identity or configuration is unavailable")
	}
	if client == nil || client.Pool == nil || client.Pool.Assignment != nil || !validPoolRunnerIdentity(identity) || !desktopManagedHex(configurationFileSHA256, 64) || client.Pool.PoolID != config.PoolID || client.Pool.RunnerID != config.RunnerID || identity.PoolID != config.PoolID || identity.RunnerID != config.RunnerID || digest([]byte(client.Token)) != identity.CredentialSHA256 || config.Slots > identity.MaxSlots {
		return invalid()
	}
	origin, err := cloudControlOrigin(client.BaseURL)
	if err != nil || origin != identity.ControlURL {
		return invalid()
	}
	frozen, err := resolvedPoolConfig(config)
	if err != nil {
		return invalid()
	}
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	session, err := client.PoolSession(bounded)
	if err != nil {
		return out, err
	}
	if session.Runner.CredentialVersion != identity.CredentialVersion || session.Runner.WorkerID != identity.WorkerID || frozen.Slots > session.Runner.MaxSlots {
		return invalid()
	}
	granted := map[string]bool{}
	for _, scope := range session.Scopes {
		granted[scopeKey(scope.TenantID, scope.ProjectID)] = true
	}
	out = pc.PoolConnectionCheck{Protocol: pc.PoolConnectionCheckProtocol, PoolID: config.PoolID, RunnerID: config.RunnerID, WorkerID: session.Runner.WorkerID, CredentialVersion: session.Runner.CredentialVersion, PoolActive: session.PoolActive, ConfiguredSlots: config.Slots, CurrentMaxSlots: session.Runner.MaxSlots, ConfigurationFileSHA256: configurationFileSHA256, CheckedAt: time.Now().UTC().Format(time.RFC3339Nano), Scopes: []pc.PoolConnectionScope{}}
	for _, scope := range frozen.Scopes {
		out.Scopes = append(out.Scopes, pc.PoolConnectionScope{TenantID: scope.TenantID, ProjectID: scope.ProjectID, Granted: granted[scopeKey(scope.TenantID, scope.ProjectID)]})
	}
	return out, nil
}

// The API returns the full existing Runner/PoolScope DTO. Only its known
// metadata names may supplement the required machine identity projection.
// Missing required booleans/arrays are not fabricated from Go zero values.
func decodePoolSession(raw []byte) (PoolRunnerSession, error) {
	var out PoolRunnerSession
	invalid := func() (PoolRunnerSession, error) { return out, fmt.Errorf("invalid bounded Pool session response") }
	var root map[string]any
	if strictjson.Decode(raw, &root, 2<<20) != nil || !poolSessionFields(root, []string{"runner", "pool_active", "scopes"}, nil) {
		return invalid()
	}
	runner, ok := root["runner"].(map[string]any)
	if !ok || !poolSessionFields(runner, []string{"id", "pool_id", "worker_id", "max_slots", "credential_version"}, []string{"name", "version", "status", "credential_expires_at", "last_seen_at", "allowed_actions", "created_at"}) {
		return invalid()
	}
	scopes, ok := root["scopes"].([]any)
	if !ok || len(scopes) > 256 {
		return invalid()
	}
	for _, value := range scopes {
		scope, ok := value.(map[string]any)
		if !ok || !poolSessionFields(scope, []string{"tenant_id", "project_id"}, []string{"pool_id", "grant_id", "grant_version", "project_weight", "status"}) {
			return invalid()
		}
	}
	if json.Unmarshal(raw, &out) != nil {
		return invalid()
	}
	seen := map[string]bool{}
	for _, scope := range out.Scopes {
		key := scopeKey(scope.TenantID, scope.ProjectID)
		if !validPoolLocalID(scope.TenantID) || !validPoolLocalID(scope.ProjectID) || seen[key] || scope.PoolID != "" && scope.PoolID != out.Runner.PoolID || scope.GrantID != "" && !validPoolLocalID(scope.GrantID) || scope.Status != "" && scope.Status != "active" {
			return invalid()
		}
		seen[key] = true
	}
	return out, nil
}

func poolSessionFields(value map[string]any, required, optional []string) bool {
	allowed := map[string]bool{}
	for _, name := range append(append([]string{}, required...), optional...) {
		allowed[name] = true
	}
	for _, name := range required {
		if value[name] == nil {
			return false
		}
	}
	for name, item := range value {
		if !allowed[name] || item == nil {
			return false
		}
	}
	return true
}
