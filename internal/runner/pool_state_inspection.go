package runner

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"unicode/utf8"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const (
	poolInspectionMaxSlots     = 128
	poolInspectionMaxBytes     = 64 << 20
	poolInspectionClaimBytes   = 2 << 20
	poolInspectionIdentitySize = 4 << 10
)

// PoolStateInspection projects only machine-local slot facts. Complete describes
// scan coverage; even complete scans combine separate, non-atomic file reads.
type PoolStateInspection struct {
	SchemaVersion int                       `json:"schema_version"`
	Component     string                    `json:"component"`
	Status        string                    `json:"status"`
	Complete      bool                      `json:"complete"`
	Slots         []PoolStateInspectionSlot `json:"slots"`
}

type PoolStateInspectionSlot struct {
	SlotID        string                   `json:"slot_id"`
	JournalStatus string                   `json:"journal_status"`
	JournalCounts *JournalInspectionCounts `json:"journal_counts,omitempty"`
	ClaimState    string                   `json:"claim_state"`
}

type poolInspectionIdentity struct {
	controlSHA, poolID, runnerID, configSHA string
	credentialVersion                       int
}

// InspectPoolState reads the exact on-disk pool directory. No runtime locks,
// callbacks, remote checks, or state initialization take place.
func InspectPoolState(poolDirectory string) PoolStateInspection {
	out := PoolStateInspection{SchemaVersion: 1, Component: "local_pool_runner_state", Slots: []PoolStateInspectionSlot{}}
	root, status := openPoolInspectionRoot(poolDirectory)
	out.Status = status
	if root == nil {
		return out
	}
	defer root.Close()

	remaining := int64(poolInspectionMaxBytes)
	identityRaw, status := readPoolInspectionFile(root, "identity.json", poolInspectionIdentitySize, &remaining)
	out.Status = status
	if status != "available" {
		return out
	}
	identity, valid := decodePoolInspectionIdentity(identityRaw)
	if !valid {
		out.Status = "invalid"
		return out
	}

	slots, status := openPoolInspectionChild(root, "slots")
	if status == "missing" {
		// Maintenance-only operation can legitimately have no slot directory.
		out.Status, out.Complete = "available", true
		return out
	}
	out.Status = status
	if slots == nil {
		return out
	}
	defer slots.Close()

	names, complete, status := poolInspectionSlotNames(slots)
	out.Status, out.Complete = status, complete
	for _, name := range names {
		slot := inspectPoolSlot(slots, name, identity, &remaining)
		out.Slots = append(out.Slots, slot)
		if slot.JournalStatus != "available" || !poolInspectionClaimObserved(slot.ClaimState) {
			out.Complete = false
		}
	}
	return out
}

func openPoolInspectionRoot(dir string) (*os.Root, string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, "unreadable"
	}
	info, err := os.Lstat(abs)
	if os.IsNotExist(err) {
		return nil, "missing"
	}
	if err != nil || !poolInspectionDirectory(info) {
		return nil, "unreadable"
	}
	for ancestor := filepath.Dir(abs); ; ancestor = filepath.Dir(ancestor) {
		parent, err := os.Lstat(ancestor)
		if err != nil || !poolInspectionDirectory(parent) {
			return nil, "unreadable"
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
	root, err := os.OpenRoot(abs)
	return bindPoolInspectionRoot(root, info, err)
}

func poolInspectionDirectory(info os.FileInfo) bool {
	return info.IsDir() && info.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0
}

func openPoolInspectionChild(root *os.Root, name string) (*os.Root, string) {
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return nil, "missing"
	}
	if err != nil || !poolInspectionDirectory(info) {
		return nil, "unreadable"
	}
	child, err := root.OpenRoot(name)
	return bindPoolInspectionRoot(child, info, err)
}

