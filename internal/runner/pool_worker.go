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
		for id, spec := range s.Commands {
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
				kind, _, err := localArtifactCapability(id, spec)
				if err != nil || kind == "" {
					return fmt.Errorf("pool artifacts require a dedicated build or package-test command")
				}
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
	AllowInstalledToolDiscovery     bool
	AllowToolPreparation            bool
	MaintenanceOnly                 bool
	PollInterval, HeartbeatInterval time.Duration
	Diagnostic                      func(PoolDiagnostic)
	local                           map[string]*Worker
	configSHA                       string
	credentialVersion               int
	inventoryRetry                  map[string]time.Time
}

var errPoolInventoryIdentity = errors.New("pool inventory machine identity changed")

// ErrPoolShutdownUnconfirmed means the pool could not prove local execution
// and ownership had ended. Remote result delivery remains a separate fact.
var ErrPoolShutdownUnconfirmed = errors.New("pool shutdown could not be confirmed")

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

func (w *PoolWorker) Run(ctx context.Context) (runErr error) {
	if w.AllowToolPreparation && !w.AllowInstalledToolDiscovery {
		return fmt.Errorf("tool preparation requires installed discovery")
	}
	if w.MaintenanceOnly && !w.AllowInstalledToolDiscovery {
		return fmt.Errorf("maintenance-only requires installed tool discovery")
	}
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
	defer func() {
		if unlock() != nil {
			w.notice("pool_shutdown_unconfirmed", "")
			runErr = errors.Join(runErr, fmt.Errorf("%w: pool state ownership could not be released", ErrPoolShutdownUnconfirmed))
		}
	}()
	if w.MaintenanceOnly {
		if err = requireDrainedPoolSlots(filepath.Join(w.StateDir, "slots")); err != nil {
			return err
		}
	}
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
	checkInventory := false
	defer func() {
		for _, scope := range w.Config.Scopes {
			local := w.local[scopeKey(scope.TenantID, scope.ProjectID)]
			if local == nil {
				continue
			}
			var evidenceErr error
			if checkInventory {
				evidenceErr = local.Journal.requireNoActiveToolMaintenance()
			}
			if closeErr := local.Journal.Close(); evidenceErr != nil || closeErr != nil {
				if w.Diagnostic != nil {
					w.Diagnostic(PoolDiagnostic{Code: "inventory_shutdown_unconfirmed", TenantID: scope.TenantID, ProjectID: scope.ProjectID})
				}
				runErr = errors.Join(runErr, fmt.Errorf("%w: inventory ownership or maintenance needs reconciliation", ErrPoolShutdownUnconfirmed))
			}
		}
	}()
	for _, scope := range w.Config.Scopes {
		key := scopeKey(scope.TenantID, scope.ProjectID)
		open := OpenJournal
		if w.MaintenanceOnly {
			open = OpenMaintenanceJournal
		}
		j, err := open(filepath.Join(w.StateDir, "inventories", key))
		if err != nil {
			return err
		}
		client := *w.Client
		pool := *w.Client.Pool
		pool.TenantID, pool.ProjectID = scope.TenantID, scope.ProjectID
		client.Pool = &pool
		client.ProjectID = scope.ProjectID
		w.local[key] = &Worker{Client: &client, Journal: j, ID: session.Runner.WorkerID, AllowInstalledToolDiscovery: w.AllowInstalledToolDiscovery, AllowToolPreparation: w.AllowToolPreparation, Exec: ExecConfig{AllowExec: w.AllowExec, Workspace: scope.Workspace, Repository: scope.Repository, BaseCommit: scope.BaseCommit, WorktreeRoot: scope.Worktrees, Commands: scope.Commands, ModelDeployments: scope.ModelDeployments, External: scope.External, Secrets: []string{w.Client.Token}}}
		if err = w.local[key].enableToolMaintenance(); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	checkInventory = true
	if w.MaintenanceOnly {
		return w.runInventoryOwner(ctx)
	}
	done := make(chan error, w.Config.Slots+1)
	// One owner services all project inventories independently of occupied
	// execution slots. Slots only read the resulting readiness gates.
	go func() { done <- w.runInventoryOwner(ctx) }()
	for i := 0; i < w.Config.Slots; i++ {
		go func(slot string) { done <- w.runSlot(ctx, slot, session.Runner.WorkerID) }(strconv.Itoa(i))
	}
	var first error
	var unconfirmed []error
	for i := 0; i < w.Config.Slots+1; i++ {
		err := <-done
		if err == nil {
			continue
		}
		cancel()
		if errors.Is(err, ErrPoolShutdownUnconfirmed) {
			unconfirmed = append(unconfirmed, err)
		} else if first == nil {
			first = err
		}
	}
	if len(unconfirmed) == 0 {
		return first
	}
	return errors.Join(append(unconfirmed, first)...)
}

func (w *PoolWorker) refreshInventories(ctx context.Context) error {
	session, err := w.Client.PoolSession(ctx)
	if err != nil {
		return err
	}
	if session.Runner.CredentialVersion != w.credentialVersion {
		return fmt.Errorf("%w: credential generation changed", errPoolInventoryIdentity)
	}
	if w.inventoryRetry == nil {
		w.inventoryRetry = map[string]time.Time{}
	}
	active := map[string]bool{}
	for _, s := range session.Scopes {
		active[scopeKey(s.TenantID, s.ProjectID)] = true
	}
	for key, local := range w.local {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if local.ID != session.Runner.WorkerID {
			return fmt.Errorf("%w: worker changed", errPoolInventoryIdentity)
		}
		if !session.PoolActive || !active[key] {
			local.setInventoryUnavailable()
			local.inventoryNext = time.Time{}
			continue
		}
		if time.Now().Before(w.inventoryRetry[key]) {
			local.setInventoryUnavailable()
			continue
		}
		if err := local.tickToolInventory(ctx); err != nil {
			local.setInventoryUnavailable()
			// A project report/inspection can lose its grant or source while
			// another project's attempt remains authorized. Retain the original
			// checkpoint, quarantine this scope, and never cancel sibling slots.
			delay := 5 * time.Second
			if permanentInventoryError(err) {
				delay = 30 * time.Second
			}
			w.inventoryRetry[key] = time.Now().Add(delay)
			if w.Diagnostic != nil {
				w.Diagnostic(PoolDiagnostic{Code: "inventory_scope_unavailable", TenantID: local.Client.Pool.TenantID, ProjectID: local.Client.ProjectID})
			}
			continue
		}
		delete(w.inventoryRetry, key)
	}
	return nil
}

func (w *PoolWorker) runInventoryOwner(ctx context.Context) error {
	poll := w.PollInterval
	if poll <= 0 {
		poll = 2 * time.Second
		if w.Config.PollIntervalMS != 0 {
			poll = time.Duration(w.Config.PollIntervalMS) * time.Millisecond
		}
	}
	for ctx.Err() == nil {
		if err := w.refreshInventories(ctx); err != nil {
			for _, local := range w.local {
				local.setInventoryUnavailable()
				local.inventoryNext = time.Time{}
			}
			var status *runnerHTTPStatusError
			if errors.Is(err, errPoolInventoryIdentity) || errors.As(err, &status) && (status.Status == 401 || status.Status == 403) {
				return err
			}
			if ctx.Err() != nil {
				break
			}
			w.notice("inventory_unavailable", "")
		}
		if !wait(ctx, poll) {
			break
		}
	}
	return nil
}

func (w *PoolWorker) runSlot(ctx context.Context, slot, worker string) error {
	j, err := OpenJournal(filepath.Join(w.StateDir, "slots", slot))
	if err != nil {
		w.notice("slot_shutdown_unconfirmed", slot)
		return fmt.Errorf("%w: slot %s original state is unavailable", ErrPoolShutdownUnconfirmed, slot)
	}
	priorUnknown := j.acknowledgedUnknownResults()
	runErr := w.runSlotBusiness(ctx, slot, worker, j)
	evidenceErr := j.requireShutdownBusinessEvidence(priorUnknown)
	_, claimErr := loadPoolClaim(j)
	closeErr := j.Close()
	if evidenceErr != nil || claimErr != nil || closeErr != nil {
		w.notice("slot_shutdown_unconfirmed", slot)
		return fmt.Errorf("%w: slot %s needs original execution or state reconciliation", ErrPoolShutdownUnconfirmed, slot)
	}
	return runErr
}

func (w *PoolWorker) runSlotBusiness(ctx context.Context, slot, worker string, j *Journal) error {
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
		for _, a := range j.pendingCloudExecutions() {
			if ctx.Err() != nil {
				return nil
			}
			local := w.local[scopeKey(a.TenantID, a.ProjectID)]
			if local == nil || a.WorkerID != worker {
				return fmt.Errorf("pool cloud recovery lacks its fixed local scope")
			}
			client, e := w.Client.forPoolAssignment(a)
			if e != nil {
				return e
			}
			executor := Worker{Client: client, Journal: j, ID: worker, Exec: local.currentExecSnapshot(), inventory: local.currentInventory()}
			if e = executor.runAssignment(ctx, a, beat); e != nil {
				return e
			}
		}
		for _, a := range j.pendingReleases() {
			if ctx.Err() != nil {
				return nil
			}
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
			executor := Worker{Client: client, Journal: j, ID: worker, Exec: local.currentExecSnapshot(), inventory: local.currentInventory()}
			if e = executor.runAssignment(ctx, a, beat); e != nil {
				return e
			}
		}
		recovery := Worker{Client: w.Client, Journal: j, ID: worker}
		replayErr := recovery.resumeArtifacts(ctx, beat)
		if replayErr == nil {
			replayErr = replayPool(ctx, w.Client, j)
		}
		if replayErr != nil {
			var status *runnerHTTPStatusError
			if errors.As(replayErr, &status) && (status.Status == 401 || status.Status == 403) {
				return fmt.Errorf("pool callback authority rejected; original outbox retained for reconciliation")
			}
			w.notice("callback_pending", slot)
			if !wait(ctx, poll) {
				break
			}
			continue
		}
		if ctx.Err() != nil {
			return nil
		}
		_ = ReplayWorkspaceObservations(ctx, w.Client, j, nil)
		_ = ReplayWorkspaceWrites(ctx, w.Client, j, nil)
		if ctx.Err() != nil {
			return nil
		}
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
		active := map[string]bool{}
		for _, scope := range session.Scopes {
			active[scopeKey(scope.TenantID, scope.ProjectID)] = true
		}
		scopes := []PoolCapabilityScope{}
		for _, scope := range w.Config.Scopes {
			local := w.local[scopeKey(scope.TenantID, scope.ProjectID)]
			if active[scopeKey(scope.TenantID, scope.ProjectID)] && local.canClaimInventory() {
				caps := runnerCapabilities(local.currentExecSnapshot(), ToolCapabilities(local.currentInventory()), false)
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
		if ctx.Err() != nil {
			return nil
		}
		checkpoint, e := beginPoolClaim(j, w.Client.BaseURL, w.configSHA, w.credentialVersion, PoolClaimRequest{PoolID: w.Config.PoolID, RunnerID: w.Config.RunnerID, WorkerID: worker, SlotID: slot, Scopes: scopes})
		if e != nil {
			return e
		}
		if ctx.Err() != nil {
			return nil
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
			// The original claim response remains replayable until acceptance.
			if ctx.Err() != nil {
				return nil
			}
			if a.PoolCredentialVersion != w.credentialVersion {
				return fmt.Errorf("dispatch credential differs from local session")
			}
			local := w.local[scopeKey(a.TenantID, a.ProjectID)]
			if local == nil {
				return fmt.Errorf("dispatch lacks exact local project configuration")
			}
			if a.ReleaseExecution != nil || a.Type == "external_workflow" && a.RepositoryIntegration == nil && a.WorkBuddyCloud == nil {
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
			if ctx.Err() != nil && a.WorkBuddyCloud != nil {
				// A WorkBuddy result requires an original waiting proof. Preserve
				// the accepted assignment for reconciliation without a provider call.
				return nil
			}
			if (ctx.Err() != nil || cancelled || authorityErr != nil) && a.WorkBuddyCloud == nil {
				if e = j.Finish(Result{Assignment: *a, ExitCode: -1, Error: "pool execution not started: pre-launch authority unavailable or cancellation requested"}); e != nil {
					return e
				}
				w.notice("execution_not_started", slot)
				continue
			}
			executor := Worker{Client: client, Journal: j, ID: worker, Exec: local.currentExecSnapshot(), inventory: local.currentInventory()}
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
	return nil
}
