package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	rc "github.com/ai-mada/suma/internal/desktoprecoverycontract"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type desktopRecoveryState struct {
	root        *os.Root
	lock        *os.File
	directory   string
	identity    os.FileInfo
	entries     map[string]json.RawMessage
	document    map[string]any
	unsupported bool
	originalSHA string
}

func (s *desktopRecoveryState) close() {
	if s.lock != nil {
		_ = s.lock.Close()
	}
	if s.root != nil {
		_ = s.root.Close()
	}
}
func recoveryResult(plan *rc.Plan, reasons ...string) rc.Result {
	result := rc.Result{Protocol: rc.ResultProtocol, Plan: plan, Reasons: []string{}}
	if plan != nil {
		result.Total = len(plan.Items)
		result.Remaining = result.Total
	}
	for _, reason := range reasons {
		seen := false
		for _, old := range result.Reasons {
			seen = seen || old == reason
		}
		if !seen {
			result.Reasons = append(result.Reasons, reason)
		}
	}
	return result
}
func (s *desktopRecoveryState) unchangedDirectory() bool {
	info, err := os.Lstat(s.directory)
	return err == nil && os.SameFile(s.identity, info)
}
func openDesktopRecovery(stateDir, project, worker, temp string) (*desktopRecoveryState, string) {
	empty := rc.Plan{Protocol: rc.PlanProtocol, ProjectID: project, WorkerID: worker, SnapshotSHA256: strings.Repeat("0", 64), Items: []rc.Item{}}
	if rc.ValidatePlan(empty) != nil || !desktopManagedPath(stateDir) || filepath.Dir(filepath.Clean(stateDir)) == filepath.Clean(stateDir) || runtime.GOOS == "windows" && (!cursorDiscoveryWindowsDrive(stateDir) || cursorDiscoveryWindowsNamespace(stateDir)) {
		return nil, "invalid_state"
	}
	stateDir = filepath.Clean(stateDir)
	for name := stateDir; ; name = filepath.Dir(name) {
		info, err := os.Lstat(name)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, "no_saved_results"
			}
			return nil, "invalid_state"
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, "invalid_state"
		}
		if filepath.Dir(name) == name {
			break
		}
	}
	root, err := os.OpenRoot(stateDir)
	if err != nil {
		return nil, "invalid_state"
	}
	s := &desktopRecoveryState{root: root, directory: stateDir}
	fail := func(reason string) (*desktopRecoveryState, string) { s.close(); return nil, reason }
	s.identity, err = root.Stat(".")
	if err != nil || !s.unchangedDirectory() {
		return fail("state_changed")
	}
	lockInfo, err := root.Lstat("journal.lock")
	if err != nil {
		if desktopRenewalLockBusy(err) {
			return fail("active_worker")
		}
		return fail("invalid_state")
	}
	if !lockInfo.Mode().IsRegular() || lockInfo.Mode()&os.ModeSymlink != 0 {
		return fail("invalid_state")
	}
	s.lock, err = lockExistingJournalForRecovery(filepath.Join(stateDir, "journal.lock"))
	if err != nil {
		if desktopRenewalLockBusy(err) {
			return fail("active_worker")
		}
		return fail("invalid_state")
	}
	owned, err := s.lock.Stat()
	if err != nil || !os.SameFile(lockInfo, owned) {
		return fail("state_changed")
	}
	raw, err := readDesktopRenewalFile(root, "journal.json", 16<<20)
	if err != nil {
		return fail("invalid_state")
	}
	if desktopRenewalDecode(raw, &s.entries) != nil || s.entries == nil || len(s.entries) > 10000 {
		return fail("invalid_state")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&s.document) != nil || s.document == nil {
		return fail("invalid_state")
	}
	canonical, err := json.Marshal(s.document)
	if err != nil {
		return fail("invalid_state")
	}
	s.originalSHA = rc.Digest(canonical)
	for id, rawEntry := range s.entries {
		var e entry
		if desktopRenewalDecode(rawEntry, &e) != nil || id != key(e.Assignment) || e.Assignment.Attempt < 1 || e.Assignment.Fence < 1 {
			return fail("invalid_state")
		}
		if e.Assignment.WorkerID != worker || e.Assignment.ProjectID != "" && e.Assignment.ProjectID != project || e.Assignment.PoolID != "" {
			return fail("source_changed")
		}
		if e.Result == nil || e.Result.OutcomeUnknown || e.ArtifactPending != nil || e.CandidatePending != nil || e.Result.WorkBuddyCloudWait != nil {
			s.unsupported = true
		}
		for _, p := range e.Progress {
			if !validProgressDisposition(p.Acked, p.Rejection, p.HostOperation != nil) {
				return fail("invalid_state")
			}
			if p.pending() {
				s.unsupported = true
			}
		}
		if e.Terminal != nil && (len(e.Terminal.Pending) > 0 || len(e.Terminal.Unapplied) > 0) {
			s.unsupported = true
		}
		if e.ToolPermissions != nil {
			for _, request := range e.ToolPermissions.Requests {
				if request.Decision == nil {
					s.unsupported = true
				}
				for _, observation := range request.Observations {
					if !observation.Acked || observation.ReconciliationRequired {
						s.unsupported = true
					}
				}
			}
		}
	}
	directory, err := root.Open(".")
	if err != nil {
		return fail("invalid_state")
	}
	files, err := directory.ReadDir(65)
	_ = directory.Close()
	if err != nil && err != io.EOF {
		return fail("invalid_state")
	}
	for _, file := range files {
		name := file.Name()
		if name == "journal.json" || name == "journal.lock" {
			continue
		}
		if name == temp {
			continue
		} // The exact approved replacement is verified before rename.
		if file.IsDir() || file.Type()&os.ModeSymlink != 0 {
			s.unsupported = true
			continue
		}
		limit := int64(2 << 20)
		if name == "workspace-observations.json" {
			limit = 32 << 20
		}
		if name != "claim.json" && name != "tool-inventory.json" && name != "workspace-observations.json" && name != "workspace-writes.json" {
			s.unsupported = true
			continue
		}
		data, err := readDesktopRenewalFile(root, name, limit)
		if err != nil {
			s.unsupported = true
			continue
		}
		switch name {
		case "claim.json":
			var value ClaimCheckpoint
			if desktopRenewalDecode(data, &value) != nil || !value.Complete || !value.Responded {
				s.unsupported = true
			}
		case "tool-inventory.json":
			var value InventoryReport
			if desktopRenewalDecode(data, &value) != nil || !desktopManagedHex(value.ScopeSHA256, 64) || !value.Acked {
				s.unsupported = true
			}
		case "workspace-observations.json":
			var value workspaceObservationJournal
			if desktopRenewalDecode(data, &value) != nil || value.Entries == nil {
				s.unsupported = true
			}
			for _, e := range value.Entries {
				if !e.Acked || e.Result == nil || !validWorkspaceObservationCheckpoint(e) {
					s.unsupported = true
				}
			}
		case "workspace-writes.json":
			var value workspaceWriteJournal
			if desktopRenewalDecode(data, &value) != nil || value.Entries == nil {
				s.unsupported = true
			}
			if value.Claim != nil && !value.Claim.Complete {
				s.unsupported = true
			}
			for _, e := range value.Entries {
				if !e.Acked || e.Result == nil || !validWorkspaceWriteCheckpoint(e) {
					s.unsupported = true
				}
			}
		}
	}
	if !s.unchangedDirectory() {
		return fail("state_changed")
	}
	return s, ""
}

