package runner

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"unicode"
	"unicode/utf8"
)

const journalInspectionMaxBytes int64 = 16 << 20
const journalInspectionItemLimit = 100

// JournalInspection is a read-only projection of one local journal.json. It
// never carries an Assignment, Result, Progress or raw OS/JSON error: those
// types can contain credentials, command arguments, source files and logs.
type JournalInspection struct {
	SchemaVersion int                     `json:"schema_version"`
	Component     string                  `json:"component"`
	Status        string                  `json:"status"`
	Limit         int                     `json:"limit"`
	Complete      bool                    `json:"complete"`
	Counts        JournalInspectionCounts `json:"counts"`
	Items         []JournalInspectionItem `json:"items"`
}

type JournalInspectionCounts struct {
	Entries             int `json:"entries"`
	PendingResults      int `json:"pending_results"`
	AcknowledgedResults int `json:"acknowledged_results"`
	UnknownResults      int `json:"unknown_results"`
	PendingProgress     int `json:"pending_progress"`
	UnresolvedInbox     int `json:"unresolved_inbox"`
	PendingArtifacts    int `json:"pending_artifacts"`
	PendingCandidates   int `json:"pending_candidates"`
	TruncatedProgress   int `json:"truncated_progress"`
}

type JournalInspectionItem struct {
	EntryRef           string `json:"entry_ref"`
	RunID              string `json:"run_id,omitempty"`
	NodeID             string `json:"node_id,omitempty"`
	Attempt            int    `json:"attempt"`
	Fence              int64  `json:"fence"`
	ResultRecorded     bool   `json:"result_recorded"`
	ResultAcknowledged bool   `json:"result_acknowledged"`
	OutcomeUnknown     bool   `json:"outcome_unknown"`
	PendingProgress    int    `json:"pending_progress"`
	ProgressTruncated  bool   `json:"progress_truncated"`
	ArtifactPending    bool   `json:"artifact_pending"`
	CandidatePending   bool   `json:"candidate_pending"`
	IdentifiersOmitted bool   `json:"identifiers_omitted"`
}

// Decode only the metadata needed for a public projection. Unknown fields are
// skipped by encoding/json rather than retained as private runtime structs.
type inspectionDiskEntry struct {
	Assignment *struct {
		RunID   string `json:"run_id"`
		NodeID  string `json:"node_id"`
		Attempt int    `json:"attempt"`
		Fence   int64  `json:"fence"`
	} `json:"assignment"`
	Result *struct {
		OutcomeUnknown bool `json:"outcome_unknown"`
	} `json:"result"`
	Acked    bool `json:"acked"`
	Progress []struct {
		Acked         bool              `json:"acked"`
		Rejection     ProgressRejection `json:"rejection"`
		HostOperation *struct{}         `json:"host_operation"`
	} `json:"progress"`
	ProgressTruncated bool      `json:"progress_truncated"`
	ArtifactPending   *struct{} `json:"artifact_pending"`
	CandidatePending  *struct{} `json:"candidate_pending"`
}

func newJournalInspection(status string) JournalInspection {
	return JournalInspection{SchemaVersion: 1, Component: "local_execution_journal", Status: status, Limit: journalInspectionItemLimit, Items: []JournalInspectionItem{}}
}

