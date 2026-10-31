package workbuddycloudcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

const CloudHistoryShareMaxEntries = 8

type CloudHistorySharePreviewRequest struct {
	ProjectID           string `json:"project_id"`
	TaskID              string `json:"task_id"`
	ExpectedTaskVersion int    `json:"expected_task_version"`
	ExpectedLoadVersion int    `json:"expected_load_version"`
	Sequences           []int  `json:"sequences"`
}

func (r CloudHistorySharePreviewRequest) Validate() error {
	if !validID(r.ProjectID, 64) || !validID(r.TaskID, 64) || !expectedVersion(r.ExpectedTaskVersion) || !validVersion(r.ExpectedLoadVersion) || !cloudHistoryShareSequences(r.Sequences) {
		return ErrInvalid
	}
	return nil
}
func cloudHistoryShareSequences(items []int) bool {
	if len(items) < 1 || len(items) > CloudHistoryShareMaxEntries {
		return false
	}
	previous := 0
	for _, item := range items {
		if item <= previous || item > CloudHistoryMaxEntries {
			return false
		}
		previous = item
	}
	return true
}

type CloudHistoryShareRequest struct {
	ProjectID           string `json:"project_id"`
	TaskID              string `json:"task_id"`
	ExpectedTaskVersion int    `json:"expected_task_version"`
	ExpectedLoadVersion int    `json:"expected_load_version"`
	Sequences           []int  `json:"sequences"`
	PreviewSHA256       string `json:"preview_sha256"`
	IdempotencyKey      string `json:"idempotency_key"`
}

func (r CloudHistoryShareRequest) PreviewRequest() CloudHistorySharePreviewRequest {
	return CloudHistorySharePreviewRequest{ProjectID: r.ProjectID, TaskID: r.TaskID, ExpectedTaskVersion: r.ExpectedTaskVersion, ExpectedLoadVersion: r.ExpectedLoadVersion, Sequences: r.Sequences}
}
func (r CloudHistoryShareRequest) Validate() error {
	if r.PreviewRequest().Validate() != nil || !cc.ValidHash(r.PreviewSHA256) || !operationID(r.IdempotencyKey) {
		return ErrInvalid
	}
	return nil
}

type CloudHistorySharePreview struct {
	ProjectID       string                   `json:"project_id"`
	TaskID          string                   `json:"task_id"`
	TaskTitle       string                   `json:"task_title"`
	TaskVersion     int                      `json:"task_version"`
	LoadID          string                   `json:"load_id"`
	LoadVersion     int                      `json:"load_version"`
	LoadState       string                   `json:"load_state"`
	Coverage        string                   `json:"coverage"`
	Source          CloudCollaborationSource `json:"source"`
	Entries         []CloudHistoryEntry      `json:"entries"`
	Body            string                   `json:"body"`
	BodySHA256      string                   `json:"body_sha256"`
	SelectionSHA256 string                   `json:"selection_sha256"`
	PreviewSHA256   string                   `json:"preview_sha256"`
	SharingScope    string                   `json:"sharing_scope"`
}