// The same pure envelope is used by Complete and the offline planner. This
// helper grants no transport access and preserves the existing wire format.
func resultCallbackPayload(project string, r Result) map[string]any {
	return map[string]any{"project_id": project, "result": r}
}
func (s *desktopRecoveryState) item(project, worker, id string) (rc.Item, bool, error) {
	rawEntry, ok := s.entries[id]
	if !ok {
		return rc.Item{}, false, os.ErrNotExist
	}
	var fields map[string]json.RawMessage
	if desktopRenewalDecode(rawEntry, &fields) != nil {
		return rc.Item{}, false, os.ErrInvalid
	}
	var a Assignment
	if json.Unmarshal(fields["assignment"], &a) != nil {
		return rc.Item{}, false, os.ErrInvalid
	}
	var r Result
	decoder := json.NewDecoder(bytes.NewReader(fields["result"]))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if len(fields["result"]) == 0 || decoder.Decode(&r) != nil || r.RunID != a.RunID || r.NodeID != a.NodeID || r.Attempt != a.Attempt || r.Fence != a.Fence || r.WorkerID != worker || r.OperationID == "" {
		return rc.Item{}, false, os.ErrInvalid
	}
	var acked bool
	if json.Unmarshal(fields["acked"], &acked) != nil {
		return rc.Item{}, false, os.ErrInvalid
	}
	body, err := json.Marshal(resultCallbackPayload(project, r))
	if err != nil {
		return rc.Item{}, false, err
	}
	serverSHA, err := rc.ServerResultSHA(fields["result"])
	if err != nil {
		return rc.Item{}, false, err
	}
	return rc.Item{JournalKey: id, RunID: r.RunID, NodeID: r.NodeID, Attempt: r.Attempt, Fence: strconv.FormatInt(r.Fence, 10), OperationID: r.OperationID, BodySHA256: rc.Digest(body), ResultSHA256: serverSHA}, acked, nil
}
func (s *desktopRecoveryState) normalizedSHA(items []rc.Item) (string, error) {
	raw, err := json.Marshal(s.document)
	if err != nil {
		return "", err
	}
	var copy map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&copy) != nil {
		return "", os.ErrInvalid
	}
	for _, item := range items {
		entry, ok := copy[item.JournalKey].(map[string]any)
		if !ok {
			return "", os.ErrInvalid
		}
		entry["acked"] = false
	}
	raw, err = json.Marshal(copy)
	if err != nil {
		return "", err
	}
	return rc.Digest(raw), nil
}
func PrepareDesktopResultRecovery(stateDir, project, worker string) rc.Result {
	s, reason := openDesktopRecovery(stateDir, project, worker, "")
	if reason != "" {
		return recoveryResult(nil, reason)
	}
	defer s.close()
	plan := rc.Plan{Protocol: rc.PlanProtocol, ProjectID: project, WorkerID: worker, Items: []rc.Item{}}
	keys := make([]string, 0, len(s.entries))
	for key := range s.entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, id := range keys {
		fields := s.document[id].(map[string]any)
		if fields["result"] == nil {
			continue
		}
		if acked, ok := fields["acked"].(bool); !ok {
			return recoveryResult(nil, "invalid_state")
		} else if acked {
			continue
		}
		item, _, err := s.item(project, worker, id)
		if err != nil {
			return recoveryResult(nil, "invalid_state")
		}
		if len(plan.Items) == rc.MaxItems {
			s.unsupported = true
			break
		}
		plan.Items = append(plan.Items, item)
	}
	sha, err := s.normalizedSHA(plan.Items)
	if err != nil {
		return recoveryResult(nil, "invalid_state")
	}
	plan.SnapshotSHA256 = sha
	if rc.ValidatePlan(plan) != nil {
		return recoveryResult(nil, "invalid_state")
	}
	result := recoveryResult(&plan)
	if s.unsupported {
		result.Reasons = append(result.Reasons, "unsupported_pending")
	}
	if len(plan.Items) == 0 {
		result.Reasons = append(result.Reasons, "no_saved_results")
	}
	return result
}
func ApplyDesktopResultRecovery(stateDir, project, worker string, plan rc.Plan, proof rc.Proof, key []byte, now time.Time) rc.Result {
	return desktopResultRecovery(stateDir, project, worker, plan, proof, key, now, true)
}
func InspectDesktopResultRecovery(stateDir, project, worker string, plan rc.Plan, proof rc.Proof, key []byte, now time.Time) rc.Result {
	return desktopResultRecovery(stateDir, project, worker, plan, proof, key, now, false)
}
func desktopResultRecovery(stateDir, project, worker string, plan rc.Plan, proof rc.Proof, key []byte, now time.Time, apply bool) rc.Result {
	if rc.ValidatePlan(plan) != nil {
		return recoveryResult(nil, "invalid_state")
	}
	if plan.ProjectID != project || plan.WorkerID != worker {
		return recoveryResult(&plan, "source_changed")
	}
	withProof := proof.Protocol != "" || len(key) != 0
	if withProof || apply {
		if rc.CheckProof(proof, plan, key, now, true) != nil {
			return recoveryResult(&plan, "access_changed")
		}
		if apply && rc.CheckProof(proof, plan, key, now, false) != nil {
			return recoveryResult(&plan, "proof_expired")
		}
	}
	temp := "journal-recovery-" + rc.PlanSHA256(plan) + ".tmp"
	s, reason := openDesktopRecovery(stateDir, project, worker, temp)
	if reason != "" {
		return recoveryResult(&plan, reason)
	}
	defer s.close()
	sha, err := s.normalizedSHA(plan.Items)
	if err != nil || sha != plan.SnapshotSHA256 {
		return recoveryResult(&plan, "state_changed")
	}
	result := recoveryResult(&plan)
	changed := false
	for i, item := range plan.Items {
		current, acked, err := s.item(project, worker, item.JournalKey)
		if err != nil || current != item {
			return recoveryResult(&plan, "state_changed")
		}
		if !withProof {
			if acked {
				return recoveryResult(&plan, "state_changed")
			}
			continue
		}
		status := proof.Items[i].Status
		if acked && status != "matched" {
			return recoveryResult(&plan, "state_changed")
		}
		if status == "matched" {
			if acked {
				result.Recovered++
			} else if apply {
				s.document[item.JournalKey].(map[string]any)["acked"] = true
				changed = true
			}
		} else {
			code := "receipt_mismatch"
			if status == "missing" {
				code = "receipt_missing"
			} else if status == "source_changed" {
				code = "source_changed"
			}
			addDesktopRecoveryReason(&result, code)
		}
	}
	if changed {
		data, err := json.Marshal(s.document)
		if err != nil {
			return recoveryResult(&plan, "invalid_state")
		}
		if err = s.commit(plan, temp, data); err != nil {
			return recoveryResult(&plan, "result_unknown")
		}
		for i, receipt := range proof.Items {
			if receipt.Status == "matched" {
				_, oldAck, _ := s.item(project, worker, plan.Items[i].JournalKey)
				if !oldAck {
					result.Recovered++
				}
			}
		}
	}
	result.Remaining = result.Total - result.Recovered
	if !withProof {
		addDesktopRecoveryReason(&result, "result_unknown")
	}
	if s.unsupported {
		addDesktopRecoveryReason(&result, "unsupported_pending")
	}
	if !changed {
		if _, err := s.root.Lstat(temp); err == nil {
			addDesktopRecoveryReason(&result, "unsupported_pending")
		}
	}
	return result
}
func addDesktopRecoveryReason(result *rc.Result, reason string) {
	for _, old := range result.Reasons {
		if old == reason {
			return
		}
	}
	result.Reasons = append(result.Reasons, reason)
}

