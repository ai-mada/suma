package runner

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ai-mada/suma/internal/strictjson"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
	wwc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

const DesktopRunnerRenewalCheckProtocol = "suma-desktop-runner-renewal-check-v1"

// This private local observation contains no journal contents or process paths.
// It is not a server execution disposition or permission to restart work.
type DesktopRunnerRenewalCheck struct {
	Protocol string   `json:"protocol"`
	Safe     bool     `json:"safe"`
	Reasons  []string `json:"reasons"`
}

func InspectDesktopRunnerRenewalState(stateDirectory string) DesktopRunnerRenewalCheck {
	check := DesktopRunnerRenewalCheck{Protocol: DesktopRunnerRenewalCheckProtocol, Reasons: []string{}}
	reasons := map[string]bool{}
	add := func(reason string) { reasons[reason] = true }
	finish := func() DesktopRunnerRenewalCheck {
		for _, reason := range []string{"active_worker", "pending_execution", "pending_receipt", "unknown_outcome", "invalid_state", "unreadable_state"} {
			if reasons[reason] {
				check.Reasons = append(check.Reasons, reason)
			}
		}
		check.Safe = len(check.Reasons) == 0
		return check
	}
	if !desktopManagedPath(stateDirectory) || filepath.Dir(filepath.Clean(stateDirectory)) == filepath.Clean(stateDirectory) || runtime.GOOS == "windows" && (!cursorDiscoveryWindowsDrive(stateDirectory) || cursorDiscoveryWindowsNamespace(stateDirectory)) {
		add("unreadable_state")
		return finish()
	}
	stateDirectory = filepath.Clean(stateDirectory)
	for path := stateDirectory; ; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err != nil && !os.IsNotExist(err) || err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			add("unreadable_state")
			return finish()
		}
		if filepath.Dir(path) == path {
			break
		}
	}
	stateInfo, err := os.Lstat(stateDirectory)
	if os.IsNotExist(err) {
		return finish()
	}
	root, err := os.OpenRoot(stateDirectory)
	if err != nil {
		add("unreadable_state")
		return finish()
	}
	defer root.Close()
	anchored, err := root.Stat(".")
	if err != nil || stateInfo == nil || !os.SameFile(stateInfo, anchored) {
		add("unreadable_state")
		return finish()
	}
	sameState := func() bool {
		current, err := os.Lstat(stateDirectory)
		return err == nil && os.SameFile(anchored, current)
	}
	dir, err := root.Open(".")
	if err != nil {
		add("unreadable_state")
		return finish()
	}
	files, err := dir.ReadDir(17)
	_ = dir.Close()
	if err != nil && err != io.EOF {
		add("unreadable_state")
		return finish()
	}
	if len(files) == 0 {
		if !sameState() {
			add("unreadable_state")
		}
		return finish()
	}
	// Never create journal.lock. Its absence beside existing state cannot prove
	// that a failed start or an exchanged state directory has fully drained.
	before, err := root.Lstat("journal.lock")
	if err != nil {
		if os.IsNotExist(err) {
			add("invalid_state")
		} else if desktopRenewalLockBusy(err) {
			add("active_worker")
		} else {
			add("unreadable_state")
		}
		return finish()
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		add("invalid_state")
		return finish()
	}
	lock, err := lockExistingJournal(filepath.Join(stateDirectory, "journal.lock"))
	if err != nil {
		if desktopRenewalLockBusy(err) {
			add("active_worker")
		} else {
			add("unreadable_state")
		}
		return finish()
	}
	defer lock.Close()
	locked, err := lock.Stat()
	if err != nil || !os.SameFile(before, locked) {
		add("unreadable_state")
		return finish()
	}
	// Holding the original execution lock prevents a Worker from concurrently
	// reopening any of these checkpoints. os.Root anchors all subsequent reads.
	dir, err = root.Open(".")
	if err != nil {
		add("unreadable_state")
		return finish()
	}
	files, err = dir.ReadDir(17)
	_ = dir.Close()
	if err != nil && err != io.EOF {
		add("unreadable_state")
		return finish()
	}
	limits := map[string]int64{"journal.lock": 4096, "journal.json": 16 << 20, "claim.json": 2 << 20, "tool-inventory.json": 1 << 20, "workspace-observations.json": 32 << 20, "workspace-writes.json": 2 << 20}
	rawFiles := map[string][]byte{}
	for _, file := range files {
		limit, known := limits[file.Name()]
		if !known || file.IsDir() || file.Type()&os.ModeSymlink != 0 {
			add("invalid_state")
			continue
		}
		if file.Name() == "journal.lock" {
			continue
		}
		raw, err := readDesktopRenewalFile(root, file.Name(), limit)
		if err != nil {
			add("unreadable_state")
			continue
		}
		rawFiles[file.Name()] = raw
	}
	if len(rawFiles) > 0 && rawFiles["journal.json"] == nil {
		add("invalid_state")
	}
	if raw := rawFiles["journal.json"]; raw != nil {
		inspectDesktopRenewalJournal(raw, add)
	}
	if raw := rawFiles["claim.json"]; raw != nil {
		var claim ClaimCheckpoint
		if desktopRenewalDecode(raw, &claim) != nil || claim.SchemaVersion != 1 || len(claim.OperationID) != 32 || !tc.ValidDigest(claim.ControlSHA256) || claim.ProjectID == "" || claim.WorkerID == "" || len(claim.Capabilities) > 512 || claim.Complete && !claim.Responded || claim.Assignment != nil && validateDispatch(*claim.Assignment, claim.WorkerID) != nil {
			add("invalid_state")
		} else if !claim.Complete {
			add("pending_receipt")
		}
	}
	if raw := rawFiles["tool-inventory.json"]; raw != nil {
		var inventory InventoryReport
		if desktopRenewalDecode(raw, &inventory) != nil || inventory.WorkerID == "" || len(inventory.OperationID) != 32 || !tc.ValidDigest(inventory.ScopeSHA256) || inventory.Items == nil || len(inventory.Items) > 128 {
			add("invalid_state")
		} else {
			for _, item := range inventory.Items {
				if tc.ValidateObservation(item) != nil {
					add("invalid_state")
				}
			}
			if !inventory.Acked {
				add("pending_receipt")
			} else if _, err := time.Parse(time.RFC3339, inventory.ObservedAt); err != nil {
				add("invalid_state")
			}
		}
	}
	if raw := rawFiles["workspace-observations.json"]; raw != nil {
		var state workspaceObservationJournal
		if desktopRenewalDecode(raw, &state) != nil || state.Protocol != woc.Protocol || state.Entries == nil || len(state.Entries) > 256 {
			add("invalid_state")
		} else {
			for key, entry := range state.Entries {
				if key != workspaceObservationKey(entry.ControlSHA256, entry.Lease) || !validWorkspaceObservationCheckpoint(entry) {
					add("invalid_state")
					continue
				}
				if !entry.Acked {
					add("pending_receipt")
				}
				if entry.Acked && entry.Result == nil {
					add("invalid_state")
				}
			}
		}
	}
	if raw := rawFiles["workspace-writes.json"]; raw != nil {
		var state workspaceWriteJournal
		if desktopRenewalDecode(raw, &state) != nil || state.Protocol != wwc.Protocol || state.Entries == nil || len(state.Entries) > 256 {
			add("invalid_state")
		} else {
			if state.Claim != nil {
				if !validWorkspaceWriteClaim(*state.Claim) {
					add("invalid_state")
				}
				if !state.Claim.Complete {
					add("pending_receipt")
				}
			}
			for key, entry := range state.Entries {
				if key != workspaceWriteKey(entry.ControlSHA256, entry.Lease) || !validWorkspaceWriteCheckpoint(entry) {
					add("invalid_state")
					continue
				}
				if !entry.Acked {
					add("pending_receipt")
				}
				if entry.Acked && entry.Result == nil {
					add("invalid_state")
				}
				if entry.ProcessingStarted && entry.Result == nil {
					add("unknown_outcome")
				}
			}
		}
	}
	after, err := root.Lstat("journal.lock")
	if err != nil || !os.SameFile(locked, after) {
		add("unreadable_state")
	}
	if !sameState() {
		add("unreadable_state")
	}
	return finish()
}

