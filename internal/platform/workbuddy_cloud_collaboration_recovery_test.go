package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

type cloudHistoryRecoveryResult struct {
	load wc.CloudHistoryLoad
	err  error
}

func cloudHistoryRecoveryStatus(t *testing.T, err error, want int) {
	t.Helper()
	var api *APIError
	if !errors.As(err, &api) || api.Status != want {
		t.Fatalf("wanted status %d, received %v", want, err)
	}
}

func cloudHistoryRecoveryWait(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("original load did not reach its unlocked external request")
	}
}

func TestWorkBuddyCloudCollaborationRecoveryActiveLoadReplaysAndRejectsParallelNode(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	h, sub, posts := cloudHistoryFixture(t, []string{cloudHistoryText("agent_message_chunk", "original")}, func() {
		enteredOnce.Do(func() { close(entered) })
		<-release
	})
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req := cloudHistoryRequest(h, sub)
	result := make(chan cloudHistoryRecoveryResult, 1)
	go func() {
		load, err := h.w.LoadCloudHistory(ctx, h.f.actor, h.f.project, sub.ID, req)
		result <- cloudHistoryRecoveryResult{load: load, err: err}
	}()
	cloudHistoryRecoveryWait(t, entered)
	view, err := h.w.CloudCollaboration(ctx, h.f.actor, h.f.project, sub.ID)
	if err != nil || view.Version != 1 || view.LatestLoad == nil || view.LatestLoad.State != "loading" || view.LatestLoad.Overdue {
		t.Fatal("active load was not persisted before external I/O", err)
	}
	replay, err := h.w.LoadCloudHistory(ctx, h.f.actor, h.f.project, sub.ID, req)
	if err != nil || replay.ID != view.LatestLoad.ID || replay.State != "loading" || replay.OperationID != req.IdempotencyKey || replay.HeadVersionAtStart != req.ExpectedVersion || posts.Load() != 2 {
		t.Fatal("active receipt replay performed another attachment", err, posts.Load())
	}
	parallel := req
	parallel.IdempotencyKey, parallel.ExpectedVersion = newID(), view.Version
	_, err = h.w.LoadCloudHistory(ctx, h.f.actor, h.f.project, sub.ID, parallel)
	cloudHistoryRecoveryStatus(t, err, 409)
	if posts.Load() != 2 {
		t.Fatal("parallel node request reached the provider")
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case got := <-result:
		if got.err != nil || got.load.State != "loaded" || got.load.ID != replay.ID {
			t.Fatal("original load did not finish under its reserved identity", got.err)
		}
	case <-ctx.Done():
		t.Fatal("original load did not finish")
	}
}

