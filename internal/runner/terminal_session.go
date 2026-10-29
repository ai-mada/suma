package runner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	term "github.com/ai-mada/suma/internal/terminalcontract"
	"github.com/ai-mada/suma/internal/terminalruntime"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// ExecuteTerminalSession is entered only for the distinct terminal_session DAG
// node. It owns a new attempt worktree and PTY for the entire process tree
// lifetime. Shell exit never becomes an agent candidate, test or acceptance.
func ExecuteTerminalSession(ctx context.Context, a Assignment, cfg ExecConfig) Result {
	r := Result{Assignment: a, ExitCode: -1}
	fail := func(err error) Result { r.Error = err.Error(); return r }
	if !cfg.AllowExec || cfg.TerminalClient == nil || a.Type != "terminal_session" || len(a.Command) != 1 || a.Idempotent || a.Attempt != 1 || a.Fence < 1 || a.DispatchID == "" {
		return fail(fmt.Errorf("Terminal execution requires its original non-retryable assignment and control client"))
	}
	client := cfg.TerminalClient
	if a.PoolID != "" {
		var err error
		client, err = client.forPoolAssignment(a)
		if err != nil {
			return fail(err)
		}
	}
	id, ok := a.Config["terminal_session_id"].(string)
	if !ok || id == "" || len(id) > 128 || strings.ContainsAny(id, "/\\\x00") {
		return fail(fmt.Errorf("Terminal session identity is invalid"))
	}
	spec, ok := cfg.Commands[a.Command[0]]
	spec = normalizedCommand(spec)
	if !ok || !spec.TerminalSession || spec.ReadOnly || spec.Adapter != "command" || spec.Input != "" || spec.NativeWorkspace != nil || spec.TestEvidence || spec.TestReport || spec.ArtifactInput || spec.ArtifactOutput != "" || spec.ModelExecutionProtocol != "" || len(spec.EnvFrom) != 0 || a.CodeBaseline != nil || hasCandidate(a) || a.CandidateConflictRepair != nil || a.CandidateComposition != nil || a.ReleaseExecution != nil || a.RepositoryIntegration != nil {
		return fail(fmt.Errorf("Terminal execution requires an isolated administrator shell profile"))
	}
	if _, hasModel := a.Config["model_execution"]; hasModel {
		return fail(fmt.Errorf("Terminal sessions cannot inherit a model gateway binding"))
	}
	for key, value := range spec.Env {
		if strings.Contains(strings.ToUpper(key), "SUMA_") || credential.MatchString(key+"="+value) {
			return fail(fmt.Errorf("Terminal profile cannot forward execution credentials"))
		}
	}
	profile, executable, err := launchProfile(ctx, a, cfg, spec)
	if err != nil {
		return fail(err)
	}
	if !tc.SupportsTerminalSessionProfile(profile) {
		return fail(fmt.Errorf("Terminal profile does not have a current real PTY capability"))
	}
	r.ToolEvidence = &tc.Evidence{Profile: profile, SessionID: id, Terminal: "failed", UsageState: tc.UsageUnreported}
	request := term.ExchangeRequest{SessionID: id, RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, DispatchID: a.DispatchID, OperationID: operationID(), OutputChunks: []term.OutputChunk{}, InputReceipts: []term.InputReceipt{}, State: "starting", ExitCode: -1}
	startCtx, startCancel := context.WithTimeout(ctx, 10*time.Second)
	authority, err := client.TerminalExchange(startCtx, request)
	startCancel()
	if err != nil {
		return fail(err)
	}
	if authority.StopRequested {
		return fail(fmt.Errorf("Terminal execution was stopped before starting"))
	}
	workspace, err := prepareWorkspace(ctx, a, cfg, spec)
	if err != nil {
		return fail(err)
	}
	env, err := terminalEnvironment(spec, workspace.Directory)
	if err != nil {
		return fail(err)
	}
	// Recheck the executable bytes and complete profile after preparation. An
	// administrator edit between inventory and launch cannot silently change it.
	latestExecutable, latestSHA, err := commandExecutable(spec)
	if err != nil {
		return fail(err)
	}
	latestDigest, err := effectiveCommandProfileDigest(cfg, spec, latestSHA)
	if err != nil || latestExecutable != executable || latestDigest != profile.ProfileSHA256 {
		return fail(fmt.Errorf("Terminal shell profile changed before process launch"))
	}
	duration := time.Duration(a.TimeoutSeconds) * time.Second
	if duration < time.Second || duration > 2*time.Hour {
		return fail(fmt.Errorf("Terminal duration must be between 1 and 7200 seconds"))
	}
	closeObservation := func() {}
	if cfg.WorkspaceObservations != nil {
		readConfig := cfg
		readConfig.Secrets = append(append([]string{}, cfg.Secrets...), client.Token)
		reader, readErr := openLiveWorkspaceRead(ctx, a, readConfig, workspace, profile.ProfileSHA256)
		if readErr != nil {
			return fail(fmt.Errorf("Terminal workspace observation could not be opened: %w", readErr))
		}
		unregister, readErr := cfg.WorkspaceObservations.registerRead(reader, nil)
		if readErr != nil {
			_ = reader.Close()
			return fail(fmt.Errorf("Terminal workspace observation registration failed: %w", readErr))
		}
		closed := false
		closeObservation = func() {
			if !closed {
				closed = true
				reader.cancel()
				unregister()
				_ = reader.Close()
			}
		}
		defer closeObservation()
	}
	session, err := terminalruntime.Start(terminalruntime.Config{ID: id, Executable: executable, Args: append([]string{}, spec.Args...), WorkDir: workspace.Source, Env: env, JournalDir: filepath.Join(workspace.Directory, "terminal-journal"), Columns: authority.Columns, Rows: authority.Rows, MaxDuration: duration})
	if err != nil {
		return fail(err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = session.Stop(stopCtx)
	}()
	var ack uint64
	var pendingReceipts []term.InputReceipt
	unknownInputs := map[string]bool{}
	var terminalError error
	controlEpoch := authority.ControlEpoch
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		state := session.Snapshot()
		for inputID := range unknownInputs {
			if settled, ok := session.InputReceipt(inputID); ok && settled.State == "delivered" {
				pendingReceipts = append(pendingReceipts, term.InputReceipt{InputID: settled.InputID, Sequence: int64(settled.Sequence), SHA256: settled.SHA256, State: settled.State, BytesWritten: settled.BytesWritten})
				delete(unknownInputs, inputID)
			}
		}
		page := session.ReadOutput(ack, 128*1024)
		if len(page.Chunks) > 32 {
			page.Chunks = page.Chunks[:32]
			page.NextSequence = page.Chunks[len(page.Chunks)-1].Sequence
		}
		if page.Gap {
			terminalError = fmt.Errorf("Terminal output acknowledgement fell behind retained history")
			break
		}
		request = term.ExchangeRequest{SessionID: id, RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, DispatchID: a.DispatchID, OperationID: operationID(), OutputChunks: []term.OutputChunk{}, InputReceipts: append([]term.InputReceipt{}, pendingReceipts...), State: state.State, Directory: workspace.Source, ProcessID: state.ProcessID, TreeStopped: state.TreeStopped, ExitCode: -1, Error: state.Reason}
		if state.ExitCode != nil {
			request.ExitCode = *state.ExitCode
		}
		if len(unknownInputs) > 0 && (state.State == "exited" || state.State == "stopped" || state.State == "reconciling") {
			request.State = "reconciling"
			request.Error = terminalruntime.ErrInputUnknown.Error()
		}
		for _, chunk := range page.Chunks {
			if len(request.OutputChunks) >= 32 {
				break
			}
			request.OutputChunks = append(request.OutputChunks, term.OutputChunk{Sequence: int64(chunk.Sequence), DataBase64: base64.StdEncoding.EncodeToString(chunk.Data)})
		}
		response, e := terminalExchangeOriginal(ctx, client, request)
		if e != nil {
			terminalError = e
			break
		}
		if response.OutputAckSequence < int64(ack) || response.OutputAckSequence > int64(page.NextSequence) {
			terminalError = fmt.Errorf("Terminal output acknowledgement differs from original sent bytes")
			break
		}
		ack = uint64(response.OutputAckSequence)
		pendingReceipts = nil
		if response.ControlEpoch < controlEpoch {
			terminalError = fmt.Errorf("Terminal control epoch moved backwards")
			break
		}
		controlEpoch = response.ControlEpoch
		if state.State == "exited" || state.State == "stopped" || state.State == "reconciling" {
			// Flush all original output pages before returning its final state.
			if ack >= state.LastOutputSequence {
				break
			}
			continue
		}
		if response.StopRequested {
			stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			e = session.Stop(stopCtx)
			cancel()
			if e != nil && !errors.Is(e, terminalruntime.ErrTreeUnknown) {
				terminalError = e
				break
			}
			continue
		}
		if response.Columns != state.Columns || response.Rows != state.Rows {
			if e = session.Resize(response.Columns, response.Rows); e != nil {
				terminalError = e
				break
			}
		}
		for _, input := range response.Inputs {
			data, decodeErr := term.DecodeBytes(input.DataBase64, term.MaxInputBytes)
			if decodeErr != nil || input.Sequence < 1 || digest(data) != input.SHA256 {
				terminalError = fmt.Errorf("Terminal input differs from its original accepted bytes")
				break
			}
			inputCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			receipt, writeErr := session.SubmitInput(inputCtx, input.InputID, uint64(input.Sequence), data)
			cancel()
			if receipt.InputID != "" {
				pendingReceipts = append(pendingReceipts, term.InputReceipt{InputID: receipt.InputID, Sequence: int64(receipt.Sequence), SHA256: receipt.SHA256, State: receipt.State, BytesWritten: receipt.BytesWritten})
			}
			if errors.Is(writeErr, terminalruntime.ErrInputUnknown) {
				unknownInputs[input.InputID] = true
			} else if writeErr == nil {
				delete(unknownInputs, input.InputID)
			}
			if writeErr != nil && !errors.Is(writeErr, terminalruntime.ErrInputUnknown) {
				terminalError = writeErr
				break
			}
			if errors.Is(writeErr, terminalruntime.ErrInputUnknown) {
				break
			}
		}
		if terminalError != nil {
			break
		}
		select {
		case <-ctx.Done():
			terminalError = ctx.Err()
		case <-session.Done():
			continue
		case <-ticker.C:
			continue
		}
		break
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	stopErr := session.Stop(stopCtx)
	cancel()
	state := session.Snapshot()
	closeObservation()
	if state.ExitCode != nil {
		r.ExitCode = *state.ExitCode
	}
	r.Stopped = state.TreeStopped && state.State == "stopped"
	r.OutcomeUnknown = !state.TreeStopped || state.State == "reconciling" || len(unknownInputs) > 0
	if len(unknownInputs) > 0 {
		terminalError = errors.Join(terminalError, terminalruntime.ErrInputUnknown)
	}
	if terminalError != nil || stopErr != nil {
		r.Error = errors.Join(terminalError, stopErr).Error()
	}
	if state.Reason != "" && r.Error == "" {
		r.Error = state.Reason
	}
	// Record a local workspace snapshot only after actual process-tree stop.
	// Returning this map as WorkspaceEvidence would falsely make shell activity
	// eligible as a native developer candidate, so it deliberately stays local.
	if state.TreeStopped {
		captureCtx, captureCancel := context.WithTimeout(context.Background(), 30*time.Second)
		local, inspectErr := workspace.capture(captureCtx, r)
		captureCancel()
		if inspectErr != nil {
			r.OutcomeUnknown = true
			r.Error = errors.Join(terminalError, inspectErr).Error()
		} else {
			raw, _ := json.Marshal(map[string]any{"terminal_session_id": id, "directory": workspace.Source, "head_commit": local["head_commit"], "tree_stopped": true, "candidate": false, "test_evidence": false, "human_acceptance": false})
			r.Output = string(raw)
		}
	}
	if r.OutcomeUnknown {
		r.ToolEvidence.Terminal = "unknown"
	} else if r.ExitCode == 0 && r.Error == "" && !r.Stopped {
		r.ToolEvidence.Terminal = "completed"
	}
	return r
}

// Only transport failures retry, with exactly the same operation and bytes.
// An HTTP authorization/conflict response stops input immediately.
func terminalExchangeOriginal(ctx context.Context, client *Client, request term.ExchangeRequest) (term.ExchangeResponse, error) {
	var response term.ExchangeResponse
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		response, err = client.TerminalExchange(callCtx, request)
		cancel()
		if err == nil {
			return response, nil
		}
		var status *runnerHTTPStatusError
		if errors.As(err, &status) {
			return response, err
		}
		if attempt == 2 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return response, ctx.Err()
		case <-timer.C:
		}
	}
	return response, err
}