// One deterministic, plan-bound replacement supports a crash after fsync but
// before rename. Only exactly matching approved bytes may be resumed; an
// unknown file is retained and blocks the operation rather than being deleted.
func (s *desktopRecoveryState) commit(plan rc.Plan, temp string, data []byte) error {
	file, err := s.root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		_, err = file.Write(data)
		if err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
	} else if !os.IsExist(err) {
		return err
	}
	saved, err := readDesktopRenewalFile(s.root, temp, 16<<20)
	if err != nil {
		return err
	}
	if len(saved) < len(data) && bytes.HasPrefix(data, saved) {
		// A crash can leave an exact prefix, including the zero-byte create.
		// Complete only that approved prefix; never truncate or overwrite an
		// unrelated file, even if it happens to use the deterministic name.
		appendFile, err := s.root.OpenFile(temp, os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		_, err = appendFile.Write(data[len(saved):])
		if err == nil {
			err = appendFile.Sync()
		}
		closeErr := appendFile.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		saved, err = readDesktopRenewalFile(s.root, temp, 16<<20)
		if err != nil {
			return err
		}
	}
	if !bytes.Equal(saved, data) {
		return fmt.Errorf("recovery replacement differs from approved bytes")
	}
	current, err := readDesktopRenewalFile(s.root, "journal.json", 16<<20)
	if err != nil {
		return err
	}
	var currentDocument map[string]any
	decoder := json.NewDecoder(bytes.NewReader(current))
	decoder.UseNumber()
	if decoder.Decode(&currentDocument) != nil {
		return os.ErrInvalid
	}
	currentCanonical, err := json.Marshal(currentDocument)
	if err != nil || rc.Digest(currentCanonical) != s.originalSHA || !s.unchangedDirectory() {
		return os.ErrInvalid
	}
	return s.root.Rename(temp, "journal.json")
}
