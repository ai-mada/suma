package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	cu "github.com/ai-mada/suma/internal/cursorcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

// Called by the one inventory owner. It writes only fixed tool configurations;
// browser input cannot select a path, argv, command ID or executable.
func (w *Worker) serviceToolPreparation(ctx context.Context) (bool, error) {
	p, err := w.Journal.preparationCheckpoint()
	if err != nil {
		return true, err
	}
	if !w.AllowToolPreparation {
		if p != nil && !p.Complete {
			return true, fmt.Errorf("original tool preparation requires its local opt-in")
		}
		return false, nil
	}
	if !w.AllowInstalledToolDiscovery {
		return true, fmt.Errorf("tool preparation requires installed discovery")
	}
	scope, _ := InventoryScopeSHA256(w.Client.BaseURL, w.Client.ProjectID, w.ID)
	if p != nil && (p.ScopeSHA256 != scope || p.WorkerID != w.ID) {
		return true, fmt.Errorf("tool preparation belongs to another machine scope")
	}
	if p == nil || p.Complete {
		if !w.canClaimInventory() {
			return false, nil
		}
		if p != nil && p.Operation != nil {
			if err = w.Journal.archivePreparation(*p); err != nil {
				return true, err
			}
		}
		p = &toolPreparationCheckpoint{ScopeSHA256: scope, WorkerID: w.ID, ClaimID: operationID()}
		if err = w.Journal.savePreparation(*p); err != nil {
			return true, err
		}
	}
	if !p.Responded {
		claim, e := w.Client.claimToolPreparation(ctx, w.ID, p.ClaimID)
		if e != nil {
			return true, e
		}
		p.Responded, p.TenantID, p.Operation = true, claim.TenantID, claim.Preparation
		if p.Operation == nil {
			p.Complete = true
		} else if w.maintenance == nil || p.Operation.InstanceID != w.maintenance.InstanceID {
			return true, fmt.Errorf("tool preparation logical owner differs")
		} else if p.Operation.State == "failed" || p.Operation.State == "expired" || p.Operation.State == "superseded" || p.Operation.State == "closed_unknown" {
			p.Complete = true
		}
		if err = w.Journal.savePreparation(*p); err != nil {
			return true, err
		}
	}
	if p.Complete {
		return false, nil
	}
	w.setInventoryUnavailable()
	if p.Result != nil {
		if !p.ResultAcked {
			return true, w.deliverToolPreparation(ctx, p, p.Result, false)
		}
		if p.Result.Outcome == "prepared" {
			return true, w.finishPreparedInventory(ctx, p)
		}
		if p.Closure != nil {
			return true, w.deliverToolPreparation(ctx, p, p.Closure, true)
		}
		if p.Result.Outcome == "unknown" {
			proof, e := w.Journal.preparationStopProof(*p)
			if e != nil {
				return true, nil
			}
			closure := *p.Result
			closure.IdempotencyKey = operationID()
			closure.Outcome = "closed_unknown"
			closure.StopProof = &proof
			p.Closure = &closure
			if err = w.Journal.savePreparation(*p); err != nil {
				return true, err
			}
			return true, w.deliverToolPreparation(ctx, p, p.Closure, true)
		}
		return true, fmt.Errorf("unfinished original preparation result")
	}
	// If the head was accepted immediately before a crash, recover its original
	// inventory and receipt. File presence alone is never used as completion.
	if p.Started || p.Operation.State != "claimed" {
		if !p.Started {
			p.Started = true
			setPreparationSourceIntent(p)
		}
		head, e := w.Journal.readToolGenerationHead()
		if e != nil {
			return true, e
		}
		if head != nil && head.Generation.Preparation == preparationReference(*p.Operation) && head.Generation.SourceSHA256 == p.Operation.SourceSHA256 {
			if head.Generation.TenantID != p.TenantID {
				return true, fmt.Errorf("accepted preparation tenant differs")
			}
			if e = w.ensureAcceptedGeneration(ctx, head); e != nil {
				return true, e
			}
			p.Result = w.preparationResult(*p, "prepared", "", &head.Receipt, nil)
		} else {
			p.Result = w.preparationResult(*p, "unknown", "worker_restarted", nil, nil)
		}
		if err = w.Journal.savePreparation(*p); err != nil {
			return true, err
		}
		return true, w.deliverToolPreparation(ctx, p, p.Result, false)
	}
	return true, w.prepareManagedTool(ctx, p)
}
func (w *Worker) preparationResult(p toolPreparationCheckpoint, outcome, code string, receipt *tp.GenerationReceipt, proof *tp.StopProof) *tp.ResultRequest {
	return &tp.ResultRequest{ProjectID: w.Client.ProjectID, WorkerID: w.ID, Preparation: preparationReference(*p.Operation), SourceSHA256: p.Operation.SourceSHA256, IdempotencyKey: operationID(), Outcome: outcome, FailureCode: code, Receipt: receipt, StopProof: proof}
}
func (w *Worker) deliverToolPreparation(ctx context.Context, p *toolPreparationCheckpoint, r *tp.ResultRequest, closure bool) error {
	_, err := w.Client.resultToolPreparation(ctx, *p.Operation, *r)
	if err != nil {
		return err
	}
	if closure {
		p.Complete = true
	} else {
		p.ResultAcked = true
		p.Complete = r.Outcome == "failed"
	}
	if err = w.Journal.savePreparation(*p); err != nil {
		return err
	}
	if p.Complete {
		w.inventoryNext = time.Time{}
	}
	return nil
}
func (w *Worker) finishPreparedInventory(ctx context.Context, p *toolPreparationCheckpoint) error {
	head, err := w.Journal.readToolGenerationHead()
	if err != nil {
		return err
	}
	if head == nil || head.Generation.Preparation != preparationReference(*p.Operation) || head.Receipt.LocalReceiptSHA256 != p.Result.Receipt.LocalReceiptSHA256 {
		return fmt.Errorf("accepted preparation head differs")
	}
	if err = w.ensureAcceptedGeneration(ctx, head); err != nil {
		return err
	}
	report := head.Inventory
	// AckInventory may have reached disk before the head's mirror update.
	w.Journal.mu.Lock()
	saved := w.Journal.inventory
	if saved != nil && saved.OperationID == report.OperationID && saved.Acked {
		report = *saved
	}
	w.Journal.mu.Unlock()
	recovered := report.Acked || head.InventoryStarted
	if !head.InventoryStarted {
		head.InventoryStarted = true
		raw, _ := json.Marshal(head)
		w.Journal.mu.Lock()
		err = writeEvidence(w.Journal.toolGenerationHeadPath(), raw)
		w.Journal.mu.Unlock()
		if err != nil {
			return err
		}
	}
	if !report.Acked {
		report, err = w.acknowledgeInventory(ctx, report)
		if err != nil {
			return err
		}
	}
	head.Inventory, head.InventoryAcked = report, true
	raw, _ := json.Marshal(head)
	w.Journal.mu.Lock()
	err = writeEvidence(w.Journal.toolGenerationHeadPath(), raw)
	w.Journal.mu.Unlock()
	if err != nil {
		return err
	}
	p.Complete = true
	if err = w.Journal.savePreparation(*p); err != nil {
		return err
	}
	// Restart/recovered ACK is a past observation, not a new TTL. A normal
	// current periodic observation may reopen the gate on the following tick.
	w.inventoryNext = time.Time{}
	if recovered {
		w.setInventoryUnavailable()
	} else {
		w.acceptInventory(report)
	}
	return nil
}
func (w *Worker) prepareManagedTool(ctx context.Context, p *toolPreparationCheckpoint) error {
	op := *p.Operation
	setPreparationSourceIntent(p)
	code := ""
	deadline, _ := time.Parse(time.RFC3339Nano, op.LeaseUntil)
	if ctx.Err() != nil || !time.Now().Before(deadline) {
		code = "not_started"
	}
	if op.ExpectedGenerationSHA256 != w.currentGenerationSHA() {
		code = "generation_changed"
	}
	if op.AdapterID == managedCodexAdapterID && code == "" {
		if _, ok := managedCodexTemplate(w.Exec); !ok {
			code = "preparation_not_supported"
		}
	}
	var native installationNativeObservation
	if code == "" {
		var err error
		native, err = w.Journal.preparationNative(ctx, p.ScopeSHA256, op)
		if err != nil {
			code = "native_source_changed"
			if err.Error() == "rescan_required" {
				code = "rescan_required"
			}
		} else {
			switch {
			case op.AdapterID == managedCodexAdapterID:
				p.InstalledIntent, err = w.managedCodexPreparationIntent(op, native)
			case genericInstalledAdapter(op.AdapterID):
				p.InstalledIntent, err = installedPreparationIntent(op, native)
			default:
				p.Intent.Executable = native.Executable
			}
			if err != nil {
				code = "configuration_invalid"
				setPreparationSourceIntent(p)
			}
		}
	}
	// Persist the exact typed intent before ANY new file or native process.
	p.Started = true
	if err := w.Journal.savePreparation(*p); err != nil {
		return err
	}
	directory, source, err := w.Journal.preparationProbeDirectory(*p)
	if err != nil {
		return err
	}
	collector, err := newToolProbeCollectorAt(directory, source)
	if err != nil {
		return err
	}
	probeCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	probeCtx = collector.context(probeCtx)
	var head *toolGenerationHead
	if code == "" {
		head, err = w.buildManagedGeneration(probeCtx, *p, native)
		if err != nil {
			code = "preparation_failed"
		}
	}
	interrupted := probeCtx.Err()
	if e := collector.closeReturned(); e != nil {
		err = e
	}
	proof, proofErr := w.Journal.preparationStopProof(*p)
	if err != nil || proofErr != nil || interrupted != nil || collector.hasUnverifiedNativeExit() {
		if proofErr == nil && interrupted == nil && code != "" {
			p.Result = w.preparationResult(*p, "failed", code, nil, &proof)
		} else {
			unknown := "preparation_interrupted"
			if interrupted == context.DeadlineExceeded {
				unknown = "preparation_timeout"
			}
			p.Result = w.preparationResult(*p, "unknown", unknown, nil, nil)
		}
	} else if code != "" {
		p.Result = w.preparationResult(*p, "failed", code, nil, &proof)
	} else {
		head.Receipt.StopProof = &proof
		if err = w.persistManagedGeneration(*p, head); err != nil {
			return err
		}
		if err = w.publishToolGeneration(head); err != nil {
			return err
		}
		p.Result = w.preparationResult(*p, "prepared", "", &head.Receipt, nil)
	}
	if err = w.Journal.savePreparation(*p); err != nil {
		return err
	}
	return w.deliverToolPreparation(ctx, p, p.Result, false)
}
func (w *Worker) buildManagedGeneration(ctx context.Context, p toolPreparationCheckpoint, native installationNativeObservation) (*toolGenerationHead, error) {
	directory, err := w.Journal.maintenanceDirectory("tool-command-generations")
	if err != nil {
		return nil, err
	}
	artifact := filepath.Join(directory, p.Operation.ID+"-commands.json")
	if p.InstalledIntent != nil {
		if err = validateInstalledPreparationIntent(*p.Operation, *p.InstalledIntent); err != nil {
			return nil, err
		}
		err = writeInstalledToolPreparation(ctx, artifact, *p.InstalledIntent)
	} else {
		raw, _ := json.Marshal(p.Intent)
		_, err = PrepareACPToolCommands(ctx, artifact, bytes.NewReader(raw))
	}
	if err != nil {
		return nil, err
	}
	// Recovering this file is NOT retrying preparation: this call occurs once,
	// after the original writer returned successfully in the same started turn.
	data, err := os.ReadFile(artifact)
	if err != nil || len(data) > 2<<20 {
		return nil, fmt.Errorf("prepared artifact unavailable")
	}
	commands, err := DecodeCommands(data)
	if err != nil || len(commands) != 1 {
		return nil, fmt.Errorf("prepared artifact differs")
	}
	var spec CommandSpec
	for _, s := range commands {
		spec = s
	}
	if !sameACPProbeInstallation(ctx, native.Executable, native.Item.ExecutableSHA256) {
		return nil, fmt.Errorf("prepared native installation changed")
	}
	current := w.currentExecSnapshot()
	merged, err := commandMapSnapshot(current.Commands)
	if err != nil {
		return nil, err
	}
	id := tp.CommandID(p.Operation.ID)
	if _, exists := merged[id]; exists || len(merged) >= 128 {
		return nil, fmt.Errorf("managed command generation is full or already occupied")
	}
	merged[id] = spec
	current.Commands = merged
	if err = ValidateToolConfiguration(current); err != nil {
		return nil, err
	}
	observations, err := w.describeToolSnapshot(ctx, current)
	if err != nil {
		return nil, err
	}
	var profile tc.Profile
	for i, item := range observations {
		if item.Profile.CommandID == id {
			profile = item.Profile
			if p.Operation.AdapterID == managedCodexAdapterID &&
				(!item.Available || item.CodexCatalog == nil || tc.ValidateObservation(item) != nil ||
					profile.ExecutableSHA256 != native.Item.ExecutableSHA256 || profile.ToolVersion != native.Item.Version) {
				return nil, fmt.Errorf("prepared Codex app-server catalog or scanned identity is unavailable")
			}
			if !current.AllowExec {
				profile, err = observePreparedTool(ctx, current, id, spec)
				if err != nil {
					return nil, err
				}
				observations[i].Profile = profile // Available/reason retain the real local gate.
			}
			break
		}
	}
	if profile.CommandID != id || tc.ValidateProfile(profile, true) != nil {
		return nil, fmt.Errorf("prepared tool did not produce a complete bound profile")
	}
	profiles := map[string]tc.Profile{}
	delta := map[string]CommandSpec{}
	prior, e := w.Journal.readToolGenerationHead()
	if e != nil {
		return nil, e
	}
	if prior != nil {
		for id, p := range prior.Generation.Profiles {
			profiles[id] = tc.CloneProfile(p)
		}
		for id, s := range prior.Generation.Commands {
			delta[id] = s
		}
	}
	profiles[id] = tc.CloneProfile(profile)
	delta[id] = spec
	generation := toolCommandGeneration{Protocol: tp.Protocol, ScopeSHA256: p.ScopeSHA256, TenantID: p.TenantID, ProjectID: w.Client.ProjectID, WorkerID: w.ID, BaseSHA256: w.managedBaseSHA, ParentSHA256: p.Operation.ExpectedGenerationSHA256, Preparation: preparationReference(*p.Operation), SourceSHA256: p.Operation.SourceSHA256, Commands: delta, Profiles: profiles}
	generationRaw, _ := json.Marshal(generation)
	if len(generationRaw) > 2<<20 {
		return nil, fmt.Errorf("managed generation exceeds its bound")
	}
	sha := digest(generationRaw)
	receipt := tp.GenerationReceipt{ParentGenerationSHA256: generation.ParentSHA256, GenerationSHA256: sha, CommandID: id, ArtifactSHA256: digest(data), Profile: profile}
	h := &toolGenerationHead{GenerationSHA256: sha, Generation: generation, Receipt: receipt, Inventory: InventoryReport{WorkerID: w.ID, OperationID: operationID(), Items: cloneObservations(observations), ScopeSHA256: p.ScopeSHA256}}
	return h, nil
}

