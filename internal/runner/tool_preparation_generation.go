package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

// A generation is immutable. Every slot copies one complete snapshot; an old
// assignment retains its original map even when maintenance publishes a head.
type toolCommandGeneration struct {
	Protocol     string       `json:"protocol"`
	ScopeSHA256  string       `json:"scope_sha256"`
	TenantID     string       `json:"tenant_id"`
	ProjectID    string       `json:"project_id"`
	WorkerID     string       `json:"worker_id"`
	BaseSHA256   string       `json:"base_sha256"`
	ParentSHA256 string       `json:"parent_sha256"`
	Preparation  tp.Reference `json:"preparation"`
	SourceSHA256 string       `json:"source_sha256"`
	// Only generated ACP commands are durable here. Original hand-written Env
	// values stay in their original configuration and the in-memory snapshot.
	Commands map[string]CommandSpec `json:"commands"`
	Profiles map[string]tc.Profile  `json:"profiles"`
}
type toolGenerationHead struct {
	GenerationSHA256 string                `json:"generation_sha256"`
	Generation       toolCommandGeneration `json:"generation"`
	Receipt          tp.GenerationReceipt  `json:"receipt"`
	Inventory        InventoryReport       `json:"inventory"`
	InventoryStarted bool                  `json:"inventory_started"`
	InventoryAcked   bool                  `json:"inventory_acked"`
}
type toolLocalAcceptance struct {
	GenerationSHA256 string       `json:"generation_sha256"`
	Preparation      tp.Reference `json:"preparation"`
	SourceSHA256     string       `json:"source_sha256"`
	ArtifactSHA256   string       `json:"artifact_sha256"`
	Profile          tc.Profile   `json:"profile"`
	StopProof        tp.StopProof `json:"stop_proof"`
}

func commandMapSnapshot(commands map[string]CommandSpec) (map[string]CommandSpec, error) {
	if commands == nil {
		commands = map[string]CommandSpec{}
	}
	raw, err := json.Marshal(commands)
	if err != nil || len(raw) > 2<<20 {
		return nil, fmt.Errorf("command generation exceeds its bound")
	}
	return DecodeCommands(raw)
}
func baseExecutionSHA(cfg ExecConfig) (string, error) {
	commands, err := commandMapSnapshot(cfg.Commands)
	if err != nil {
		return "", err
	}
	// Execution permission, process mode and credential VALUES are intentionally
	// not part of the base. Repository/resource changes are different snapshots.
	raw, err := json.Marshal(struct {
		Commands                                     map[string]CommandSpec
		Workspace, Repository, BaseCommit, Worktrees string
		Models, External                             any
	}{commands, cfg.Workspace, cfg.Repository, cfg.BaseCommit, cfg.WorktreeRoot, cfg.ModelDeployments, cfg.External})
	if err != nil {
		return "", err
	}
	return digest(raw), nil
}
func (w *Worker) currentExecSnapshot() ExecConfig {
	w.execMu.RLock()
	defer w.execMu.RUnlock()
	cfg := w.Exec
	if w.managedCommands != nil {
		cfg.Commands = w.managedCommands
	}
	return cfg
}
func (w *Worker) currentGenerationSHA() string {
	w.execMu.RLock()
	defer w.execMu.RUnlock()
	return w.managedGenerationSHA
}
func (w *Worker) initializeCommandGeneration() error {
	base, err := baseExecutionSHA(w.Exec)
	if err != nil {
		return err
	}
	scope, err := InventoryScopeSHA256(w.Client.BaseURL, w.Client.ProjectID, w.ID)
	if err != nil {
		return err
	}
	commands, err := commandMapSnapshot(w.Exec.Commands)
	if err != nil {
		return err
	}
	w.execMu.Lock()
	defer w.execMu.Unlock()
	if w.managedGenerationSHA == "" {
		w.managedGenerationSHA = digest([]byte(scope + ":" + base))
		w.managedBaseSHA = base
		w.managedCommands = commands
	}
	return nil
}
func (j *Journal) toolGenerationHeadPath() string {
	return filepath.Join(filepath.Dir(j.path), "tool-command-head.json")
}
func (j *Journal) readToolGenerationHead() (*toolGenerationHead, error) {
	var h toolGenerationHead
	if _, err := os.Lstat(j.toolGenerationHeadPath()); os.IsNotExist(err) {
		return nil, nil
	}
	if _, err := readToolProbeJSON(j.toolGenerationHeadPath(), &h); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(h.Generation)
	if h.Generation.Protocol != tp.Protocol || !tc.ValidDigest(h.GenerationSHA256) || digest(raw) != h.GenerationSHA256 || tp.ValidateGenerationReceipt(h.Receipt) != nil || h.Receipt.GenerationSHA256 != h.GenerationSHA256 || h.Receipt.ParentGenerationSHA256 != h.Generation.ParentSHA256 || h.Receipt.StopProof.Preparation != h.Generation.Preparation || h.Receipt.StopProof.SourceSHA256 != h.Generation.SourceSHA256 || h.Inventory.Preparation == nil || tp.ValidateInventoryBinding(*h.Inventory.Preparation) != nil || h.Inventory.Preparation.GenerationSHA256 != h.GenerationSHA256 || h.Inventory.Preparation.Reference != h.Generation.Preparation || h.Inventory.Preparation.SourceSHA256 != h.Generation.SourceSHA256 || h.Inventory.Preparation.LocalReceiptSHA256 != h.Receipt.LocalReceiptSHA256 {
		return nil, fmt.Errorf("managed generation head differs")
	}
	return &h, nil
}

