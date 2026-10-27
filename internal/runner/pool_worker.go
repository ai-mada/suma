package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	rc "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Local scopes are configurations, never project grants. Paths and command
// environments remain local and are never included in machine diagnostics.
type PoolLocalScope struct {
	TenantID         string                  `json:"tenant_id"`
	ProjectID        string                  `json:"project_id"`
	Workspace        string                  `json:"workspace"`
	Repository       string                  `json:"repository,omitempty"`
	BaseCommit       string                  `json:"base_commit,omitempty"`
	Worktrees        string                  `json:"worktrees,omitempty"`
	Commands         map[string]CommandSpec  `json:"commands"`
	External         map[string]ExternalSpec `json:"external,omitempty"`
	ModelDeployments []ModelDeploymentSpec   `json:"model_deployments,omitempty"`
}
type PoolLocalConfig struct {
	SchemaVersion       int              `json:"schema_version"`
	PoolID              string           `json:"pool_id"`
	RunnerID            string           `json:"runner_id"`
	Slots               int              `json:"slots"`
	PollIntervalMS      int              `json:"poll_interval_ms,omitempty"`
	HeartbeatIntervalMS int              `json:"heartbeat_interval_ms,omitempty"`
	Scopes              []PoolLocalScope `json:"scopes"`
}

func DecodePoolConfig(raw []byte) (PoolLocalConfig, error) {
	var c PoolLocalConfig
	if len(raw) > 2<<20 {
		return c, fmt.Errorf("pool configuration exceeds 2 MiB")
	}
	if _, err := decodeToolObject(raw); err != nil {
		return c, fmt.Errorf("pool configuration must be a unique-key JSON object")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, fmt.Errorf("invalid pool configuration fields")
	}
	return c, validatePoolLocalConfig(c)
}
func validatePoolLocalConfig(c PoolLocalConfig) error {
	if c.SchemaVersion != 1 || c.PoolID == "" || c.RunnerID == "" || c.Slots < 1 || c.Slots > 128 || len(c.Scopes) < 1 || len(c.Scopes) > 256 {
		return fmt.Errorf("pool configuration requires identity, 1–128 slots and 1–256 local scopes")
	}
	if !validPoolLocalID(c.PoolID) || !validPoolLocalID(c.RunnerID) {
		return fmt.Errorf("invalid pool or runner identifier")
	}
	if c.PollIntervalMS != 0 && (c.PollIntervalMS < 100 || c.PollIntervalMS > 60000) || c.HeartbeatIntervalMS != 0 && (c.HeartbeatIntervalMS < 100 || c.HeartbeatIntervalMS > 10000) {
		return fmt.Errorf("pool poll interval must be 100–60000 ms and heartbeat 100–10000 ms; omitted/zero uses defaults")
	}
	seen := map[string]bool{}
	for _, s := range c.Scopes {
		key := s.TenantID + "/" + s.ProjectID
		if !validPoolLocalID(s.TenantID) || !validPoolLocalID(s.ProjectID) || seen[key] || s.Workspace == "" {
			return fmt.Errorf("invalid or duplicate local project scope")
		}
		seen[key] = true
		for id, spec := range s.External {
			if id == "" || spec.Protocol != rc.RepositoryProtocol || spec.TokenEnv == "" {
				return fmt.Errorf("pool external configuration only supports explicit repository integration providers")
			}
			if _, err := externalURL(spec); err != nil {
				return err
			}
		}
		if err := ValidateToolConfiguration(ExecConfig{Commands: s.Commands, ModelDeployments: s.ModelDeployments}); err != nil {
			return err
		}
		for _, spec := range s.Commands {
			if spec.Adapter == "suma" {
				if project := spec.Env["SUMA_PROJECT_ID"]; project != "" && project != s.ProjectID {
					return fmt.Errorf("native agent project differs from its local pool scope")
				}
				for _, name := range spec.EnvFrom {
					if strings.EqualFold(name, "SUMA_PROJECT_ID") {
						return fmt.Errorf("native agent project cannot come from a mutable parent environment")
					}
				}
			}
			if spec.ArtifactInput || spec.ArtifactOutput != "" {
				return fmt.Errorf("pool artifact execution requires a separate authorized protocol")
			}
			for key := range spec.Env {
				if strings.EqualFold(key, "SUMA_POOL_RUNNER_TOKEN") || strings.EqualFold(key, "SUMA_RUNNER_TOKEN") {
					return fmt.Errorf("machine credentials cannot be command environment values")
				}
			}
			for _, key := range spec.EnvFrom {
				if strings.EqualFold(key, "SUMA_POOL_RUNNER_TOKEN") || strings.EqualFold(key, "SUMA_RUNNER_TOKEN") {
					return fmt.Errorf("machine credentials cannot be forwarded to child processes")
				}
			}
		}
	}
	return nil
}