// The preparation probe observes only the original ACP metadata path. It does
// not flip AllowExec, inspect provider login files, initialize ACP or call a model.
func observePreparedTool(ctx context.Context, cfg ExecConfig, id string, spec CommandSpec) (tc.Profile, error) {
	profile := tc.Profile{CommandID: id, Adapter: spec.Adapter, Protocol: spec.Protocol, ReadOnly: spec.ReadOnly}
	executable, sha, err := commandExecutable(spec)
	if err != nil {
		return profile, err
	}
	profile.ExecutableSHA256 = sha
	profile.ProfileSHA256, err = effectiveCommandProfileDigest(cfg, spec, sha)
	if err != nil {
		return profile, err
	}
	profile.ToolVersion, err = toolVersion(ctx, executable, spec, cfg.Secrets, cfg.Workspace)
	if err != nil {
		return profile, err
	}
	if spec.ACP != nil {
		profile.ACPExecution, err = localACPCapability(spec)
		if err != nil {
			return profile, err
		}
		if spec.ACP.Runtime == "cursor" {
			profile.UserInputProtocol = cu.InputProtocol
		}
	} else {
		if spec.InstalledCommand == nil || validateInstalledCommandPins(ctx, spec) != nil {
			return profile, fmt.Errorf("installed tool metadata has no exact source pins")
		}
		profile.ClaudeExecution, err = localClaudeCapability(spec)
		if err != nil {
			return profile, err
		}
	}
	if tc.ValidateProfile(profile, true) != nil {
		return profile, fmt.Errorf("incomplete prepared tool metadata")
	}
	return profile, nil
}

