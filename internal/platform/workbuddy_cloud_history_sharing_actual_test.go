package platform

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Opt-in actual165 host. Normal test runs do not seed data or start a server.
// The seed is a new controlled SDK/SSE fixture, never the prior164 private data.
func TestWorkBuddyCloudHistorySharingActualHTTPFixture(t *testing.T) {
	if os.Getenv("SUMA_HISTORY_SHARE_ACTUAL") != "1" {
		t.Skip("owned actual165 host requires explicit final freeze and runtime gate")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal("actual165 working directory unavailable")
	}
	for {
		if _, err = os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatal("actual165 repository root unavailable")
		}
		root = next
	}
	directory := filepath.Join(root, ".cache", "workbuddy-history-share-165", "actual")
	requested, err := filepath.Abs(os.Getenv("SUMA_HISTORY_SHARE_ACTUAL_DIR"))
	if err != nil || filepath.Clean(requested) != filepath.Clean(directory) {
		t.Fatal("actual165 requires its exact owned cache directory")
	}
	if _, err = os.Stat(filepath.Join(directory, "run-authorized.json")); err != nil {
		t.Fatal("actual165 final source gate required")
	}
	private := filepath.Join(directory, "private")
	if err = os.MkdirAll(private, 0700); err != nil {
		t.Fatal("actual165 private directory unavailable")
	}
	if _, err = os.Stat(filepath.Join(private, "descriptor.json")); !os.IsNotExist(err) {
		t.Fatal("actual165 original descriptor exists; reconcile instead of reseeding")
	}

	// The existing165 helper creates one controlled known task and invokes the
	// real SDK load once. GUI sharing traffic is measured only after this seed.
	h, load, preview := historySharingFixture(t)
	if load.State != "loaded" || load.Coverage != "unverified" || load.EntryCount != 3 || load.UnsupportedCount != 1 {
		t.Fatal("actual165 terminal controlled history seed unavailable")
	}
	var outgoing, taskAccess, acpGet, acpPost atomic.Int32
	client := *h.w.app.HTTPClient
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.Transport = cloudHistoryRoundTripper(func(r *http.Request) (*http.Response, error) {
		outgoing.Add(1)
		if r.Method == http.MethodGet && r.URL.Path == "/openapi/v2/tasks/cloud-task" {
			taskAccess.Add(1)
		}
		if r.URL.Path == "/sessions/cloud-task" {
			if r.Method == http.MethodGet {
				acpGet.Add(1)
			} else if r.Method == http.MethodPost {
				acpPost.Add(1)
			}
		}
		return base.RoundTrip(r)
	})
	h.w.app.HTTPClient = &client
	a, result := cloudResultFixture(t, h)
	result.WorkBuddyCloudWait.SubmissionID = load.Source.SubmissionID
	result.WorkBuddyCloudWait.SubmissionVersion = load.Source.SubmissionVersion
	if err = completeCloudFixture(t, h, a, result); err != nil {
		t.Fatal("actual165 original trusted waiting projection unavailable")
	}
	initialRun, err := h.f.e.GetRun(context.Background(), h.f.user.TenantID, h.f.project, load.Source.RunID)
	if err != nil || len(initialRun.Nodes) != 1 || initialRun.Nodes[0].State != "waiting" {
		t.Fatal("actual165 original waiting run unavailable")
	}

	// Use the normal invitation and acceptance handlers; no actor, membership,
	// subscription or notification row is manufactured with SQL.
	ownerCookie := *h.f.cookie
	invitation := governanceInvite(h.f, "history-share165-reader@example.test", "member", []any{map[string]any{"project_id": h.f.project, "role": "viewer"}})
	accepted := governanceAccept(h.f, invitation, "history-share165-reader@example.test")
	readerCookie := *h.f.cookie
	h.f.call("GET", "/auth/session", nil, 200)
	readerView := h.f.view
	if readerView.ID == h.f.user.ID || readerView.TenantID != h.f.user.TenantID {
		t.Fatal("actual165 second current browser identity unavailable")
	}
	projectViewer := false
	for _, raw := range accepted["projects"].([]any) {
		project, ok := raw.(map[string]any)
		if ok && project["id"] == h.f.project && project["role"] == "viewer" {
			projectViewer = true
		}
	}
	if !projectViewer {
		t.Fatal("actual165 normal project viewer grant unavailable")
	}
	readerActor, authErr := h.f.s.AuthenticateActor(context.Background(), readerCookie.Value)
	if authErr != nil {
		t.Fatal("actual165 current reader browser credential unavailable")
	}
	subscriptionHTTP(t, h.f, readerActor, "", preview.TaskID, map[string]any{"project_id": h.f.project, "task_id": preview.TaskID, "expected_version": 0, "action": "follow", "idempotency_key": newID()}, 200)
	h.f.cookie = &ownerCookie
	h.f.call("GET", "/auth/session", nil, 200)
	ownerView := h.f.view
	stats := func() map[string]any {
		out := map[string]any{"provider_creates_total": h.creates.Load(), "post_seed_external_requests": outgoing.Load(), "post_seed_session_access": taskAccess.Load(), "post_seed_acp_get": acpGet.Load(), "post_seed_acp_post": acpPost.Load()}
		for name, table := range map[string]string{"conversation_heads": "suma_task_conversation_heads", "messages": "suma_task_conversation_messages", "shares": "suma_workbuddy_cloud_history_shares", "notifications": "suma_notification_inbox", "receipts": "suma_operations", "activities": "suma_activity"} {
			var count int
			if queryErr := h.f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); queryErr != nil {
				out["database_stats_unavailable"] = true
			} else {
				out[name] = count
			}
		}
		for name, query := range map[string]string{
			"target_messages":        "SELECT COUNT(*) FROM suma_task_conversation_messages WHERE tenant_id=? AND project_id=? AND task_id=?",
			"target_shares":          "SELECT COUNT(*) FROM suma_workbuddy_cloud_history_shares WHERE tenant_id=? AND project_id=? AND task_id=?",
			"reader_follow_notices":  "SELECT COUNT(*) FROM suma_notification_inbox WHERE tenant_id=? AND project_id=? AND task_id=? AND kind='task_followed' AND recipient_user_id=?",
			"reader_unread_notices":  "SELECT COUNT(*) FROM suma_notification_inbox WHERE tenant_id=? AND project_id=? AND task_id=? AND kind='task_followed' AND recipient_user_id=? AND read_at=''",
			"reader_follower_links":  "SELECT COUNT(*) FROM suma_task_subscription_messages WHERE tenant_id=? AND project_id=? AND task_id=? AND recipient_user_id=?",
		} {
			args := []any{h.f.user.TenantID, h.f.project, preview.TaskID}
			if name != "target_messages" && name != "target_shares" {
				args = append(args, readerView.ID)
			}
			var count int
			if queryErr := h.f.s.DB.QueryRow(query, args...).Scan(&count); queryErr != nil {
				out["target_stats_unavailable"] = true
			} else {
				out[name] = count
			}
		}
		var followed int
		if queryErr := h.f.s.DB.QueryRow("SELECT COUNT(*) FROM suma_notification_inbox WHERE kind='task_followed'").Scan(&followed); queryErr == nil {
			out["follow_notifications"] = followed
		} else {
			out["notification_stats_unavailable"] = true
		}
		var taskVersion int
		if queryErr := h.f.s.DB.QueryRow("SELECT version FROM suma_tasks WHERE id=?", preview.TaskID).Scan(&taskVersion); queryErr == nil {
			out["target_task_version"] = taskVersion
		}
		if run, runErr := h.f.e.GetRun(context.Background(), h.f.user.TenantID, h.f.project, load.Source.RunID); runErr == nil && len(run.Nodes) == 1 {
			out["run_version"], out["run_state"] = run.Version, run.State
			out["node_state"], out["node_reason"] = run.Nodes[0].State, run.Nodes[0].Reason
			out["attempt"], out["fence"] = run.Nodes[0].Attempt, run.Nodes[0].Fence
		} else {
			out["run_stats_unavailable"] = true
		}
		return out
	}
	uiOrigin := "http://127.0.0.1:43993"
	platform := (&Server{Service: h.f.s, Engine: h.f.e, WorkBuddy: h.w, WebDir: filepath.Join(root, "apps", "web"), PublicURL: uiOrigin, AllowedOrigins: []string{uiOrigin}}).Handler()
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/__actual165/status" {
			platform.ServeHTTP(w, r)
			return
		}
		cookie, cookieErr := r.Cookie(ownerCookie.Name)
		if r.Method != http.MethodGet || cookieErr != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(ownerCookie.Value)) != 1 && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(readerCookie.Value)) != 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(stats())
	}))
	defer host.Close()
	var sequence int
	var databaseName, databasePath string
	if err = h.f.s.DB.QueryRow("PRAGMA database_list").Scan(&sequence, &databaseName, &databasePath); err != nil {
		t.Fatal("actual165 original database location unavailable")
	}
	historySharing165PrivateWrite(t, filepath.Join(private, "descriptor.json"), map[string]any{"origin": host.URL, "ui_origin": uiOrigin, "pid": os.Getpid(), "project_id": h.f.project, "source_run_id": load.Source.RunID, "source_node_id": load.Source.NodeID, "source_submission_id": load.Source.SubmissionID, "load_id": load.ID, "load_version": load.Version, "load_state": load.State, "coverage": load.Coverage, "target_task_id": preview.TaskID, "target_task_version": preview.ExpectedTaskVersion, "supported_sequences": []int{1, 2}, "unsupported_sequence": 3, "actors": map[string]any{"owner": map[string]any{"user_id": ownerView.ID, "organization_id": ownerView.TenantID, "cookie": map[string]any{"name": ownerCookie.Name, "value": ownerCookie.Value}}, "reader": map[string]any{"user_id": readerView.ID, "organization_id": readerView.TenantID, "project_role": "viewer", "cookie": map[string]any{"name": readerCookie.Name, "value": readerCookie.Value}}}, "initial_stats": stats(), "seed": map[string]any{"fixture": "historySharingFixture reuses controlled SDK/SSE and creates a terminal private load before GUI", "controlled_provider_task_created": 1, "sdk_history_load_invoked": 1, "sdk_counter_not_retained_by_seed_helper": true, "post_seed_counter_scope": "All subsequent provider requests, including App/preview/share/recovery; expected zero"}, "database_source": databasePath, "data_directory_source": h.f.s.DataDir})
	historySharing165PrivateWrite(t, filepath.Join(private, "restore-metadata.json"), map[string]any{"encryption_key_base64": base64.StdEncoding.EncodeToString(h.w.key), "client_id": h.w.app.ClientID, "client_secret": h.w.app.ClientSecret, "app_digest": h.w.appDigest, "base_url": h.w.app.BaseURL, "redirect_uri": h.w.app.RedirectURI, "fixture_only": true})
	t.Log("owned165 HTTP fixture ready; two browser credentials remain private")
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	timedOut := false
	waiting := true
	for waiting {
		select {
		case <-deadline.C:
			timedOut, waiting = true, false
		case <-ticker.C:
			if _, stopErr := os.Stat(filepath.Join(directory, "host.stop")); stopErr == nil {
				waiting = false
			}
		}
	}
	host.Close()
	final := stats()
	snapshot := filepath.Join(private, "snapshot.sqlite")
	if _, err = os.Stat(snapshot); !os.IsNotExist(err) {
		t.Fatal("actual165 original snapshot exists; replacement refused")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err = h.f.s.DB.ExecContext(ctx, "VACUUM INTO ?", snapshot); err != nil {
		t.Fatal("actual165 consistent private snapshot export failed")
	}
	if err = os.Chmod(snapshot, 0600); err != nil {
		t.Fatal("actual165 private snapshot permissions unavailable")
	}
	data, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal("actual165 private snapshot hash unavailable")
	}
	digest := sha256.Sum256(data)
	historySharing165PrivateWrite(t, filepath.Join(private, "host-final.json"), map[string]any{"stats": final, "host_closed": true, "host_timeout": timedOut, "snapshot": map[string]any{"path": snapshot, "bytes": len(data), "sha256": hex.EncodeToString(digest[:]), "method": "Parameterized VACUUM INTO absent private target after HTTP close before helper cleanup"}, "original_database_source": databasePath, "original_data_directory_source": h.f.s.DataDir, "original_sources_cleanup_expected": true})
	if timedOut {
		t.Fatal("actual165 host deadline reached; private snapshot preserved")
	}
}

func historySharing165PrivateWrite(t *testing.T, file string, value any) {
	t.Helper()
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("actual165 private record exists or cannot be created")
	}
	if err = json.NewEncoder(f).Encode(value); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatal("actual165 private record persistence unavailable")
	}
}