func bindPoolInspectionRoot(root *os.Root, expected os.FileInfo, openErr error) (*os.Root, string) {
	if openErr != nil {
		return nil, "unreadable"
	}
	anchored, err := root.Stat(".")
	if err != nil || !os.SameFile(expected, anchored) {
		root.Close()
		return nil, "unreadable"
	}
	return root, "available"
}

func poolInspectionSlotNames(root *os.Root) ([]string, bool, string) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, false, "unreadable"
	}
	defer dir.Close()
	entries, err := dir.ReadDir(poolInspectionMaxSlots + 1)
	if err != nil && err != io.EOF {
		return nil, false, "unreadable"
	}
	if len(entries) > poolInspectionMaxSlots {
		return nil, false, "over_limit"
	}
	var ids []int
	complete := true
	for _, entry := range entries {
		name := entry.Name()
		id, err := strconv.Atoi(name)
		if err != nil || id < 0 || id >= poolInspectionMaxSlots || strconv.Itoa(id) != name {
			complete = false
			continue
		}
		ids = append(ids, id)
	}
	sort.Ints(ids)
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, strconv.Itoa(id))
	}
	return names, complete, "available"
}

func inspectPoolSlot(root *os.Root, name string, identity poolInspectionIdentity, remaining *int64) PoolStateInspectionSlot {
	slot := PoolStateInspectionSlot{SlotID: name}
	dir, status := openPoolInspectionChild(root, name)
	if dir == nil {
		slot.JournalStatus, slot.ClaimState = status, status
		return slot
	}
	defer dir.Close()

	journalRaw, journalStatus := readPoolInspectionFile(dir, "journal.json", journalInspectionMaxBytes, remaining)
	slot.JournalStatus = journalStatus
	if journalStatus == "available" {
		// The single-slot inspector owns journal count semantics; never include
		// its detailed identifiers in the machine-wide projection.
		journal := inspectJournalBytes(journalRaw)
		slot.JournalStatus = journal.Status
		if journal.Status == "available" {
			slot.JournalCounts = &journal.Counts
		}
	}

	claimRaw, claimStatus := readPoolInspectionFile(dir, "pool-claim.json", poolInspectionClaimBytes, remaining)
	slot.ClaimState = claimStatus
	if claimStatus == "available" {
		slot.ClaimState = inspectPoolClaimBytes(claimRaw, name, identity)
	}
	return slot
}

func readPoolInspectionFile(root *os.Root, name string, limit int64, remaining *int64) ([]byte, string) {
	before, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return nil, "missing"
	}
	if err != nil || !before.Mode().IsRegular() {
		return nil, "unreadable"
	}
	size := before.Size()
	if size < 0 || size > limit || size > *remaining {
		return nil, "over_limit"
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, "unreadable"
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !samePoolInspectionFile(before, opened) {
		return nil, "unreadable"
	}
	// Charge every attempted read to the aggregate budget, even if the file
	// changes before EOF. Reading exactly the measured size bounds growth races.
	*remaining -= size
	raw := make([]byte, size)
	if _, err = io.ReadFull(file, raw); err != nil {
		return nil, "unreadable"
	}
	after, err := file.Stat()
	if err != nil || !samePoolInspectionFile(opened, after) {
		return nil, "unreadable"
	}
	return raw, "available"
}

func samePoolInspectionFile(before, after os.FileInfo) bool {
	return after.Mode().IsRegular() && os.SameFile(before, after) &&
		after.Size() == before.Size() && after.ModTime().Equal(before.ModTime())
}

func decodePoolInspectionIdentity(raw []byte) (poolInspectionIdentity, bool) {
	var fields []json.RawMessage
	var identity poolInspectionIdentity
	if !utf8.Valid(raw) || json.Unmarshal(raw, &fields) != nil || len(fields) != 5 ||
		json.Unmarshal(fields[0], &identity.controlSHA) != nil ||
		json.Unmarshal(fields[1], &identity.poolID) != nil ||
		json.Unmarshal(fields[2], &identity.runnerID) != nil ||
		json.Unmarshal(fields[3], &identity.credentialVersion) != nil ||
		json.Unmarshal(fields[4], &identity.configSHA) != nil {
		return identity, false
	}
	return identity, tc.ValidDigest(identity.controlSHA) && tc.ValidDigest(identity.configSHA) &&
		validPoolLocalID(identity.poolID) && validPoolLocalID(identity.runnerID) && identity.credentialVersion > 0
}

