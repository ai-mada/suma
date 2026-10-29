package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strconv"
)

// Binding contains references and a digest only. It must never contain the
// provider token, prompt, ACP credentials or a returned sandbox address.
type WorkBuddyBinding struct {
	RequestSHA256     string `json:"request_sha256"`
	CredentialRef     string `json:"credential_ref"`
	CredentialVersion int    `json:"credential_version"`
	Endpoint          string `json:"endpoint"`
}

// Prepared means the request may already have reached the provider. Only the
// call that durably creates it receives the one-time submission permission.
type WorkBuddyCheckpoint struct {
	Binding WorkBuddyBinding `json:"binding"`
	State   string           `json:"state"`
	TaskID  string           `json:"task_id,omitempty"`
}
type workBuddyJournalCheckpoint struct {
	Version          int              `json:"version"`
	AssignmentSHA256 string           `json:"assignment_sha256"`
	Binding          WorkBuddyBinding `json:"binding"`
	State            string           `json:"state"`
	TaskID           string           `json:"task_id,omitempty"`
}

var workBuddyID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,149}$`)
var workBuddyTaskID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)
var workBuddyDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Reject duplicate/aliased/unknown checkpoint fields rather than silently
// deleting unrecognized content when the journal is next persisted.
func (p *workBuddyJournalCheckpoint) UnmarshalJSON(raw []byte) error {
	if len(raw) > 4096 {
		return fmt.Errorf("invalid WorkBuddy checkpoint")
	}
	object, err := decodeToolObject(raw)
	if err != nil {
		return fmt.Errorf("invalid WorkBuddy checkpoint")
	}
	allowed := map[string]bool{"version": true, "assignment_sha256": true, "binding": true, "state": true, "task_id": true}
	for field := range object {
		if !allowed[field] {
			return fmt.Errorf("invalid WorkBuddy checkpoint field")
		}
	}
	binding, ok := object["binding"].(map[string]any)
	if !ok || len(binding) != 4 {
		return fmt.Errorf("invalid WorkBuddy checkpoint binding")
	}
	for _, field := range []string{"request_sha256", "credential_ref", "credential_version", "endpoint"} {
		if _, ok = binding[field]; !ok {
			return fmt.Errorf("invalid WorkBuddy checkpoint binding")
		}
	}
	type plain workBuddyJournalCheckpoint
	var decoded plain
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&decoded) != nil || decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("invalid WorkBuddy checkpoint")
	}
	*p = workBuddyJournalCheckpoint(decoded)
	return nil
}

func validWorkBuddyBinding(b WorkBuddyBinding) bool {
	if !workBuddyDigest.MatchString(b.RequestSHA256) || !workBuddyID.MatchString(b.CredentialRef) || b.CredentialVersion < 1 || len(b.Endpoint) > 512 {
		return false
	}
	u, err := url.Parse(b.Endpoint)
	if err != nil || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.RawPath != "" || u.Path != "/openapi/v2/tasks" {
		return false
	}
	if u.Scheme == "https" && u.Host == "www.workbuddy.cn" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
			return false
		}
	}
	return u.Host != ""
}
func workBuddyAssignmentDigest(a Assignment) (string, error) {
	for _, id := range []string{a.TenantID, a.ProjectID, a.RunID, a.NodeID, a.WorkerID} {
		if !workBuddyID.MatchString(id) {
			return "", fmt.Errorf("invalid WorkBuddy assignment scope")
		}
	}
	if a.Attempt < 1 || a.Fence < 1 {
		return "", fmt.Errorf("invalid WorkBuddy attempt identity")
	}
	if a.PoolID != "" {
		if validPoolAssignment(a) != nil {
			return "", fmt.Errorf("invalid WorkBuddy pool scope")
		}
	} else if a.PoolRunnerID != "" || a.PoolSlotID != "" || a.PoolGrantID != "" || a.PoolGrantVersion != 0 || a.PoolCredentialID != "" || a.PoolCredentialVersion != 0 {
		return "", fmt.Errorf("invalid WorkBuddy private scope")
	}
	raw, err := json.Marshal(a)
	if err != nil {
		return "", fmt.Errorf("invalid WorkBuddy assignment")
	}
	return digest(raw), nil
}
func workBuddyMixed(e entry) bool {
	return e.Terminal != nil || e.ModelGateway != nil || e.WorkspaceTrace != nil || e.ArtifactPending != nil || e.CandidatePending != nil || e.Assignment.ReleaseExecution != nil || e.Assignment.RepositoryIntegration != nil
}
func validWorkBuddyCheckpoint(a Assignment, inboxKey string, p workBuddyJournalCheckpoint) error {
	hash, err := workBuddyAssignmentDigest(a)
	if err != nil || inboxKey != key(a) || p.Version != 1 || hash != p.AssignmentSHA256 || !validWorkBuddyBinding(p.Binding) || (p.State != "prepared" && p.State != "task_known") || p.State == "prepared" && p.TaskID != "" || p.State == "task_known" && !workBuddyTaskID.MatchString(p.TaskID) {
		return fmt.Errorf("WorkBuddy checkpoint differs from its durable assignment or binding")
	}
	return nil
}

// Check the persisted inbox as well as memory. Accept's generic failure path
// can retain an unpersisted entry, and generic assignment fields may alias a
// caller's maps. Neither can authorize a chargeable cloud request.
func (j *Journal) workBuddyInboxLocked(a Assignment) (entry, error) {
	if j.unlock == nil {
		return entry{}, fmt.Errorf("WorkBuddy journal is closed")
	}
	hash, err := workBuddyAssignmentDigest(a)
	if err != nil {
		return entry{}, err
	}
	k := key(a)
	e, exists := j.entries[k]
	if !exists {
		return entry{}, fmt.Errorf("WorkBuddy durable inbox is absent")
	}
	memoryHash, err := workBuddyAssignmentDigest(e.Assignment)
	if err != nil || hash != memoryHash {
		return entry{}, fmt.Errorf("WorkBuddy assignment scope changed")
	}
	raw, err := os.ReadFile(j.path)
	if err != nil {
		return entry{}, fmt.Errorf("WorkBuddy durable inbox is unavailable")
	}
	var diskEntries map[string]json.RawMessage
	if json.Unmarshal(raw, &diskEntries) != nil {
		return entry{}, fmt.Errorf("WorkBuddy durable inbox is corrupt")
	}
	diskRaw, exists := diskEntries[k]
	if !exists {
		return entry{}, fmt.Errorf("WorkBuddy inbox was not durably accepted")
	}
	if _, err = decodeToolObjectBounded(diskRaw, 262144); err != nil {
		return entry{}, fmt.Errorf("WorkBuddy durable inbox is corrupt")
	}
	var disk entry
	if json.Unmarshal(diskRaw, &disk) != nil {
		return entry{}, fmt.Errorf("WorkBuddy durable inbox is corrupt")
	}
	diskHash, err := workBuddyAssignmentDigest(disk.Assignment)
	if err != nil || diskHash != hash || !reflect.DeepEqual(disk.WorkBuddy, e.WorkBuddy) || (disk.Result == nil) != (e.Result == nil) || disk.Acked != e.Acked || workBuddyMixed(disk) || workBuddyMixed(e) {
		return entry{}, fmt.Errorf("WorkBuddy inbox differs from durable execution facts")
	}
	if e.WorkBuddy != nil {
		if err = validWorkBuddyCheckpoint(e.Assignment, k, *e.WorkBuddy); err != nil {
			return entry{}, err
		}
	}
	return e, nil
}

func (j *Journal) BeginWorkBuddySubmission(a Assignment, b WorkBuddyBinding) (bool, error) {
	if !validWorkBuddyBinding(b) {
		return false, fmt.Errorf("invalid WorkBuddy submission binding")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	e, err := j.workBuddyInboxLocked(a)
	if err != nil {
		return false, err
	}
	if e.Result != nil || e.Acked {
		return false, fmt.Errorf("WorkBuddy assignment is final")
	}
	if e.WorkBuddy != nil {
		if e.WorkBuddy.Binding != b {
			return false, fmt.Errorf("WorkBuddy submission binding changed")
		}
		return false, nil
	}
	hash, err := workBuddyAssignmentDigest(a)
	if err != nil {
		return false, err
	}
	previous := e
	e.WorkBuddy = &workBuddyJournalCheckpoint{Version: 1, AssignmentSHA256: hash, Binding: b, State: "prepared"}
	j.entries[key(a)] = e
	if err = j.save(); err != nil {
		j.entries[key(a)] = previous
		return false, err
	}
	return true, nil
}
func (j *Journal) RecordWorkBuddyTask(a Assignment, b WorkBuddyBinding, taskID string) error {
	if !workBuddyTaskID.MatchString(taskID) || !validWorkBuddyBinding(b) {
		return fmt.Errorf("invalid WorkBuddy task receipt")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	e, err := j.workBuddyInboxLocked(a)
	if err != nil {
		return err
	}
	if e.Result != nil || e.Acked || e.WorkBuddy == nil || e.WorkBuddy.Binding != b {
		return fmt.Errorf("WorkBuddy task receipt lacks its active original submission")
	}
	if e.WorkBuddy.State == "task_known" {
		if e.WorkBuddy.TaskID != taskID {
			return fmt.Errorf("WorkBuddy original task cannot be replaced")
		}
		return nil
	}
	previous := e
	checkpoint := *e.WorkBuddy
	checkpoint.State = "task_known"
	checkpoint.TaskID = taskID
	e.WorkBuddy = &checkpoint
	j.entries[key(a)] = e
	if err = j.save(); err != nil {
		j.entries[key(a)] = previous
		return err
	}
	return nil
}
func (j *Journal) WorkBuddySubmission(a Assignment) (WorkBuddyCheckpoint, bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, err := j.workBuddyInboxLocked(a)
	if err != nil {
		return WorkBuddyCheckpoint{}, false, err
	}
	if e.WorkBuddy == nil {
		return WorkBuddyCheckpoint{}, false, nil
	}
	p := e.WorkBuddy
	return WorkBuddyCheckpoint{Binding: p.Binding, State: p.State, TaskID: p.TaskID}, true, nil
}