func TestWorkBuddyCloudCollaborationRecoveryOverdueNewAttemptFencesOriginalCallbacks(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	h, sub, posts := cloudHistoryFixture(t, []string{cloudHistoryText("agent_message_chunk", "stale callback")}, func() {
		close(entered)
		<-release
	})
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req := cloudHistoryRequest(h, sub)
	result := make(chan cloudHistoryRecoveryResult, 1)
	go func() {
		load, err := h.w.LoadCloudHistory(ctx, h.f.actor, h.f.project, sub.ID, req)
		result <- cloudHistoryRecoveryResult{load: load, err: err}
	}()
	cloudHistoryRecoveryWait(t, entered)
	view, err := h.w.CloudCollaboration(ctx, h.f.actor, h.f.project, sub.ID)
	if err != nil || view.LatestLoad == nil {
		t.Fatal("missing prepared load", err)
	}
	old := *view.LatestLoad
	started, err := time.Parse(time.RFC3339Nano, old.StartedAt)
	if err != nil {
		t.Fatal(err)
	}
	old.StartedAt = started.Add(-31 * time.Second).Format(time.RFC3339Nano)
	old.DeadlineAt = started.Add(-time.Second).Format(time.RFC3339Nano)
	if _, err = h.f.s.DB.Exec(`UPDATE suma_workbuddy_cloud_history_loads SET deadline_at=?,payload=? WHERE id=?`, old.DeadlineAt, stringify(old), old.ID); err != nil {
		t.Fatal(err)
	}
	view, err = h.w.CloudCollaboration(ctx, h.f.actor, h.f.project, sub.ID)
	if err != nil || view.LatestLoad == nil || !view.LatestLoad.Overdue || view.LatestLoad.State != "loading" || view.Version != 1 || view.LatestLoad.Version != old.Version {
		t.Fatal("database deadline did not produce a nonmutating overdue view", err)
	}
	// Keep the original connection alive, but fail the new task GET. The new
	// command must commit its fence without creating a second stream.
	h.w.app.HTTPClient = &http.Client{Transport: cloudHistoryRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("controlled unavailable task observation")
	})}
	newRequest := req
	newRequest.ExpectedVersion, newRequest.IdempotencyKey = view.Version, newID()
	latest, err := h.w.LoadCloudHistory(ctx, h.f.actor, h.f.project, sub.ID, newRequest)
	if err != nil || latest.ID == old.ID || latest.State != "interrupted" || latest.OperationID != newRequest.IdempotencyKey || latest.HeadVersionAtStart != 1 || posts.Load() != 2 {
		t.Fatal("explicit attempt did not replace expired collection safely", err, posts.Load())
	}
	_, err = h.w.cloudHistoryFinish(ctx, h.f.actor, h.f.project, old.ID, old.Source, "loaded", "")
	cloudHistoryRecoveryStatus(t, err, 409)
	err = h.w.cloudHistoryAppend(ctx, h.f.actor, h.f.project, old.ID, old.Source, wc.CloudHistoryEntry{Kind: "assistant_text", Text: "forbidden stale append"})
	cloudHistoryRecoveryStatus(t, err, 409)
	releaseOnce.Do(func() { close(release) })
	select {
	case got := <-result:
		if got.err == nil {
			t.Fatal("original connection published after its fence")
		}
	case <-ctx.Done():
		t.Fatal("fenced original connection did not close")
	}
	var entries int
	if err = h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_workbuddy_cloud_history_entries WHERE load_id=?`, old.ID).Scan(&entries); err != nil || entries != 0 {
		t.Fatal("old callback stored entries after replacement", entries, err)
	}
	view, err = h.w.CloudCollaboration(ctx, h.f.actor, h.f.project, sub.ID)
	if err != nil || view.LatestLoad == nil || view.LatestLoad.ID != latest.ID || view.LastLoaded != nil {
		t.Fatal("old completion replaced the newer head", err)
	}
}

func TestWorkBuddyCloudCollaborationRecoveryArchiveRequiresBrowserAndSurvivesDisconnectedGrant(t *testing.T) {
	h, sub, posts := cloudHistoryFixture(t, []string{cloudHistoryText("agent_message_chunk", "stored owner observation")}, nil)
	ctx := context.Background()
	load, err := h.w.LoadCloudHistory(ctx, h.f.actor, h.f.project, sub.ID, cloudHistoryRequest(h, sub))
	if err != nil {
		t.Fatal(err)
	}
	credential, err := h.f.s.IssueProjectCredential(ctx, h.f.actor, ProjectCredentialIssueRequest{ProjectID: h.f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	api := domainCActor(t, h.f.s, credential.Token)
	_, err = h.w.CloudCollaboration(ctx, api, h.f.project, sub.ID)
	cloudHistoryRecoveryStatus(t, err, 403)
	_, err = h.w.CloudHistory(ctx, api, h.f.project, load.ID, 0, 50)
	cloudHistoryRecoveryStatus(t, err, 403)
	if _, err = h.f.s.DB.Exec(`UPDATE suma_workbuddy_connections SET state='disconnected' WHERE id=?`, h.delegation.ConnectionID); err != nil {
		t.Fatal(err)
	}
	view, err := h.w.CloudCollaboration(ctx, h.f.actor, h.f.project, sub.ID)
	if err != nil || view.LastLoaded == nil || view.LastLoaded.ID != load.ID || len(view.AllowedActions) != 0 {
		t.Fatal("grant disconnect erased local read authority or preserved external action", err)
	}
	page, err := h.w.CloudHistory(ctx, h.f.actor, h.f.project, load.ID, 0, 50)
	if err != nil || len(page.Items) != 1 || page.Items[0].Text != "stored owner observation" || posts.Load() != 2 {
		t.Fatal("archive read used OAuth or contacted provider", err, posts.Load())
	}
}

func TestWorkBuddyCloudCollaborationRecoveryArchiveRejectsChangedTaskAndNodeSpec(t *testing.T) {
	for _, change := range []string{"task", "node_spec"} {
		t.Run(change, func(t *testing.T) {
			h, sub, posts := cloudHistoryFixture(t, []string{cloudHistoryText("agent_message_chunk", "original source only")}, nil)
			ctx := context.Background()
			load, err := h.w.LoadCloudHistory(ctx, h.f.actor, h.f.project, sub.ID, cloudHistoryRequest(h, sub))
			if err != nil {
				t.Fatal(err)
			}
			if change == "task" {
				sub.Task.ID = "different_task"
				if _, err = h.f.s.DB.Exec(`UPDATE suma_workbuddy_cloud_submissions SET payload=? WHERE id=?`, stringify(sub), sub.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				var raw string
				if err = h.f.s.DB.QueryRow(`SELECT spec FROM dag_nodes WHERE run_id=? AND id=?`, sub.RunID, sub.NodeID).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				var node map[string]any
				if err = json.Unmarshal([]byte(raw), &node); err != nil {
					t.Fatal(err)
				}
				node["config"].(map[string]any)["changed_source"] = true
				if _, err = h.f.s.DB.Exec(`UPDATE dag_nodes SET spec=? WHERE run_id=? AND id=?`, stringify(node), sub.RunID, sub.NodeID); err != nil {
					t.Fatal(err)
				}
			}
			_, err = h.w.CloudCollaboration(ctx, h.f.actor, h.f.project, sub.ID)
			cloudHistoryRecoveryStatus(t, err, 409)
			_, err = h.w.CloudHistory(ctx, h.f.actor, h.f.project, load.ID, 0, 50)
			cloudHistoryRecoveryStatus(t, err, 409)
			if posts.Load() != 2 {
				t.Fatal("invalidated archive attempted a provider connection")
			}
		})
	}
}