// The head and original outbound inventory are a SINGLE durable record. The
// ordinary inventory file is a recoverable mirror, never the acceptance fact.
func (j *Journal) acceptToolGeneration(h toolGenerationHead) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.inventory != nil && !j.inventory.Acked {
		return fmt.Errorf("previous inventory must be acknowledged before acceptance")
	}
	raw, err := json.Marshal(h)
	if err != nil || len(raw) > 2<<20 {
		return fmt.Errorf("managed head exceeds its bound")
	}
	if err = writeEvidence(j.toolGenerationHeadPath(), raw); err != nil {
		return err
	}
	reportRaw, _ := json.Marshal(h.Inventory)
	if err = writeEvidence(j.inventoryPath(), reportRaw); err != nil {
		return err
	}
	report := h.Inventory
	j.inventory = &report
	return nil
}
func (w *Worker) publishToolGeneration(h *toolGenerationHead) error {
	commands, err := w.mergeManagedCommands(h.Generation.Commands)
	if err != nil {
		return err
	}
	w.execMu.Lock()
	w.managedCommands = commands
	w.managedGenerationSHA = h.GenerationSHA256
	w.execMu.Unlock()
	w.setInventoryUnavailable()
	return nil
}
func (w *Worker) mergeManagedCommands(delta map[string]CommandSpec) (map[string]CommandSpec, error) {
	base, err := commandMapSnapshot(w.Exec.Commands)
	if err != nil {
		return nil, err
	}
	for id, spec := range delta {
		if _, exists := base[id]; exists {
			return nil, fmt.Errorf("managed command cannot replace a base command")
		}
		base[id] = spec
	}
	if len(base) > 128 {
		return nil, fmt.Errorf("command generation exceeds its bound")
	}
	raw, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	return DecodeCommands(raw)
}
func (w *Worker) restoreToolGeneration(ctx context.Context) error {
	if w.managedRestored {
		return nil
	}
	h, err := w.Journal.readToolGenerationHead()
	if err != nil {
		return err
	}
	if h == nil {
		// A node without managed state must first recover original inventory and
		// inspection ACKs. Its current configuration may intentionally be invalid
		// during that reconciliation; recovery is not a new configuration probe.
		w.managedRestored = true
		return nil
	}
	if err := w.initializeCommandGeneration(); err != nil {
		return err
	}
	if !w.AllowToolPreparation || !w.AllowInstalledToolDiscovery {
		return fmt.Errorf("managed tool generation requires original preparation opt-in")
	}
	scope, _ := InventoryScopeSHA256(w.Client.BaseURL, w.Client.ProjectID, w.ID)
	if h.Generation.ScopeSHA256 != scope || h.Generation.BaseSHA256 != w.managedBaseSHA || h.Generation.ProjectID != w.Client.ProjectID || h.Generation.WorkerID != w.ID {
		return fmt.Errorf("managed generation belongs to another original scope or base commands")
	}
	// Replay the ORIGINAL head claim. A fresh nonce could claim another request
	// while we are merely checking identity after restart.
	claim, err := w.Client.claimToolPreparation(ctx, w.ID, h.Generation.Preparation.ClaimID)
	if err != nil {
		return err
	}
	if claim.TenantID != h.Generation.TenantID || claim.Preparation == nil || preparationReference(*claim.Preparation) != h.Generation.Preparation || claim.Preparation.SourceSHA256 != h.Generation.SourceSHA256 {
		return fmt.Errorf("managed generation canonical scope differs")
	}
	if err = w.ensureAcceptedGeneration(ctx, h); err != nil {
		return err
	}
	w.managedRestored = true
	return nil
}

