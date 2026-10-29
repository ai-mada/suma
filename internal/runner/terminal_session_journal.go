package runner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	term "github.com/ai-mada/suma/internal/terminalcontract"
	"github.com/ai-mada/suma/internal/terminalruntime"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// This is a transport outbox under the original durable assignment. Recovery
// sends only already captured facts and discards any returned input commands.
type terminalCheckpoint struct {
	Pending                   []term.ExchangeRequest `json:"pending"`
	JournalDirectory          string                 `json:"journal_directory"`
	Directory                 string                 `json:"directory"`
	Profile                   *tc.Profile            `json:"profile"`
	Recovered                 bool                   `json:"recovered"`
	ReconciliationOperationID string                 `json:"reconciliation_operation_id"`
	Unapplied                 []term.ExchangeRequest `json:"unapplied"`
	LastExchange              *term.ExchangeRequest  `json:"last_exchange"`
}

func validTerminalExchangeAssignment(a Assignment, r term.ExchangeRequest) bool {
	id, _ := a.Config["terminal_session_id"].(string)
	return a.Type == "terminal_session" && a.Attempt == 1 && a.Fence > 0 && a.DispatchID != "" && r.SessionID == id && id != "" && r.RunID == a.RunID && r.NodeID == a.NodeID && r.Attempt == a.Attempt && r.Fence == a.Fence && r.WorkerID == a.WorkerID && r.DispatchID == a.DispatchID && r.OperationID != ""
}
func cloneTerminalExchange(r term.ExchangeRequest) term.ExchangeRequest {
	raw, _ := json.Marshal(r)
	var out term.ExchangeRequest
	_ = json.Unmarshal(raw, &out)
	return out
}

func (j *Journal) checkpointTerminalExchange(a Assignment, r term.ExchangeRequest) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || key(e.Assignment) != k || e.Assignment.WorkerID != a.WorkerID || !validTerminalExchangeAssignment(e.Assignment, r) {
		return fmt.Errorf("terminal exchange differs from original durable assignment")
	}
	previous := e
	p := terminalCheckpoint{Pending: []term.ExchangeRequest{}}
	if e.Terminal != nil {
		p = *e.Terminal
		p.Pending = append([]term.ExchangeRequest{}, e.Terminal.Pending...)
	}
	if p.ReconciliationOperationID != "" {
		return fmt.Errorf("original terminal has an explicit reviewed reconciliation disposition")
	}
	for _, old := range p.Pending {
		if old.OperationID == r.OperationID {
			if stringifyTerminalExchange(old) != stringifyTerminalExchange(r) {
				return fmt.Errorf("terminal operation changed its original facts")
			}
			return nil
		}
	}
	if len(p.Pending) >= 256 {
		return fmt.Errorf("terminal transport outbox limit reached; original facts retained")
	}
	p.Pending = append(p.Pending, cloneTerminalExchange(r))
	e.Terminal = &p
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = previous
		return err
	}
	return nil
}
func stringifyTerminalExchange(r term.ExchangeRequest) string {
	raw, _ := json.Marshal(r)
	return string(raw)
}

func (j *Journal) ackTerminalExchange(a Assignment, id string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.Terminal == nil || len(e.Terminal.Pending) == 0 || e.Terminal.Pending[0].OperationID != id {
		return fmt.Errorf("terminal exchange acknowledgement differs from original outbox")
	}
	previous := e
	p := *e.Terminal
	last := cloneTerminalExchange(p.Pending[0])
	p.LastExchange = &last
	p.Pending = append([]term.ExchangeRequest{}, p.Pending[1:]...)
	e.Terminal = &p
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = previous
		return err
	}
	return nil
}

type pendingTerminalExchange struct {
	Assignment Assignment
	Request    term.ExchangeRequest
}