// InspectJournalState does not call OpenJournal: opening the runtime journal
// takes a write lock, creates state, and records restart outcomes. Inspection
// instead reads a bounded regular file through an anchored directory handle.
// A concurrent atomic replace can yield the previous or next complete file;
// this is a local single-file observation, never current server acceptance.
func InspectJournalState(dir string) JournalInspection {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return newJournalInspection("unreadable")
	}
	info, err := os.Lstat(abs)
	if os.IsNotExist(err) {
		return newJournalInspection("missing")
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return newJournalInspection("unreadable")
	}
	// Refuse linked ancestors as well as the final directory. No recursive
	// discovery or payload is read outside the explicitly selected directory.
	for ancestor := filepath.Dir(abs); ; ancestor = filepath.Dir(ancestor) {
		parent, e := os.Lstat(ancestor)
		if e != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 {
			return newJournalInspection("unreadable")
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
	root, err := os.OpenRoot(abs)
	if err != nil {
		return newJournalInspection("unreadable")
	}
	defer root.Close()
	// Bind the opened directory to the identity inspected before OpenRoot; a
	// concurrent directory replacement must not retarget this diagnostic read.
	anchored, err := root.Stat(".")
	if err != nil || !os.SameFile(info, anchored) {
		return newJournalInspection("unreadable")
	}
	before, err := root.Lstat("journal.json")
	if os.IsNotExist(err) {
		return newJournalInspection("missing")
	}
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		return newJournalInspection("unreadable")
	}
	if before.Size() > journalInspectionMaxBytes {
		return newJournalInspection("over_limit")
	}
	f, err := root.Open("journal.json")
	if err != nil {
		return newJournalInspection("unreadable")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return newJournalInspection("unreadable")
	}
	if opened.Size() > journalInspectionMaxBytes {
		return newJournalInspection("over_limit")
	}
	raw, err := io.ReadAll(io.LimitReader(f, journalInspectionMaxBytes+1))
	if err != nil {
		return newJournalInspection("unreadable")
	}
	if int64(len(raw)) > journalInspectionMaxBytes {
		return newJournalInspection("over_limit")
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(opened, after) || after.Size() != opened.Size() || !after.ModTime().Equal(opened.ModTime()) {
		return newJournalInspection("unreadable")
	}
	return inspectJournalBytes(raw)
}

func inspectJournalBytes(raw []byte) JournalInspection {
	invalid := newJournalInspection("invalid")
	if !utf8.Valid(raw) {
		return invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return invalid
	}
	out := newJournalInspection("available")
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return invalid
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return invalid
		}
		seen[key] = true
		var disk inspectionDiskEntry
		if decoder.Decode(&disk) != nil || disk.Assignment == nil || disk.Assignment.RunID == "" || disk.Assignment.NodeID == "" || disk.Assignment.Attempt < 1 || disk.Assignment.Fence < 1 || disk.Acked && disk.Result == nil {
			return invalid
		}
		identity := disk.Assignment
		digest := sha256.Sum256([]byte(key))
		item := JournalInspectionItem{EntryRef: hex.EncodeToString(digest[:]), Attempt: identity.Attempt, Fence: identity.Fence, ResultRecorded: disk.Result != nil, ResultAcknowledged: disk.Acked, ProgressTruncated: disk.ProgressTruncated, ArtifactPending: disk.ArtifactPending != nil, CandidatePending: disk.CandidatePending != nil}
		if safeInspectionIdentifier(identity.RunID) {
			item.RunID = identity.RunID
		} else {
			item.IdentifiersOmitted = true
		}
		if safeInspectionIdentifier(identity.NodeID) {
			item.NodeID = identity.NodeID
		} else {
			item.IdentifiersOmitted = true
		}
		out.Counts.Entries++
		if disk.Result != nil {
			item.OutcomeUnknown = disk.Result.OutcomeUnknown
			if disk.Acked {
				out.Counts.AcknowledgedResults++
			} else {
				out.Counts.PendingResults++
				if disk.Result.OutcomeUnknown {
					out.Counts.UnknownResults++
				}
			}
		} else if disk.ArtifactPending == nil && disk.CandidatePending == nil {
			out.Counts.UnresolvedInbox++
		}
		for _, progress := range disk.Progress {
			if !validProgressDisposition(progress.Acked, progress.Rejection, progress.HostOperation != nil) {
				return invalid
			}
			if !progress.Acked && progress.Rejection == "" {
				item.PendingProgress++
				out.Counts.PendingProgress++
			}
		}
		if disk.ProgressTruncated {
			out.Counts.TruncatedProgress++
		}
		if disk.ArtifactPending != nil {
			out.Counts.PendingArtifacts++
		}
		if disk.CandidatePending != nil {
			out.Counts.PendingCandidates++
		}
		// Retain only the deterministic first 100 opaque references in memory;
		// counts cover all entries, never just the visible item window.
		if len(out.Items) < journalInspectionItemLimit {
			out.Items = append(out.Items, item)
			sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].EntryRef < out.Items[j].EntryRef })
		} else if item.EntryRef < out.Items[len(out.Items)-1].EntryRef {
			out.Items[len(out.Items)-1] = item
			sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].EntryRef < out.Items[j].EntryRef })
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return invalid
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return invalid
	}
	out.Complete = out.Counts.Entries <= journalInspectionItemLimit
	return out
}

func safeInspectionIdentifier(value string) bool {
	if value == "" || len(value) > 400 || utf8.RuneCountInString(value) > 100 {
		return false
	}
	for _, char := range value {
		if !unicode.IsLetter(char) && !unicode.IsNumber(char) && char != '_' && char != '.' && char != ':' && char != '-' {
			return false
		}
	}
	return true
}