// Give the shell an empty private configuration home and task-local temp. The
// standard command environment helper contributes only known OS essentials;
// no provider, Runner, model gateway, or user shell profile state is inherited.
func terminalEnvironment(spec CommandSpec, directory string) ([]string, error) {
	home := filepath.Join(directory, "terminal-home")
	tmp := filepath.Join(directory, "terminal-tmp")
	for _, dir := range []string{home, tmp, filepath.Join(home, "config"), filepath.Join(home, "data")} {
		if err := safeMkdir(dir); err != nil {
			return nil, err
		}
	}
	env, _, err := commandEnvironment(CommandSpec{Env: spec.Env})
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	canonical := func(key string) string {
		if runtime.GOOS == "windows" {
			return strings.ToUpper(key)
		}
		return key
	}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		values[canonical(key)] = value
	}
	for key, value := range map[string]string{"HOME": home, "USERPROFILE": home, "APPDATA": filepath.Join(home, "config"), "LOCALAPPDATA": filepath.Join(home, "data"), "XDG_CONFIG_HOME": filepath.Join(home, "config"), "XDG_DATA_HOME": filepath.Join(home, "data"), "XDG_CACHE_HOME": filepath.Join(home, "cache"), "TEMP": tmp, "TMP": tmp, "TMPDIR": tmp, "TERM": "xterm-256color"} {
		values[canonical(key)] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := []string{}
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result, nil
}