func (j *Journal) pendingTerminalExchanges() []pendingTerminalExchange {
	j.mu.Lock()
	defer j.mu.Unlock()
	keys := []string{}
	for k, e := range j.entries {
		if e.Terminal != nil && len(e.Terminal.Pending) > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := []pendingTerminalExchange{}
	for _, k := range keys {
		e := j.entries[k]
		for _, r := range e.Terminal.Pending {
			out = append(out, pendingTerminalExchange{cloneModelAssignment(e.Assignment), cloneTerminalExchange(r)})
		}
	}
	return out
}
func (j *Journal) validateTerminalCheckpoint(a Assignment, p terminalCheckpoint) error {
	if len(p.Pending) > 256 || len(p.Unapplied) > 256 || len(p.ReconciliationOperationID) > 128 || p.ReconciliationOperationID != "" && len(p.Pending) > 0 {
		return fmt.Errorf("terminal outbox limit exceeded or reviewed disposition changed")
	}
	seen := map[string]bool{}
	for _, r := range p.Pending {
		if !validTerminalExchangeAssignment(a, r) || seen[r.OperationID] {
			return fmt.Errorf("terminal checkpoint differs from original inbox")
		}
		seen[r.OperationID] = true
	}
	for _, r := range p.Unapplied {
		if !validTerminalExchangeAssignment(a, r) || seen[r.OperationID] {
			return fmt.Errorf("terminal unapplied facts differ from original inbox")
		}
		seen[r.OperationID] = true
	}
	if p.LastExchange != nil && !validTerminalExchangeAssignment(a, *p.LastExchange) {
		return fmt.Errorf("terminal original last exchange changed")
	}
	if p.JournalDirectory != "" && (!filepath.IsAbs(p.JournalDirectory) || filepath.Base(p.JournalDirectory) != "terminal-journal" || p.Directory != filepath.Join(filepath.Dir(p.JournalDirectory), "src") || p.Profile == nil || tc.ValidateProfile(*p.Profile, true) != nil || !tc.SupportsTerminalSessionProfile(*p.Profile)) {
		return fmt.Errorf("terminal original runtime checkpoint is invalid")
	}
	return nil
}

func (j *Journal) checkpointTerminalRuntime(a Assignment, dir, source string, profile tc.Profile) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.Result != nil || e.Terminal != nil {
		return fmt.Errorf("terminal runtime checkpoint already exists or assignment is absent")
	}
	p := terminalCheckpoint{Pending: []term.ExchangeRequest{}, JournalDirectory: dir, Directory: source, Profile: &profile}
	if err := j.validateTerminalCheckpoint(a, p); err != nil {
		return err
	}
	previous := e
	e.Terminal = &p
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = previous
		return err
	}
	return nil
}

// A durable terminal receipt can supply original stop facts after restart.
// Missing ownership remains unknown; this never attaches or kills by stored PID.
func (j *Journal) importTerminalCheckpoint(e *entry) error {
	p := e.Terminal
	if p == nil || p.JournalDirectory == "" || p.Recovered || p.ReconciliationOperationID != "" || e.Result != nil {
		return nil
	}
	copy := *p
	copy.Pending = append([]term.ExchangeRequest{}, p.Pending...)
	copy.Recovered = true
	e.Terminal = &copy
	inspection, err := terminalruntime.InspectJournal(p.JournalDirectory)
	if err != nil {
		return nil
	} // an absent/unreadable runtime cannot prove stop
	id, _ := e.Assignment.Config["terminal_session_id"].(string)
	snap := inspection.Session
	if snap.ID != id || snap.WorkDir != p.Directory {
		return fmt.Errorf("terminal receipt differs from original runtime checkpoint")
	}
	if !snap.TreeStopped {
		return nil
	}
	if snap.State != "stopped" && snap.State != "exited" && snap.State != "reconciling" {
		return fmt.Errorf("terminal durable stop state is invalid")
	}
	// Recover only the immutable source receipt already captured by the
	// original Runner. Its absence or refusal never becomes a new capture and
	// cannot invalidate the independent original process-tree stop proof.
	sourceSHA, baseCommit, sourceErr := recoverTerminalCandidateSnapshot(e.Assignment, p.JournalDirectory, p.Directory)
	if sourceErr != nil {
		sourceSHA, baseCommit = "", ""
	}
	exit := -1
	if snap.ExitCode != nil {
		exit = *snap.ExitCode
	}
	inputs := []term.InputReceipt{}
	for _, r := range inspection.Inputs {
		inputs = append(inputs, term.InputReceipt{InputID: r.InputID, Sequence: int64(r.Sequence), SHA256: r.SHA256, State: r.State, BytesWritten: r.BytesWritten})
	}
	// Inputs are bounded in each exchange; output remains available from the
	// original runtime journal. A source hash is imported only from its existing
	// validated immutable manifest, never synthesized from current files.
	for len(inputs) > 0 || len(copy.Pending) == len(p.Pending) {
		count := min(32, len(inputs))
		a := e.Assignment
		r := term.ExchangeRequest{SessionID: id, RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, DispatchID: a.DispatchID, OperationID: operationID(), State: snap.State, Directory: p.Directory, ProcessID: snap.ProcessID, TreeStopped: true, ExitCode: exit, OutputChunks: []term.OutputChunk{}, InputReceipts: append([]term.InputReceipt{}, inputs[:count]...), Error: "Original terminal stop receipt recovered after Runner restart; execution effect requires review", SourceSnapshotSHA256: sourceSHA, BaseCommit: baseCommit}
		copy.Pending = append(copy.Pending, r)
		inputs = inputs[count:]
		if len(inputs) == 0 {
			break
		}
	}
	e.Terminal = &copy
	e.Result = &Result{Assignment: e.Assignment, ExitCode: exit, Stopped: true, OutcomeUnknown: true, Error: "Original terminal stopped; interrupted workspace execution requires explicit review", OperationID: operationID(), ToolEvidence: &tc.Evidence{Profile: tc.CloneProfile(*p.Profile), SessionID: id, Terminal: "unknown", UsageState: tc.UsageUnreported}}
	if sourceErr != nil {
		e.Result.Error += "; original source snapshot receipt unavailable"
	}
	return nil
}

