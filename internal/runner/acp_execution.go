package runner

import (
	"context"
	"fmt"
	"io"
	"sync"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	"github.com/ai-mada/suma/internal/acpstdio"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

// Only the original Worker calls this bridge. Process creation, its tree,
// workspace, final candidate and durable progress remain owned by Execute.
type acpExecutionBridge struct {
	driver  *acpstdio.Driver
	profile tc.Profile
	output  *output
	mu      sync.Mutex
	events  int
	cfg     ExecConfig
}

func newACPExecutionBridge(ctx context.Context, a Assignment, cfg ExecConfig, profile tc.Profile, root string) (*acpExecutionBridge, error) {
	goal, ok := a.Config["goal"].(string)
	if profile.ACPExecution == nil || cfg.Journal == nil || !ok || goal == "" {
		return nil, fmt.Errorf("ACP requires the original durable worker inbox")
	}
	if err := cfg.Journal.beginToolPermissionExecution(a, profile); err != nil {
		return nil, err
	}
	b := &acpExecutionBridge{profile: tc.CloneProfile(profile), output: newOutput(cfg.OutputLimit, cfg.Secrets), cfg: cfg}
	var inputs *acpstdio.CursorInputHandler
	if profile.ACPExecution.Runtime == "cursor" {
		cfg.ToolInputProfile = profile.ProfileSHA256
		if profile.UserInputProtocol != cu.InputProtocol || cfg.ToolInputClient == nil || cfg.Journal.beginToolInputExecution(a, profile) != nil {
			return nil, errToolInput
		}
		h := cursorToolInputHandler(a, cfg)
		inputs = &h
	}
	driver, err := acpstdio.New(ctx, acpstdio.Options{Cwd: root, Runtime: profile.ACPExecution.Runtime, Goal: goal, Model: profile.ACPExecution.Model, ConfigSHA: profile.ACPExecution.ConfigurationSHA256, Secrets: cfg.Secrets, Emit: b.emit, Inputs: inputs, Permission: func(ctx context.Context, source ac.NativeSource) (ac.Choice, error) {
		safe, err := safeToolPermission(pc.Request{IdempotencyKey: operationID(), Protocol: ac.Protocol, ACP: &source}, cfg.Secrets)
		if err != nil {
			return ac.Choice{}, err
		}
		if err = cfg.Journal.recordToolPermission(a, safe); err != nil {
			return ac.Choice{}, err
		}
		if cfg.ToolPermission == nil {
			return ac.Choice{}, errToolPermission
		}
		decision, err := cfg.ToolPermission(ctx, safe)
		if err != nil || !pc.DecisionMatchesRequest(safe, decision) || decision.ACP == nil {
			return ac.Choice{}, errToolPermission
		}
		return *decision.ACP, nil
	}})
	if err != nil {
		return nil, fmt.Errorf("ACP SDK session could not initialize")
	}
	b.driver = driver
	return b, nil
}
func (b *acpExecutionBridge) emit(event acpstdio.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events++
	message := progressText(event.Text, b.cfg.Secrets)
	kind, status := "message", ""
	switch event.Kind {
	case "session":
		kind = "session_started"
	case "tool":
		kind = "activity_updated"
		status = event.ToolState
		if status == "pending" || status == "in_progress" {
			kind = "activity_started"
		} else if status == "completed" || status == "failed" {
			kind = "activity_completed"
		}
	case "permission", "input":
		kind = "waiting_input"
	case "message":
		if message != "" {
			_, _ = b.output.Write([]byte(message))
		}
	case "notice":
	default:
		return fmt.Errorf("ACP projected activity is invalid")
	}
	if b.cfg.ToolProgress != nil {
		e := tc.Event{SchemaVersion: 1, CommandID: b.profile.CommandID, ProfileSHA256: b.profile.ProfileSHA256, Protocol: b.profile.Protocol, Kind: kind, SessionID: event.SessionID, ItemID: event.ToolCallID, Message: message, Status: status}
		if err := tc.ValidateEvent(e); err != nil {
			return fmt.Errorf("ACP projected progress is invalid")
		}
		return b.cfg.ToolProgress(e)
	}
	return nil
}
func (b *acpExecutionBridge) Write(value []byte) (int, error) { return b.driver.Write(value) }
func (b *acpExecutionBridge) CopyInput(writer io.Writer) (int64, error) {
	return b.driver.CopyInput(writer)
}
func (b *acpExecutionBridge) Cancel() error { return b.driver.Cancel() }
func (b *acpExecutionBridge) Close() error  { return b.driver.Close() }
func (b *acpExecutionBridge) CanStopAfterRPC() bool {
	b.output.mu.Lock()
	truncated := b.output.truncated
	b.output.mu.Unlock()
	return !truncated && b.driver.CanStopAfterRPC()
}
func (b *acpExecutionBridge) finish(parentStopped, treeStopped bool, exitCode *int, afterRPC bool) *tc.Evidence {
	afterRPC = afterRPC && b.driver.CanStopAfterRPC()
	result := b.driver.Finish()
	proof := result.Evidence
	proof.HostProcessStopped = parentStopped
	proof.HostProcessTreeStopped = parentStopped && treeStopped
	proof.HostProcessExitCode = exitCode
	proof.HostStoppedAfterRPC = afterRPC && proof.HostProcessTreeStopped
	b.mu.Lock()
	count := b.events
	b.mu.Unlock()
	evidence := &tc.Evidence{Profile: b.profile, SessionID: proof.SessionID, Terminal: result.Terminal, EventCount: count, Usage: cloneUsage(result.Usage), UsageState: result.UsageState, ACPExecution: &proof}
	if evidence.UsageState != "partial" {
		evidence.UsageState = tc.UsageUnreported
		evidence.Usage = nil
	}
	if !parentStopped || !treeStopped || result.Error != nil {
		evidence.Terminal = "unknown"
	}
	if err := tc.ValidateEvidence(*evidence); err != nil {
		evidence.Terminal = "unknown"
	}
	return evidence
}