// Also used after a same-process write failure between head and mirror. The
// startup flag cannot stand in for this accepted snapshot being published.
func (w *Worker) ensureAcceptedGeneration(ctx context.Context, h *toolGenerationHead) error {
	scope, _ := InventoryScopeSHA256(w.Client.BaseURL, w.Client.ProjectID, w.ID)
	if h.Generation.ScopeSHA256 != scope || h.Generation.BaseSHA256 != w.managedBaseSHA || h.Generation.ProjectID != w.Client.ProjectID || h.Generation.WorkerID != w.ID || w.Client.Pool != nil && h.Generation.TenantID != w.Client.Pool.TenantID {
		return fmt.Errorf("accepted generation belongs to another scope or base")
	}
	delta, err := commandMapSnapshot(h.Generation.Commands)
	if err != nil {
		return err
	}
	h.Generation.Commands = delta
	for id := range delta {
		if _, owned := h.Generation.Profiles[id]; !owned {
			return fmt.Errorf("accepted generation contains an unowned command")
		}
	}
	commands, err := w.mergeManagedCommands(delta)
	if err != nil {
		return err
	}
	cfg := w.Exec
	cfg.Commands = commands
	if err = ValidateToolConfiguration(cfg); err != nil {
		return err
	}
	if err = w.validateGenerationAssets(ctx, h); err != nil {
		return err
	}
	w.Journal.mu.Lock()
	if !h.InventoryAcked {
		if w.Journal.inventory != nil && !w.Journal.inventory.Acked && w.Journal.inventory.OperationID != h.Inventory.OperationID {
			w.Journal.mu.Unlock()
			return fmt.Errorf("another inventory occupies accepted generation")
		}
		if w.Journal.inventory != nil && w.Journal.inventory.OperationID == h.Inventory.OperationID && w.Journal.inventory.Acked {
			h.Inventory = *w.Journal.inventory
			h.InventoryAcked = true
		} else {
			raw, _ := json.Marshal(h.Inventory)
			err = writeEvidence(w.Journal.inventoryPath(), raw)
			if err == nil {
				r := h.Inventory
				w.Journal.inventory = &r
			}
		}
	}
	w.Journal.mu.Unlock()
	if err != nil {
		return err
	}
	return w.publishToolGeneration(h)
}
func (w *Worker) validateGenerationAssets(ctx context.Context, h *toolGenerationHead) error {
	directory := filepath.Join(filepath.Dir(w.Journal.path), "tool-command-generations")
	if _, _, err := cursorPackageRoot(directory); err != nil {
		return fmt.Errorf("managed generation directory changed")
	}
	var recorded toolCommandGeneration
	if _, err := readToolProbeJSON(filepath.Join(directory, h.GenerationSHA256+".json"), &recorded); err != nil {
		return err
	}
	a, _ := json.Marshal(recorded)
	b, _ := json.Marshal(h.Generation)
	if string(a) != string(b) {
		return fmt.Errorf("immutable command generation differs")
	}
	var accepted toolLocalAcceptance
	sha, err := readToolProbeJSON(filepath.Join(directory, h.Generation.Preparation.PreparationID+"-accepted.json"), &accepted)
	if err != nil || sha != h.Receipt.LocalReceiptSHA256 || accepted.GenerationSHA256 != h.GenerationSHA256 || accepted.Preparation != h.Generation.Preparation || accepted.SourceSHA256 != h.Generation.SourceSHA256 {
		return fmt.Errorf("local generation acceptance receipt differs")
	}
	for id, profile := range h.Generation.Profiles {
		spec, ok := h.Generation.Commands[id]
		if !ok || profile.CommandID != id || tc.ValidateProfile(profile, true) != nil || spec.ACP == nil || validateACPExecutionSpec(spec) != nil || !sameACPProbeInstallation(ctx, spec.Executable, profile.ExecutableSHA256) {
			return fmt.Errorf("managed native generation changed")
		}
		sha, err := effectiveCommandProfileDigest(w.Exec, spec, profile.ExecutableSHA256)
		if err != nil || sha != profile.ProfileSHA256 {
			return fmt.Errorf("managed configuration generation changed")
		}
	}
	if len(h.Generation.Profiles) == 0 {
		return fmt.Errorf("managed generation has no accepted profiles")
	}
	artifact := filepath.Join(directory, h.Generation.Preparation.PreparationID+"-commands.json")
	var original map[string]CommandSpec
	if _, err = readToolProbeJSON(artifact, &original); err != nil {
		return err
	}
	data, err := os.ReadFile(artifact)
	if err != nil || digest(data) != h.Receipt.ArtifactSHA256 {
		return fmt.Errorf("managed artifact bytes changed")
	}
	if accepted.ArtifactSHA256 != h.Receipt.ArtifactSHA256 || !samePreparedProfile(accepted.Profile, h.Receipt.Profile) || tp.StopProofDigest(accepted.StopProof) != tp.StopProofDigest(*h.Receipt.StopProof) {
		return fmt.Errorf("managed acceptance facts differ")
	}
	return ctx.Err()
}
func samePreparedProfile(a, b tc.Profile) bool {
	rawA, _ := json.Marshal(a)
	rawB, _ := json.Marshal(b)
	return string(rawA) == string(rawB)
}

// Disabled execution must remain disabled. Accepted metadata can remain a
// complete, byte-pinned inactive observation; it never manufactures readiness.
func (w *Worker) describeToolSnapshot(ctx context.Context, cfg ExecConfig) ([]tc.Observation, error) {
	observations, err := DescribeTools(ctx, cfg)
	if err != nil || cfg.AllowExec {
		return observations, err
	}
	head, err := w.Journal.readToolGenerationHead()
	if err != nil {
		return nil, err
	}
	if head == nil {
		return observations, nil
	}
	for i, o := range observations {
		p, ok := head.Generation.Profiles[o.Profile.CommandID]
		if !ok {
			continue
		}
		spec := cfg.Commands[o.Profile.CommandID]
		if sameACPProbeInstallation(ctx, spec.Executable, p.ExecutableSHA256) {
			sha, e := effectiveCommandProfileDigest(cfg, spec, p.ExecutableSHA256)
			if e == nil && sha == p.ProfileSHA256 {
				observations[i].Profile = tc.CloneProfile(p)
			}
		}
	}
	return observations, nil
}
