package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func historyShareInvalid() error {
	return &APIError{400, "workbuddy_cloud_history_share_invalid", "workbuddy_cloud_history_share_invalid", nil}
}
func historyShareChanged() error {
	return &APIError{409, "workbuddy_cloud_history_share_changed", "workbuddy_cloud_history_share_changed", nil}
}

// Preview and commit use the same bounded canonical facts, in the caller's one
// current browser-write transaction. No provider or OAuth material is needed.
func (w *WorkBuddyConnections) cloudHistorySharePreviewTx(ctx context.Context, a *authorizedCommand, loadID string, req wc.CloudHistorySharePreviewRequest) (wc.CloudHistorySharePreview, error) {
	if req.Validate() != nil || req.ProjectID != a.target.projectID || a.target.resourceID != req.TaskID {
		return wc.CloudHistorySharePreview{}, historyShareInvalid()
	}
	row, err := w.cloudHistoryLoadTx(ctx, a, loadID)
	if err != nil {
		return wc.CloudHistorySharePreview{}, err
	}
	source, _, err := w.cloudHistorySourceTx(ctx, a, row.load.Source.SubmissionID, false)
	if err != nil {
		return wc.CloudHistorySharePreview{}, err
	}
	if !cloudHistorySameSource(source, row.load.Source) {
		return wc.CloudHistorySharePreview{}, historyShareChanged()
	}
	if row.load.Version != req.ExpectedLoadVersion || row.load.State == "loading" {
		return wc.CloudHistorySharePreview{}, historyShareChanged()
	}
	task, err := w.s.getRecordTx(ctx, a, a.target.projectID, "tasks", req.TaskID)
	if err != nil {
		return wc.CloudHistorySharePreview{}, err
	}
	if task.Version != req.ExpectedTaskVersion {
		return wc.CloudHistorySharePreview{}, historyShareChanged()
	}
	marks := make([]string, len(req.Sequences))
	args := []any{a.target.tenantID, a.target.projectID, loadID}
	for i, sequence := range req.Sequences {
		marks[i] = "?"
		args = append(args, sequence)
	}
	rows, err := a.tx.QueryContext(ctx, w.s.q(`SELECT sequence,kind,text FROM suma_workbuddy_cloud_history_entries WHERE tenant_id=? AND project_id=? AND load_id=? AND sequence IN (`+strings.Join(marks, ",")+`) ORDER BY sequence`), args...)
	if err != nil {
		return wc.CloudHistorySharePreview{}, err
	}
	entries := []wc.CloudHistoryEntry{}
	var parts []string
	shared := []wc.CloudHistorySharedEntry{}
	for rows.Next() {
		var entry wc.CloudHistoryEntry
		if err = rows.Scan(&entry.Sequence, &entry.Kind, &entry.Text); err != nil {
			rows.Close()
			return wc.CloudHistorySharePreview{}, err
		}
		if entry.Sequence > row.load.EntryCount {
			rows.Close()
			return wc.CloudHistorySharePreview{}, historyShareChanged()
		}
		if entry.Kind == "artifact" || entry.Kind == "unsupported" || entry.Validate() != nil || strings.TrimSpace(entry.Text) == "" {
			rows.Close()
			return wc.CloudHistorySharePreview{}, historyShareInvalid()
		}
		entries = append(entries, entry)
		parts = append(parts, entry.Text)
		shared = append(shared, wc.CloudHistorySharedEntry{Sequence: entry.Sequence, Kind: entry.Kind, TextSHA256: wc.CloudHistoryTextSHA256(entry.Text)})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return wc.CloudHistorySharePreview{}, err
	}
	if len(entries) != len(req.Sequences) {
		return wc.CloudHistorySharePreview{}, historyShareChanged()
	}
	body := strings.Join(parts, "\n\n")
	if !wc.ValidCloudHistoryShareBody(body) {
		return wc.CloudHistorySharePreview{}, historyShareInvalid()
	}
	selection, err := wc.CloudHistorySelectionSHA256(shared)
	if err != nil {
		return wc.CloudHistorySharePreview{}, historyShareInvalid()
	}
	preview := wc.CloudHistorySharePreview{ProjectID: req.ProjectID, TaskID: req.TaskID, TaskTitle: task.Title, TaskVersion: task.Version, LoadID: row.load.ID, LoadVersion: row.load.Version, LoadState: row.load.State, Coverage: row.load.Coverage, Source: row.load.Source, Entries: entries, Body: body, BodySHA256: wc.CloudHistoryTextSHA256(body), SelectionSHA256: selection, SharingScope: "project_task_readers"}
	// The empty preview_sha256 field is intentionally part of this fixed typed
	// canonical projection; the final hash binds tenant and current human too.
	preview.PreviewSHA256 = digest(stringify(struct {
		Protocol, TenantID, OwnerID string
		Preview                     wc.CloudHistorySharePreview
	}{"workbuddy-cloud-history-share/v1", a.target.tenantID, a.currentPrincipal.ID, preview}))
	if preview.Validate() != nil {
		return wc.CloudHistorySharePreview{}, historyShareInvalid()
	}
	return preview, nil
}