func flushStoppedTerminal(ctx context.Context, c *Client, j *Journal, a Assignment, session *terminalruntime.Session, source, id, sourceSHA, base string, after uint64) error {
	state := session.Snapshot()
	if sourceSHA == "" {
		base = ""
	}
	var firstErr error
	inputs := []term.InputReceipt{}
	inspection, inspectErr := terminalruntime.InspectJournal(filepath.Join(filepath.Dir(source), "terminal-journal"))
	if inspectErr == nil {
		for _, input := range inspection.Inputs {
			inputs = append(inputs, term.InputReceipt{InputID: input.InputID, Sequence: int64(input.Sequence), SHA256: input.SHA256, State: input.State, BytesWritten: input.BytesWritten})
		}
	}
	for {
		page := session.ReadOutput(after, 128*1024)
		if len(page.Chunks) > 32 {
			page.Chunks = page.Chunks[:32]
		}
		request := term.ExchangeRequest{SessionID: id, RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, DispatchID: a.DispatchID, OperationID: operationID(), State: state.State, Directory: source, ProcessID: state.ProcessID, TreeStopped: state.TreeStopped, ExitCode: -1, Error: state.Reason, OutputChunks: []term.OutputChunk{}, InputReceipts: []term.InputReceipt{}, SourceSnapshotSHA256: sourceSHA, BaseCommit: base}
		if state.ExitCode != nil {
			request.ExitCode = *state.ExitCode
		}
		if page.Gap {
			request.State = "reconciling"
			request.Error = "Original terminal output has a retained-history gap"
			page.Chunks = nil
			after = state.LastOutputSequence
		}
		for _, chunk := range page.Chunks {
			request.OutputChunks = append(request.OutputChunks, term.OutputChunk{Sequence: int64(chunk.Sequence), DataBase64: base64.StdEncoding.EncodeToString(chunk.Data)})
			after = chunk.Sequence
		}
		// Every runtime input has a durable original intent/result. The final
		// receipt list includes late completions of the original single write.
		count := min(32, len(inputs))
		request.InputReceipts = append(request.InputReceipts, inputs[:count]...)
		inputs = inputs[count:]
		var err error
		if j != nil {
			if err = j.checkpointTerminalExchange(a, request); err != nil {
				return err
			}
		}
		if firstErr == nil {
			_, err = terminalExchangeDurable(ctx, c, j, a, request)
			if err != nil {
				firstErr = err
			}
		}
		if (after >= state.LastOutputSequence || len(page.Chunks) == 0) && len(inputs) == 0 {
			break
		}
	}
	return firstErr
}