func readDesktopRenewalFile(root *os.Root, name string, limit int64) ([]byte, error) {
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 || before.Size() > limit {
		return nil, os.ErrInvalid
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, os.ErrInvalid
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	after, statErr := file.Stat()
	current, currentErr := root.Lstat(name)
	if err != nil || statErr != nil || currentErr != nil || int64(len(raw)) != before.Size() || int64(len(raw)) > limit || !after.ModTime().Equal(before.ModTime()) || !os.SameFile(opened, after) || !os.SameFile(opened, current) {
		return nil, os.ErrInvalid
	}
	return raw, nil
}

func desktopRenewalDecode(raw []byte, out any) error {
	return strictjson.Decode(raw, out, len(raw))
}

func inspectDesktopRenewalJournal(raw []byte, add func(string)) {
	var entries map[string]json.RawMessage
	if desktopRenewalDecode(raw, &entries) != nil || entries == nil || len(entries) > 10000 {
		add("invalid_state")
		return
	}
	for id, rawEntry := range entries {
		var e entry
		if desktopRenewalDecode(rawEntry, &e) != nil {
			add("invalid_state")
			continue
		}
		a := e.Assignment
		if id != key(a) || a.RunID == "" || a.NodeID == "" || a.WorkerID == "" || a.Attempt < 1 || a.Fence < 1 || e.Acked && e.Result == nil {
			add("invalid_state")
			continue
		}
		if e.Result == nil {
			add("pending_execution")
		} else {
			r := e.Result
			if r.RunID != a.RunID || r.NodeID != a.NodeID || r.Attempt != a.Attempt || r.Fence != a.Fence || r.WorkerID != a.WorkerID || r.OperationID == "" {
				add("invalid_state")
			}
			if r.OutcomeUnknown {
				add("unknown_outcome")
			}
			if !e.Acked {
				add("pending_receipt")
			}
			if r.WorkBuddyCloudWait != nil {
				add("pending_execution")
			}
		}
		if e.ArtifactPending != nil || e.CandidatePending != nil {
			add("pending_receipt")
		}
		for _, progress := range e.Progress {
			if !validProgressDisposition(progress.Acked, progress.Rejection, progress.HostOperation != nil) {
				add("invalid_state")
			} else if progress.pending() {
				add("pending_receipt")
			}
		}
		if e.Cloud != nil && validCloudExecution(a, *e.Cloud) != nil {
			add("invalid_state")
		}
		if e.ModelGateway != nil {
			if validModelGatewayCheckpoint(a, id, *e.ModelGateway) != nil {
				add("invalid_state")
			}
			if !e.ModelGateway.Snapshot.Closed {
				add("unknown_outcome")
			}
			for _, record := range e.ModelGateway.Snapshot.Records {
				if record.State == "unknown" || record.State == "started" {
					add("unknown_outcome")
				}
			}
		}
		if e.WorkspaceTrace != nil {
			if validateWorkspaceTraceCheckpoint(a, *e.WorkspaceTrace) != nil {
				add("invalid_state")
			}
			if e.WorkspaceTrace.Truncated {
				add("unknown_outcome")
			}
		}
		if e.Terminal != nil {
			if (&Journal{}).validateTerminalCheckpoint(a, *e.Terminal) != nil {
				add("invalid_state")
			}
			if len(e.Terminal.Pending) > 0 {
				add("pending_receipt")
			}
			if len(e.Terminal.Unapplied) > 0 {
				add("unknown_outcome")
			}
		}
		if validToolPermissionCheckpoint(a, e.ToolPermissions) != nil {
			add("invalid_state")
		}
		if e.ToolPermissions != nil {
			for _, request := range e.ToolPermissions.Requests {
				if request.Request.Codex == nil {
					if request.Decision == nil {
						add("pending_execution")
					}
					continue
				}
				for _, event := range request.Observations {
					if !event.Acked {
						add("pending_receipt")
					}
					if event.ReconciliationRequired {
						add("unknown_outcome")
					}
				}
				o := request.Observation
				if o == nil || !o.RequestResolved || o.ItemStatus == "" || o.ItemStatus == "inProgress" {
					add("pending_execution")
				}
				if o != nil && (o.FailureCode != "" || o.ReplyStarted && !o.ReplyWritten) {
					add("unknown_outcome")
				}
			}
		}
	}
}
