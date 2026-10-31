package platform

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ai-mada/suma/internal/store"
)

func TestWorkBuddyCloudHistorySharingCurrentHumanScopeAndNoExternalIO(t *testing.T) {
	h, load, req := historySharingFixture(t)
	ctx := context.Background()
	var outside atomic.Int32
	h.w.app.HTTPClient = &http.Client{Transport: cloudHistoryRoundTripper(func(*http.Request) (*http.Response, error) {
		outside.Add(1)
		return nil, errors.New("unexpected share external call")
	})}
	other := cloudOtherWriter(t, h)
	if _, err := h.w.PreviewCloudHistoryShare(ctx, other, h.f.project, load.ID, req); err == nil {
		t.Fatal("other member previewed private source")
	}
	issued, err := h.f.s.IssueProjectCredential(ctx, h.f.actor, ProjectCredentialIssueRequest{ProjectID: h.f.project, Scope: "api", Label: "share API", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	api := domainCActor(t, h.f.s, issued.Token)
	if _, err = h.w.PreviewCloudHistoryShare(ctx, api, h.f.project, load.ID, req); err == nil {
		t.Fatal("API credential became human share")
	}
	foreign := h.f.call("POST", "/projects", map[string]any{"title": "Foreign share project", "idempotency_key": newID()}, 201)["id"].(string)
	cross := req
	cross.ProjectID = foreign
	if _, err = h.w.PreviewCloudHistoryShare(ctx, h.f.actor, foreign, load.ID, cross); err == nil {
		t.Fatal("cross-project source previewed")
	}
	preview, err := h.w.PreviewCloudHistoryShare(ctx, h.f.actor, h.f.project, load.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	share := historySharingRequest(req, preview.PreviewSHA256)
	if _, err = h.w.ShareCloudHistory(ctx, h.f.actor, h.f.project, load.ID, share); err != nil {
		t.Fatal(err)
	}
	if _, err = h.w.ShareCloudHistory(ctx, h.f.actor, h.f.project, load.ID, share); err != nil {
		t.Fatal(err)
	}
	if outside.Load() != 0 {
		t.Fatal("local share contacted provider")
	}
	if err = logoutCredentialForTest(ctx, h.f.s, h.f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = h.w.ShareCloudHistory(ctx, h.f.actor, h.f.project, load.ID, share)
	requireAuthorityStatus(t, err, 401)
}

func TestWorkBuddyCloudHistorySharingTerminalVersionsAndDiscussionBounds(t *testing.T) {
	h, load, req := historySharingFixture(t)
	ctx := context.Background()
	for _, state := range []string{"loaded", "limited", "interrupted", "rejected"} {
		fixed := load
		fixed.State = state
		if state != "loaded" {
			fixed.ErrorCode = "workbuddy_cloud_history_" + state
		}
		if _, err := h.f.s.DB.Exec(`UPDATE suma_workbuddy_cloud_history_loads SET state=?,payload=? WHERE id=?`, state, stringify(fixed), load.ID); err != nil {
			t.Fatal(err)
		}
		preview, err := h.w.PreviewCloudHistoryShare(ctx, h.f.actor, h.f.project, load.ID, req)
		if err != nil || preview.LoadState != state || preview.Coverage != "unverified" {
			t.Fatal("terminal prefix lost accurate source", state, err)
		}
	}
	loading := load
	loading.State = "loading"
	loading.FinishedAt = ""
	loading.ErrorCode = ""
	if _, err := h.f.s.DB.Exec(`UPDATE suma_workbuddy_cloud_history_loads SET state='loading',payload=? WHERE id=?`, stringify(loading), load.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.w.PreviewCloudHistoryShare(ctx, h.f.actor, h.f.project, load.ID, req); err == nil {
		t.Fatal("loading prefix was shareable")
	}
	if _, err := h.f.s.DB.Exec(`UPDATE suma_workbuddy_cloud_history_loads SET state='loaded',payload=? WHERE id=?`, stringify(load), load.ID); err != nil {
		t.Fatal(err)
	}
	stale := req
	stale.ExpectedLoadVersion++
	if _, err := h.w.PreviewCloudHistoryShare(ctx, h.f.actor, h.f.project, load.ID, stale); err == nil {
		t.Fatal("wrong terminated version previewed")
	}
	for _, text := range []string{"  ", strings.Repeat("x", 32001), strings.Repeat("中", 8001)} {
		if _, err := h.f.s.DB.Exec(`UPDATE suma_workbuddy_cloud_history_entries SET text=? WHERE load_id=? AND sequence=1`, text, load.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := h.w.PreviewCloudHistoryShare(ctx, h.f.actor, h.f.project, load.ID, req); err == nil {
			t.Fatal("blank or over-limit selection silently truncated")
		}
	}
	if domainACount(t, h.f.s, "suma_task_conversation_messages") != 0 {
		t.Fatal("invalid preview wrote a message")
	}
}

func TestWorkBuddyCloudHistorySharingNotificationOnceAndSafeMetadata(t *testing.T) {
	h, load, req := historySharingFixture(t)
	ctx := context.Background()
	reader := cloudOtherWriter(t, h)
	subscriptionCommand(t, h.f, reader, req.TaskID, 0, "follow", newID())
	before := subscriptionNotificationCount(t, h.f)
	connection, err := h.w.Get(ctx, h.f.actor, h.f.project, h.delegation.ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.w.Disconnect(ctx, h.f.actor, h.f.project, connection.ID, WorkBuddyDisconnectRequest{ProjectID: h.f.project, ExpectedVersion: connection.Version, Reason: "No new cloud access", IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	preview, err := h.w.PreviewCloudHistoryShare(ctx, h.f.actor, h.f.project, load.ID, req)
	if err != nil {
		t.Fatal("disconnected OAuth blocked existing private archive", err)
	}
	share := historySharingRequest(req, preview.PreviewSHA256)
	message, err := h.w.ShareCloudHistory(ctx, h.f.actor, h.f.project, load.ID, share)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.w.ShareCloudHistory(ctx, h.f.actor, h.f.project, load.ID, share); err != nil {
		t.Fatal(err)
	}
	if subscriptionNotificationCount(t, h.f) != before+1 || domainACount(t, h.f.s, "suma_task_subscription_messages") != 1 || domainACount(t, h.f.s, "suma_workbuddy_cloud_history_shares") != 1 {
		t.Fatal("recovery duplicated subscribed delivery")
	}
	if _, err = h.f.s.DB.Exec(`UPDATE suma_task_conversation_messages SET body='tampered shared text' WHERE id=?`, message.ID); err != nil {
		t.Fatal(err)
	}
	_, err = h.f.s.ListTaskConversation(ctx, reader, h.f.project, req.TaskID, 20, 0)
	requireAuthorityStatus(t, err, 409)
}

func TestWorkBuddyCloudHistorySharingSchemaRejectsUnknownShapeBeforeWrites(t *testing.T) {
	db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "unknown-share.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE suma_workbuddy_cloud_history_shares(id TEXT PRIMARY KEY,user_note TEXT);INSERT INTO suma_workbuddy_cloud_history_shares VALUES('original','preserve')`); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: db, Dialect: "sqlite"}
	if err = s.InitializeWorkBuddyCloudHistorySharing(context.Background()); !errors.Is(err, store.ErrInitialSchema) {
		t.Fatal("unknown share schema accepted", err)
	}
	var count int
	var note string
	if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name<>'suma_workbuddy_cloud_history_shares'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("schema rejection wrote new baseline", err)
	}
	if err = db.QueryRow(`SELECT user_note FROM suma_workbuddy_cloud_history_shares WHERE id='original'`).Scan(&note); err != nil || note != "preserve" {
		t.Fatal("unknown schema original data changed", err)
	}
}

func TestWorkBuddyCloudHistorySharingCannotSelectBeyondCommittedPrefix(t *testing.T) {
	h, load, req := historySharingFixture(t)
	if _, err := h.f.s.DB.Exec(`INSERT INTO suma_workbuddy_cloud_history_entries(tenant_id,project_id,load_id,sequence,kind,text)VALUES(?,?,?,?,?,?)`, h.f.user.TenantID, h.f.project, load.ID, 500, "assistant_text", "not in committed load prefix"); err != nil {
		t.Fatal(err)
	}
	req.Sequences = []int{500}
	if _, err := h.w.PreviewCloudHistoryShare(context.Background(), h.f.actor, h.f.project, load.ID, req); err == nil {
		t.Fatal("row beyond immutable load entry count was shareable")
	}
}