func inspectPoolClaimBytes(raw []byte, slot string, identity poolInspectionIdentity) string {
	fields, err := decodeToolObjectBounded(raw, 262144)
	if err != nil {
		return "invalid"
	}
	// encoding/json accepts case-insensitive field names. Require the exact
	// checkpoint and request keys so case variants cannot shadow these facts.
	if len(fields) == 8 {
		if _, ok := fields["assignment"].(map[string]any); !ok {
			return "invalid"
		}
	} else if len(fields) != 7 {
		return "invalid"
	}
	for _, key := range []string{"schema_version", "control_sha256", "config_sha256", "credential_version", "request", "responded", "complete"} {
		if _, ok := fields[key]; !ok {
			return "invalid"
		}
	}
	request, ok := fields["request"].(map[string]any)
	if !ok || len(request) != 6 {
		return "invalid"
	}
	for _, key := range []string{"pool_id", "runner_id", "worker_id", "slot_id", "operation_id", "scopes"} {
		if _, ok := request[key]; !ok {
			return "invalid"
		}
	}
	// The current writer always emits both flags. A missing or null flag must
	// not be silently projected as a persisted false value.
	if _, ok := fields["responded"].(bool); !ok {
		return "invalid"
	}
	if _, ok := fields["complete"].(bool); !ok {
		return "invalid"
	}
	var claim poolClaimCheckpoint
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&claim) != nil || claim.SchemaVersion != 1 ||
		claim.ControlSHA256 != identity.controlSHA || claim.ConfigSHA256 != identity.configSHA ||
		claim.CredentialVersion != identity.credentialVersion ||
		claim.Request.PoolID != identity.poolID || claim.Request.RunnerID != identity.runnerID ||
		claim.Request.WorkerID != "pool-"+identity.runnerID || claim.Request.SlotID != slot ||
		!validPoolInspectionOperation(claim.Request.OperationID) ||
		!validPoolInspectionClaimScope(claim) {
		return "invalid"
	}
	switch {
	case claim.Complete:
		return "complete"
	case claim.Responded:
		return "response_recorded"
	default:
		return "request_recorded"
	}
}

func validPoolInspectionOperation(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, digit := range value {
		if !(digit >= '0' && digit <= '9' || digit >= 'a' && digit <= 'f') {
			return false
		}
	}
	return true
}

func validPoolInspectionClaimScope(claim poolClaimCheckpoint) bool {
	if !claim.Responded && (claim.Complete || claim.Assignment != nil) || len(claim.Request.Scopes) > 256 || claim.Request.Scopes == nil {
		return false
	}
	scopes := map[[2]string]bool{}
	for _, scope := range claim.Request.Scopes {
		key := [2]string{scope.TenantID, scope.ProjectID}
		if !validPoolLocalID(scope.TenantID) || !validPoolLocalID(scope.ProjectID) || scopes[key] {
			return false
		}
		scopes[key] = true
	}
	a := claim.Assignment
	if a == nil {
		return true
	}
	return validPoolAssignment(*a) == nil && a.PoolID == claim.Request.PoolID &&
		a.PoolRunnerID == claim.Request.RunnerID && a.WorkerID == claim.Request.WorkerID &&
		a.PoolSlotID == claim.Request.SlotID && a.PoolCredentialVersion == claim.CredentialVersion &&
		scopes[[2]string{a.TenantID, a.ProjectID}]
}

func poolInspectionClaimObserved(state string) bool {
	switch state {
	case "missing", "request_recorded", "response_recorded", "complete":
		return true
	default:
		return false
	}
}