func (CloudHistorySharePreview) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddycloudcontract.CloudHistorySharePreview{body:redacted}")
}
func (p CloudHistorySharePreview) Validate() error {
	if !validID(p.ProjectID, 64) || !validID(p.TaskID, 64) || !validText(p.TaskTitle, 500, false) || !validVersion(p.TaskVersion) || !validID(p.LoadID, 64) || !validVersion(p.LoadVersion) || !cloudHistoryShareTerminal(p.LoadState) || p.Coverage != "unverified" || p.Source.Validate() != nil || p.Source.ProjectID != p.ProjectID || p.SharingScope != "project_task_readers" || !cc.ValidHash(p.PreviewSHA256) || len(p.Entries) < 1 || len(p.Entries) > CloudHistoryShareMaxEntries {
		return ErrInvalid
	}
	var parts []string
	var entries []CloudHistorySharedEntry
	previous := 0
	for _, entry := range p.Entries {
		if entry.Validate() != nil || entry.Sequence <= previous || !cloudHistoryShareKind(entry.Kind) || strings.TrimSpace(entry.Text) == "" {
			return ErrInvalid
		}
		parts = append(parts, entry.Text)
		entries = append(entries, CloudHistorySharedEntry{Sequence: entry.Sequence, Kind: entry.Kind, TextSHA256: CloudHistoryTextSHA256(entry.Text)})
		previous = entry.Sequence
	}
	selected, err := CloudHistorySelectionSHA256(entries)
	if err != nil || p.Body != strings.Join(parts, "\n\n") || !ValidCloudHistoryShareBody(p.Body) || p.BodySHA256 != CloudHistoryTextSHA256(p.Body) || p.SelectionSHA256 != selected {
		return ErrInvalid
	}
	return nil
}
func ValidCloudHistoryShareBody(body string) bool {
	return utf8.ValidString(body) && strings.TrimSpace(body) != "" && len(body) <= 32000 && utf8.RuneCountInString(body) <= 8000 && !strings.ContainsRune(body, 0) && (CloudHistoryEntry{Sequence: 1, Kind: "user_text", Text: body}).Validate() == nil
}
func cloudHistoryShareTerminal(state string) bool {
	return state == "loaded" || state == "interrupted" || state == "limited" || state == "rejected"
}
func cloudHistoryShareKind(kind string) bool {
	return kind == "user_text" || kind == "assistant_text" || kind == "tool_summary" || kind == "plan_summary"
}

type CloudHistorySharedEntry struct {
	Sequence   int    `json:"sequence"`
	Kind       string `json:"kind"`
	TextSHA256 string `json:"text_sha256"`
}

func (e CloudHistorySharedEntry) Validate() error {
	if e.Sequence < 1 || e.Sequence > CloudHistoryMaxEntries || !cloudHistoryShareKind(e.Kind) || !cc.ValidHash(e.TextSHA256) {
		return ErrInvalid
	}
	return nil
}
func CloudHistoryTextSHA256(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}
func CloudHistorySelectionSHA256(entries []CloudHistorySharedEntry) (string, error) {
	if len(entries) < 1 || len(entries) > CloudHistoryShareMaxEntries {
		return "", ErrInvalid
	}
	previous := 0
	for _, entry := range entries {
		if entry.Validate() != nil || entry.Sequence <= previous {
			return "", ErrInvalid
		}
		previous = entry.Sequence
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return "", ErrInvalid
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

// This is the immutable public attribution, deliberately smaller than the
// private source. It grants no right to retrieve an owner's private snapshot.
type CloudHistorySharedObservation struct {
	LoadID             string                    `json:"load_id"`
	LoadVersion        int                       `json:"load_version"`
	LoadState          string                    `json:"load_state"`
	Coverage           string                    `json:"coverage"`
	SourceRunID        string                    `json:"source_run_id"`
	SourceNodeID       string                    `json:"source_node_id"`
	SourceSubmissionID string                    `json:"source_submission_id"`
	Entries            []CloudHistorySharedEntry `json:"entries"`
	BodySHA256         string                    `json:"body_sha256"`
	SelectionSHA256    string                    `json:"selection_sha256"`
}

func (o CloudHistorySharedObservation) Validate() error {
	if !validID(o.LoadID, 64) || !validVersion(o.LoadVersion) || !cloudHistoryShareTerminal(o.LoadState) || o.Coverage != "unverified" || !validID(o.SourceRunID, 100) || !validID(o.SourceNodeID, 150) || !validID(o.SourceSubmissionID, 64) || !cc.ValidHash(o.BodySHA256) {
		return ErrInvalid
	}
	selected, err := CloudHistorySelectionSHA256(o.Entries)
	if err != nil || selected != o.SelectionSHA256 {
		return ErrInvalid
	}
	return nil
}
