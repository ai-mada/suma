package platform

import (
	"context"
	"strings"
	"testing"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func historySharingFixture(t *testing.T) (*cloudFixture, wc.CloudHistoryLoad, wc.CloudHistorySharePreviewRequest) {
	t.Helper()
	h, sub, _ := cloudHistoryFixture(t, []string{cloudHistoryText("user_message_chunk", "  shared user  "), cloudHistoryText("agent_message_chunk", "shared answer"), cloudHistoryText("agent_thought_chunk", "never share thought")}, nil)
	load, err := h.w.LoadCloudHistory(context.Background(), h.f.actor, h.f.project, sub.ID, cloudHistoryRequest(h, sub))
	if err != nil {
		t.Fatal(err)
	}
	if err = h.f.s.InitializeWorkBuddyCloudHistorySharing(context.Background()); err != nil {
		t.Fatal(err)
	}
	task := h.f.call("POST", "/tasks", h.f.body(map[string]any{"title": "Explicit share target"}), 201)
	return h, load, wc.CloudHistorySharePreviewRequest{ProjectID: h.f.project, TaskID: task["id"].(string), ExpectedTaskVersion: 1, ExpectedLoadVersion: load.Version, Sequences: []int{1, 2}}
}
func historySharingRequest(req wc.CloudHistorySharePreviewRequest, sha string) wc.CloudHistoryShareRequest {
	return wc.CloudHistoryShareRequest{ProjectID: req.ProjectID, TaskID: req.TaskID, ExpectedTaskVersion: req.ExpectedTaskVersion, ExpectedLoadVersion: req.ExpectedLoadVersion, Sequences: req.Sequences, PreviewSHA256: sha, IdempotencyKey: newID()}
}

func TestWorkBuddyCloudHistorySharingPreviewIsLocalAndShareIsImmutableHumanCopy(t *testing.T) {
	h, load, req := historySharingFixture(t)
	ctx := context.Background()
	tables := []string{"suma_task_conversation_heads", "suma_task_conversation_messages", "suma_workbuddy_cloud_history_shares", "suma_operations", "suma_activity", "suma_notification_inbox"}
	counts := map[string]int{}
	for _, table := range tables {
		counts[table] = domainACount(t, h.f.s, table)
	}
	preview, err := h.w.PreviewCloudHistoryShare(ctx, h.f.actor, h.f.project, load.ID, req)
	if err != nil || preview.Validate() != nil || preview.Body != "  shared user  \n\nshared answer" || preview.SharingScope != "project_task_readers" {
		t.Fatal("exact local preview", err)
	}
	for table, count := range counts {
		if domainACount(t, h.f.s, table) != count {
			t.Fatal("preview wrote formal state", table)
		}
	}
	share := historySharingRequest(req, preview.PreviewSHA256)
	message, err := h.w.ShareCloudHistory(ctx, h.f.actor, h.f.project, load.ID, share)
	if err != nil || message.Body != preview.Body || message.ActorKind != "human" || message.AuthorID != h.f.user.ID || message.CloudObservation == nil || message.CloudObservation.SelectionSHA256 != preview.SelectionSHA256 {
		t.Fatal("explicit human share", err)
	}
	other := cloudOtherWriter(t, h)
	page, err := h.f.s.ListTaskConversation(ctx, other, h.f.project, req.TaskID, 20, 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].CloudObservation == nil || page.Items[0].Body != message.Body {
		t.Fatal("target reader cannot read shared copy", err)
	}
	if _, err = h.w.CloudHistory(ctx, other, h.f.project, load.ID, 0, 50); err == nil {
		t.Fatal("sharing conferred private archive access")
	}
	if strings.Contains(stringify(message.CloudObservation), "cloud-task") || strings.Contains(stringify(message.CloudObservation), load.OperationID) {
		t.Fatal("private source metadata leaked")
	}
	// The old source is now unavailable. A committed original operation recovers
	// the ordinary message without another private-source read or new post.
	if _, err = h.f.s.DB.Exec(`DELETE FROM suma_workbuddy_cloud_history_entries WHERE load_id=?`, load.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.f.s.DB.Exec(`DELETE FROM dag_nodes WHERE run_id=?`, load.Source.RunID); err != nil {
		t.Fatal(err)
	}
	h.f.call("PATCH", "/tasks/"+req.TaskID, h.f.body(map[string]any{"expected_version": 1, "title": "Later target version"}), 200)
	replay, err := h.w.ShareCloudHistory(ctx, h.f.actor, h.f.project, load.ID, share)
	if err != nil || stringify(replay) != stringify(message) || domainACount(t, h.f.s, "suma_task_conversation_messages") != 1 {
		t.Fatal("original receipt did not recover independently of old source", err)
	}
	share.Sequences = []int{1}
	if _, err = h.w.ShareCloudHistory(ctx, h.f.actor, h.f.project, load.ID, share); err == nil {
		t.Fatal("changed original operation accepted")
	}
}

func TestWorkBuddyCloudHistorySharingSourceSelectionAndTaskVersionAreRechecked(t *testing.T) {
	h, load, req := historySharingFixture(t)
	ctx := context.Background()
	for _, selection := range [][]int{{3}, {2, 1}, {1, 1}, {500}} {
		bad := req
		bad.Sequences = selection
		if _, err := h.w.PreviewCloudHistoryShare(ctx, h.f.actor, h.f.project, load.ID, bad); err == nil {
			t.Fatal("invalid or unsupported selection previewed")
		}
	}
	preview, err := h.w.PreviewCloudHistoryShare(ctx, h.f.actor, h.f.project, load.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	share := historySharingRequest(req, preview.PreviewSHA256)
	if _, err = h.f.s.DB.Exec(`UPDATE suma_workbuddy_cloud_history_entries SET text='changed selected text' WHERE load_id=? AND sequence=1`, load.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.w.ShareCloudHistory(ctx, h.f.actor, h.f.project, load.ID, share); err == nil {
		t.Fatal("changed source text retained old preview")
	}
	if domainACount(t, h.f.s, "suma_task_conversation_messages") != 0 {
		t.Fatal("source conflict wrote message")
	}
	if _, err = h.f.s.DB.Exec(`UPDATE suma_workbuddy_cloud_history_entries SET text='  shared user  ' WHERE load_id=? AND sequence=1`, load.ID); err != nil {
		t.Fatal(err)
	}
	h.f.call("PATCH", "/tasks/"+req.TaskID, h.f.body(map[string]any{"expected_version": 1, "title": "Explicit changed target"}), 200)
	if _, err = h.w.ShareCloudHistory(ctx, h.f.actor, h.f.project, load.ID, share); err == nil {
		t.Fatal("target version changed after preview")
	}
}

func TestWorkBuddyCloudHistorySharingRelationFailureRollsBackConversation(t *testing.T) {
	h, load, req := historySharingFixture(t)
	ctx := context.Background()
	reader := cloudOtherWriter(t, h)
	subscriptionCommand(t, h.f, reader, req.TaskID, 0, "follow", newID())
	preview, err := h.w.PreviewCloudHistoryShare(ctx, h.f.actor, h.f.project, load.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	before := map[string]int{}
	for _, table := range []string{"suma_task_conversation_heads", "suma_task_conversation_messages", "suma_workbuddy_cloud_history_shares", "suma_operations", "suma_activity", "suma_notification_inbox"} {
		before[table] = domainACount(t, h.f.s, table)
	}
	if _, err = h.f.s.DB.Exec(`CREATE TRIGGER reject_history_share BEFORE INSERT ON suma_workbuddy_cloud_history_shares BEGIN SELECT RAISE(ABORT,'controlled sharing relation failure');END`); err != nil {
		t.Fatal(err)
	}
	if _, err = h.w.ShareCloudHistory(ctx, h.f.actor, h.f.project, load.ID, historySharingRequest(req, preview.PreviewSHA256)); err == nil {
		t.Fatal("message escaped failed source relation")
	}
	for table, count := range before {
		if domainACount(t, h.f.s, table) != count {
			t.Fatal("partial share committed", table)
		}
	}
}