func (w *WorkBuddyConnections) PreviewCloudHistoryShare(ctx context.Context, actor Actor, project, loadID string, req wc.CloudHistorySharePreviewRequest) (wc.CloudHistorySharePreview, error) {
	if w == nil || w.s == nil || req.ProjectID != project || req.Validate() != nil || !handoffID(loadID, false) {
		return wc.CloudHistorySharePreview{}, historyShareInvalid()
	}
	ctx, a, close, err := w.s.planningAuthority(ctx, actor, project, req.TaskID, purposeBrowserProjectWrite)
	if err != nil {
		return wc.CloudHistorySharePreview{}, err
	}
	defer close()
	preview, err := w.cloudHistorySharePreviewTx(ctx, a, loadID, req)
	if err != nil {
		return wc.CloudHistorySharePreview{}, err
	}
	if err = a.assertTx(a.tx, a.target, purposeBrowserProjectWrite); err != nil {
		return wc.CloudHistorySharePreview{}, err
	}
	return preview, nil
}

func (w *WorkBuddyConnections) ShareCloudHistory(ctx context.Context, actor Actor, project, loadID string, req wc.CloudHistoryShareRequest) (TaskConversationMessage, error) {
	if w == nil || w.s == nil || req.ProjectID != project || req.Validate() != nil || !handoffID(loadID, false) {
		return TaskConversationMessage{}, historyShareInvalid()
	}
	ctx, a, close, err := w.s.planningAuthority(ctx, actor, project, req.TaskID, purposeBrowserProjectWrite)
	if err != nil {
		return TaskConversationMessage{}, err
	}
	defer close()
	// Check target existence/current project visibility even when recovering an
	// already copied message. Its later version is not a new-post precondition.
	if _, err = w.s.getRecordTx(ctx, a, project, "tasks", req.TaskID); err != nil {
		return TaskConversationMessage{}, err
	}
	body := struct {
		Kind, LoadID string
		Request      wc.CloudHistoryShareRequest
	}{"workbuddy_cloud_history_share", loadID, req}
	hash, replay, err := w.s.commandReceipt(ctx, a, req.IdempotencyKey, body)
	if err != nil {
		return TaskConversationMessage{}, err
	}
	if replay {
		var original TaskConversationMessage
		if json.Unmarshal([]byte(hash), &original) != nil || !handoffID(original.ID, false) {
			return TaskConversationMessage{}, historyShareChanged()
		}
		saved, err := scanTaskConversation(a.tx.QueryRowContext(ctx, w.s.q(`SELECT `+taskConversationColumns+` FROM suma_task_conversation_messages WHERE tenant_id=? AND project_id=? AND task_id=? AND id=?`), a.target.tenantID, project, req.TaskID, original.ID))
		if errors.Is(err, sql.ErrNoRows) {
			return TaskConversationMessage{}, historyShareChanged()
		}
		if err != nil {
			return TaskConversationMessage{}, err
		}
		items := []TaskConversationMessage{saved}
		if err = w.s.taskConversationMessageMetadataTx(ctx, a.tx, a.target.tenantID, project, req.TaskID, items); err != nil {
			return TaskConversationMessage{}, err
		}
		saved = items[0]
		if saved.AuthorID != a.currentPrincipal.ID || saved.ActorKind != "human" || saved.CloudObservation == nil || saved.CloudObservation.LoadID != loadID || saved.CloudObservation.LoadVersion != req.ExpectedLoadVersion || saved.TaskVersion != req.ExpectedTaskVersion || stringify(saved) != stringify(original) {
			return TaskConversationMessage{}, historyShareChanged()
		}
		if err = a.assertTx(a.tx, a.target, purposeBrowserProjectWrite); err != nil {
			return TaskConversationMessage{}, err
		}
		return saved, nil
	}
	preview, err := w.cloudHistorySharePreviewTx(ctx, a, loadID, req.PreviewRequest())
	if err != nil {
		return TaskConversationMessage{}, err
	}
	if preview.PreviewSHA256 != req.PreviewSHA256 {
		return TaskConversationMessage{}, historyShareChanged()
	}
	appendRequest := TaskConversationAppendRequest{ProjectID: project, TaskID: req.TaskID, ExpectedTaskVersion: req.ExpectedTaskVersion, Body: preview.Body, IdempotencyKey: req.IdempotencyKey}
	message, err := w.s.appendTaskConversationMessageTx(ctx, actor, a, project, appendRequest)
	if err != nil {
		return TaskConversationMessage{}, err
	}
	sharedEntries := make([]wc.CloudHistorySharedEntry, len(preview.Entries))
	for i, entry := range preview.Entries {
		sharedEntries[i] = wc.CloudHistorySharedEntry{Sequence: entry.Sequence, Kind: entry.Kind, TextSHA256: wc.CloudHistoryTextSHA256(entry.Text)}
	}
	observation := wc.CloudHistorySharedObservation{LoadID: preview.LoadID, LoadVersion: preview.LoadVersion, LoadState: preview.LoadState, Coverage: preview.Coverage, SourceRunID: preview.Source.RunID, SourceNodeID: preview.Source.NodeID, SourceSubmissionID: preview.Source.SubmissionID, Entries: sharedEntries, BodySHA256: preview.BodySHA256, SelectionSHA256: preview.SelectionSHA256}
	if err = w.saveCloudHistoryShareTx(ctx, a, message, req, hash, observation); err != nil {
		return TaskConversationMessage{}, err
	}
	message.CloudObservation = &observation
	if err = w.s.saveCommandReceipt(ctx, a, req.IdempotencyKey, hash, message); err != nil {
		return TaskConversationMessage{}, err
	}
	if err = a.assertTx(a.tx, a.target, purposeBrowserProjectWrite); err != nil {
		return TaskConversationMessage{}, err
	}
	if err = a.tx.Commit(); err != nil {
		return TaskConversationMessage{}, err
	}
	return message, nil
}