func ReplayTerminalExchanges(ctx context.Context, c *Client, j *Journal) error {
	processed := map[string]bool{}
	for _, p := range j.pendingTerminalExchanges() {
		processed[key(p.Assignment)] = true
		if j.terminalReconciled(p.Assignment) {
			continue
		}
		client := c
		if p.Assignment.PoolID != "" {
			var err error
			client, err = c.forPoolAssignment(p.Assignment)
			if err != nil {
				return err
			}
		}
		response, err := terminalExchangeOriginal(ctx, client, p.Request)
		if err != nil {
			return err
		}
		if response.RecoveryOperationID != "" {
			if err = j.recordTerminalReconciliation(p.Assignment, response.RecoveryOperationID); err != nil {
				return err
			}
			continue
		}
		if err := j.ackTerminalExchange(p.Assignment, p.Request.OperationID); err != nil {
			return err
		}
	}
	// A stop exchange may already be ACKed while its expired final result
	// remains pending. Re-read that exact operation to learn a later explicit
	// review disposition; this sends no new facts and never retries shell work.
	for _, p := range j.terminalRecoveryProbes() {
		if processed[key(p.Assignment)] {
			continue
		}
		client := c
		if p.Assignment.PoolID != "" {
			var err error
			client, err = c.forPoolAssignment(p.Assignment)
			if err != nil {
				return err
			}
		}
		response, err := terminalExchangeOriginal(ctx, client, p.Request)
		if err != nil {
			return err
		}
		if response.RecoveryOperationID != "" {
			if err = j.recordTerminalReconciliation(p.Assignment, response.RecoveryOperationID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (j *Journal) terminalRecoveryProbes() []pendingTerminalExchange {
	j.mu.Lock()
	defer j.mu.Unlock()
	keys := []string{}
	for k, e := range j.entries {
		if !e.Acked && e.Result != nil && e.Terminal != nil && e.Terminal.ReconciliationOperationID == "" && len(e.Terminal.Pending) == 0 && e.Terminal.LastExchange != nil {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := []pendingTerminalExchange{}
	for _, k := range keys {
		e := j.entries[k]
		out = append(out, pendingTerminalExchange{cloneModelAssignment(e.Assignment), cloneTerminalExchange(*e.Terminal.LastExchange)})
	}
	return out
}

func terminalExchangeDurable(ctx context.Context, c *Client, j *Journal, a Assignment, r term.ExchangeRequest) (term.ExchangeResponse, error) {
	if j == nil {
		return terminalExchangeOriginal(ctx, c, r)
	}
	if err := j.checkpointTerminalExchange(a, r); err != nil {
		return term.ExchangeResponse{}, err
	}
	var out term.ExchangeResponse
	for _, p := range j.pendingTerminalExchanges() {
		if key(p.Assignment) != key(a) {
			continue
		}
		if j.terminalReconciled(a) {
			continue
		}
		response, err := terminalExchangeOriginal(ctx, c, p.Request)
		if err != nil {
			return out, err
		}
		if response.RecoveryOperationID != "" {
			if err = j.recordTerminalReconciliation(a, response.RecoveryOperationID); err != nil {
				return out, err
			}
			return response, nil
		}
		if err = j.ackTerminalExchange(a, p.Request.OperationID); err != nil {
			return out, err
		}
		if p.Request.OperationID == r.OperationID {
			out = response
		}
	}
	return out, nil
}

func (j *Journal) terminalReconciled(a Assignment) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.entries[key(a)]
	return ok && e.Terminal != nil && e.Terminal.ReconciliationOperationID != ""
}
func (j *Journal) recordTerminalReconciliation(a Assignment, id string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.Terminal == nil || id == "" || len(id) > 128 {
		return fmt.Errorf("terminal reconciliation receipt is invalid")
	}
	if e.Terminal.ReconciliationOperationID != "" {
		if e.Terminal.ReconciliationOperationID != id {
			return fmt.Errorf("terminal reconciliation receipt changed")
		}
		return nil
	}
	previous := e
	p := *e.Terminal
	p.Unapplied = append([]term.ExchangeRequest{}, p.Pending...)
	p.Pending = []term.ExchangeRequest{}
	p.ReconciliationOperationID = id
	e.Terminal = &p
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = previous
		return err
	}
	return nil
}