type PoolDiagnostic struct {
	Code      string `json:"code"`
	SlotID    string `json:"slot_id,omitempty"`
	TenantID  string `json:"tenant_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}
type PoolWorker struct {
	Client                          *Client
	Config                          PoolLocalConfig
	StateDir                        string
	AllowExec                       bool
	PollInterval, HeartbeatInterval time.Duration
	Diagnostic                      func(PoolDiagnostic)
	mu                              sync.Mutex
	local                           map[string]*Worker
	lastInventory                   time.Time
	configSHA                       string
	credentialVersion               int
}

func scopeKey(tenant, project string) string { return digest([]byte(tenant + "\x00" + project)) }
func (w *PoolWorker) notice(code, slot string) {
	if w.Diagnostic != nil {
		w.Diagnostic(PoolDiagnostic{Code: code, SlotID: slot})
	}
}

func resolvedPoolConfig(c PoolLocalConfig) (PoolLocalConfig, error) {
	var frozen PoolLocalConfig
	if err := validatePoolLocalConfig(c); err != nil {
		return frozen, err
	}
	// Freeze resolved filesystem bindings as part of the restart identity. A
	// changed working directory must not reinterpret relative project paths.
	rawConfig, _ := json.Marshal(c)
	if err := json.Unmarshal(rawConfig, &frozen); err != nil {
		return frozen, err
	}
	seenWorkspace := map[string]bool{}
	for i, scope := range frozen.Scopes {
		workspace, err := realDirectory(scope.Workspace)
		if err != nil {
			return frozen, fmt.Errorf("local project workspace unavailable")
		}
		identity := workspace
		if runtime.GOOS == "windows" {
			identity = strings.ToLower(identity)
		}
		if seenWorkspace[identity] {
			return frozen, fmt.Errorf("local project workspaces must be distinct")
		}
		seenWorkspace[identity] = true
		frozen.Scopes[i].Workspace = workspace
		if scope.Repository != "" {
			repo, err := realDirectory(scope.Repository)
			if err != nil {
				return frozen, fmt.Errorf("local project repository unavailable")
			}
			frozen.Scopes[i].Repository = repo
		}
		if scope.Worktrees != "" {
			path, err := filepath.Abs(scope.Worktrees)
			if err != nil {
				return frozen, fmt.Errorf("local project attempt directory unavailable")
			}
			frozen.Scopes[i].Worktrees = path
		}
		for name, spec := range frozen.Scopes[i].Commands {
			if spec.Adapter == "suma" {
				if spec.Env == nil {
					spec.Env = map[string]string{}
				}
				spec.Env["SUMA_PROJECT_ID"] = scope.ProjectID
				frozen.Scopes[i].Commands[name] = spec
			}
		}
	}
	return frozen, nil
}

func (w *PoolWorker) Run(ctx context.Context) error {
	frozen, err := resolvedPoolConfig(w.Config)
	if err != nil {
		return err
	}
	w.Config = frozen
	if w.Client == nil || w.Client.Pool == nil || w.Client.Pool.PoolID != w.Config.PoolID || w.Client.Pool.RunnerID != w.Config.RunnerID {
		return fmt.Errorf("pool worker/client identity mismatch")
	}
	session, err := w.Client.PoolSession(ctx)
	if err != nil {
		return err
	}
	if w.Config.Slots > session.Runner.MaxSlots {
		return fmt.Errorf("local slots exceed current machine registration")
	}
	w.credentialVersion = session.Runner.CredentialVersion
	raw, _ := json.Marshal(w.Config)
	w.configSHA = digest(raw)
	if err = os.MkdirAll(w.StateDir, 0700); err != nil {
		return err
	}
	// The identity lock prevents two processes from sharing slot state. A new
	// credential/configuration uses a new state directory instead of guessing
	// whether unfinished operations from the previous identity may be replayed.
	unlock, err := lockJournal(filepath.Join(w.StateDir, "pool.lock"))
	if err != nil {
		return fmt.Errorf("pool state is already owned or unavailable")
	}
	defer unlock()
	binding, _ := json.Marshal([]any{digest([]byte(strings.TrimRight(w.Client.BaseURL, "/"))), w.Config.PoolID, w.Config.RunnerID, w.credentialVersion, w.configSHA})
	path := filepath.Join(w.StateDir, "identity.json")
	if old, e := os.ReadFile(path); e == nil && !bytes.Equal(old, binding) {
		return fmt.Errorf("pool state belongs to a different control, credential or local configuration")
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	if err = writeEvidence(path, binding); err != nil {
		return err
	}
	w.local = map[string]*Worker{}
	defer func() {
		for _, local := range w.local {
			local.Journal.Close()
		}
	}()
	for _, scope := range w.Config.Scopes {
		key := scopeKey(scope.TenantID, scope.ProjectID)
		j, err := OpenJournal(filepath.Join(w.StateDir, "inventories", key))
		if err != nil {
			return err
		}
		client := *w.Client
		pool := *w.Client.Pool
		pool.TenantID, pool.ProjectID = scope.TenantID, scope.ProjectID
		client.Pool = &pool
		client.ProjectID = scope.ProjectID
		w.local[key] = &Worker{Client: &client, Journal: j, ID: session.Runner.WorkerID, Exec: ExecConfig{AllowExec: w.AllowExec, Workspace: scope.Workspace, Repository: scope.Repository, BaseCommit: scope.BaseCommit, WorktreeRoot: scope.Worktrees, Commands: scope.Commands, ModelDeployments: scope.ModelDeployments, External: scope.External, Secrets: []string{w.Client.Token}}}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, w.Config.Slots)
	for i := 0; i < w.Config.Slots; i++ {
		go func(slot string) { done <- w.runSlot(ctx, slot, session.Runner.WorkerID) }(strconv.Itoa(i))
	}
	var first error
	for i := 0; i < w.Config.Slots; i++ {
		e := <-done
		if e != nil && !errors.Is(e, context.Canceled) && !errors.Is(e, context.DeadlineExceeded) && first == nil {
			first = e
			cancel()
		}
	}
	if first != nil {
		return first
	}
	return ctx.Err()
}

func (w *PoolWorker) refreshInventories(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.lastInventory.IsZero() && time.Since(w.lastInventory) < 30*time.Second {
		return nil
	}
	for _, local := range w.local {
		if err := local.refreshInventory(ctx); err != nil {
			local.setInventoryUnavailable()
			w.notice("inventory_unavailable", "")
			if permanentInventoryError(err) {
				return err
			}
		}
	}
	w.lastInventory = time.Now()
	return nil
}

func (w *PoolWorker) runSlot(ctx context.Context, slot, worker string) error {
	j, err := OpenJournal(filepath.Join(w.StateDir, "slots", slot))
	if err != nil {
		return err
	}
	defer j.Close()
	poll, beat := w.PollInterval, w.HeartbeatInterval
	if poll <= 0 {
		poll = 2 * time.Second
		if w.Config.PollIntervalMS != 0 {
			poll = time.Duration(w.Config.PollIntervalMS) * time.Millisecond
		}
	}
	if beat <= 0 {
		beat = 5 * time.Second
		if w.Config.HeartbeatIntervalMS != 0 {
			beat = time.Duration(w.Config.HeartbeatIntervalMS) * time.Millisecond
		}
	}
	for ctx.Err() == nil {
		for _, a := range j.pendingReleases() {
			if a.RepositoryIntegration == nil || a.ReleaseExecution != nil {
				return fmt.Errorf("pool recovery requires a repository integration binding")
			}
			local := w.local[scopeKey(a.TenantID, a.ProjectID)]
			if local == nil {
				return fmt.Errorf("pool repository recovery lacks its fixed local scope")
			}
			client, e := w.Client.forPoolAssignment(a)
			if e != nil {
				return e
			}
			executor := Worker{Client: client, Journal: j, ID: worker, Exec: local.Exec, inventory: local.currentInventory()}
			if e = executor.runAssignment(ctx, a, beat); e != nil {
				return e
			}
		}
		if err = replayPool(ctx, w.Client, j); err != nil {
			var status *runnerHTTPStatusError
			if errors.As(err, &status) && (status.Status == 401 || status.Status == 403) {
				return fmt.Errorf("pool callback authority rejected; original outbox retained for reconciliation")
			}
			w.notice("callback_pending", slot)
			if !wait(ctx, poll) {
				break
			}
			continue
		}
		_ = ReplayWorkspaceObservations(ctx, w.Client, j, nil)
		session, e := w.Client.PoolSession(ctx)
		if e != nil {
			if permanentInventoryError(e) {
				return e
			}
			w.notice("session_unavailable", slot)
			if !wait(ctx, poll) {
				break
			}
			continue
		}
		if session.Runner.CredentialVersion != w.credentialVersion || session.Runner.WorkerID != worker {
			return fmt.Errorf("pool credential generation changed")
		}
		if err = w.refreshInventories(ctx); err != nil {
			return err
		}
		active := map[string]bool{}
		for _, scope := range session.Scopes {
			active[scopeKey(scope.TenantID, scope.ProjectID)] = true
		}
		scopes := []PoolCapabilityScope{}
		for _, scope := range w.Config.Scopes {
			local := w.local[scopeKey(scope.TenantID, scope.ProjectID)]
			if active[scopeKey(scope.TenantID, scope.ProjectID)] && local.canClaimInventory() {
				caps := append(ToolCapabilities(local.currentInventory()), ExternalCapabilities(local.Exec.External)...)
				scopes = append(scopes, PoolCapabilityScope{TenantID: scope.TenantID, ProjectID: scope.ProjectID, Capabilities: caps})
			}
		}
		// A pending request must be refreshed even when the pool is paused or a
		// grant disappeared. Server authority, not local session hints, decides.
		old, e := loadPoolClaim(j)
		if e != nil {
			return e
		}
		if (len(scopes) == 0 || !session.PoolActive) && (old == nil || old.Complete) {
			w.notice("waiting_for_scope_or_pool", slot)
			if !wait(ctx, poll) {
				break
			}
			continue
		}
		checkpoint, e := beginPoolClaim(j, w.Client.BaseURL, w.configSHA, w.credentialVersion, PoolClaimRequest{PoolID: w.Config.PoolID, RunnerID: w.Config.RunnerID, WorkerID: worker, SlotID: slot, Scopes: scopes})
		if e != nil {
			return e
		}
		a, e := w.Client.ClaimPool(ctx, checkpoint.Request)
		if e != nil {
			w.notice("claim_pending", slot)
			if !wait(ctx, poll) {
				break
			}
			continue
		}
		checkpoint.Responded = true
		checkpoint.Assignment = a
		if e = savePoolClaim(j, checkpoint); e != nil {
			return e
		}
		if a != nil && a.DispatchState == "active" {
			if a.PoolCredentialVersion != w.credentialVersion {
				return fmt.Errorf("dispatch credential differs from local session")
			}
			local := w.local[scopeKey(a.TenantID, a.ProjectID)]
			if local == nil {
				return fmt.Errorf("dispatch lacks exact local project configuration")
			}
			if a.ReleaseExecution != nil || a.Type == "external_workflow" && a.RepositoryIntegration == nil {
				return fmt.Errorf("pool external/deployment capability is not configured")
			}
			if e = j.Accept(*a); e != nil {
				return e
			}
			checkpoint.Complete = true
			if e = savePoolClaim(j, checkpoint); e != nil {
				return e
			}
			client, e := w.Client.forPoolAssignment(*a)
			if e != nil {
				return e
			}
			// Close the receive-to-launch authority gap. Session is only a hint;
			// this exact dispatch is checked again before any child is created.
			cancelled, authorityErr := client.Heartbeat(ctx, *a)
			if cancelled || authorityErr != nil {
				if e = j.Finish(Result{Assignment: *a, ExitCode: -1, Error: "pool execution not started: pre-launch authority unavailable or cancellation requested"}); e != nil {
					return e
				}
				w.notice("execution_not_started", slot)
				continue
			}
			executor := Worker{Client: client, Journal: j, ID: worker, Exec: local.Exec, inventory: local.currentInventory()}
			w.notice("execution_started", slot)
			if e = executor.runAssignment(ctx, *a, beat); e != nil {
				return e
			}
			w.notice("execution_finished", slot)
		} else {
			checkpoint.Complete = true
			if e = savePoolClaim(j, checkpoint); e != nil {
				return e
			}
			if !wait(ctx, poll) {
				break
			}
		}
	}
	return ctx.Err()
}