func setPreparationSourceIntent(p *toolPreparationCheckpoint) {
	op := p.Operation
	if usesInstalledPreparationIntent(op.AdapterID) {
		p.Intent = nil
		p.InstalledIntent = &installedToolPreparationInput{AdapterID: op.AdapterID}
		return
	}
	p.InstalledIntent = nil
	p.Intent = &ACPToolPreparationInput{AdapterID: op.AdapterID, ReviewedVersion: op.InstallationItem.Version, Model: op.Configuration.Model, CredentialEnvRef: op.Configuration.CredentialEnvRef}
}
func (w *Worker) persistManagedGeneration(p toolPreparationCheckpoint, h *toolGenerationHead) error {
	dir, err := w.Journal.maintenanceDirectory("tool-command-generations")
	if err != nil {
		return err
	}
	if _, err = writeToolProbeImmutable(filepath.Join(dir, h.GenerationSHA256+".json"), h.Generation); err != nil {
		return err
	}
	acceptance := toolLocalAcceptance{GenerationSHA256: h.GenerationSHA256, Preparation: preparationReference(*p.Operation), SourceSHA256: p.Operation.SourceSHA256, ArtifactSHA256: h.Receipt.ArtifactSHA256, Profile: h.Receipt.Profile, StopProof: *h.Receipt.StopProof}
	sha, err := writeToolProbeImmutable(filepath.Join(dir, p.Operation.ID+"-accepted.json"), acceptance)
	if err != nil {
		return err
	}
	h.Receipt.LocalReceiptSHA256 = sha
	if tp.ValidateGenerationReceipt(h.Receipt) != nil {
		return fmt.Errorf("invalid local accepted generation receipt")
	}
	h.Inventory.Preparation = &tp.InventoryBinding{Reference: preparationReference(*p.Operation), SourceSHA256: p.Operation.SourceSHA256, GenerationSHA256: h.GenerationSHA256, LocalReceiptSHA256: sha}
	h.Inventory.Maintenance = w.toolMaintenanceForGeneration(h.GenerationSHA256)
	return w.Journal.acceptToolGeneration(*h)
}
