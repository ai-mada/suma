package platform

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestNotificationPreferencesActual is an opt-in local HTTP/SQLite host, not a
// normal regression case. Seeding uses existing public handlers; only the later
// browser setting changes and network requests are actual interaction evidence.
func TestNotificationPreferencesActual(t *testing.T) {
	if os.Getenv("SUMA_NOTIFICATION_PREFS_ACTUAL") != "1" {
		t.Skip("actual167 requires the explicit final source and runtime gate")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal("actual167 working directory unavailable")
	}
	for {
		if _, err = os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatal("actual167 repository root unavailable")
		}
		root = next
	}
	directory := filepath.Join(root, ".cache", "notification-preferences-167", "actual")
	requested, err := filepath.Abs(os.Getenv("SUMA_NOTIFICATION_PREFS_ACTUAL_DIR"))
	if err != nil || filepath.Clean(requested) != filepath.Clean(directory) {
		t.Fatal("actual167 requires its exact owned cache directory")
	}
	temporary, err := filepath.Abs(os.TempDir())
	if err != nil || !strings.HasPrefix(temporary, directory+string(filepath.Separator)) {
		t.Fatal("actual167 process-local TEMP/TMP must stay in its owned cache")
	}
	notificationPreferences167Gate(t, root, directory)
	private := filepath.Join(directory, "private")
	if err = os.MkdirAll(private, 0700); err != nil {
		t.Fatal("actual167 private directory unavailable")
	}
	for _, file := range []string{filepath.Join(private, "descriptor.json"), filepath.Join(private, "snapshot.sqlite"), filepath.Join(directory, "host.stop")} {
		if _, err = os.Stat(file); !os.IsNotExist(err) {
			t.Fatal("actual167 previous state exists; reconcile instead of reseeding")
		}
	}
	var outbound, rejectedWrites atomic.Int64
	originalTransport := http.DefaultTransport
	http.DefaultTransport = notificationPreferences167Transport(func(*http.Request) (*http.Response, error) {
		outbound.Add(1)
		return nil, fmt.Errorf("actual167 external HTTP is disabled")
	})
	t.Cleanup(func() { http.DefaultTransport = originalTransport })

	f := newFixture(t)
	ownerCookie := *f.cookie
	invitation := governanceInvite(f, "notification167-viewer@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	accepted := governanceAccept(f, invitation, "notification167-viewer@example.test")
	viewerCookie := *f.cookie
	f.call("GET", "/auth/session", nil, 200)
	viewer := f.view
	if viewer.ID == f.user.ID || viewer.TenantID != f.user.TenantID {
		t.Fatal("actual167 distinct current browser identities required")
	}
	viewerGrant := false
	for _, raw := range accepted["projects"].([]any) {
		project, ok := raw.(map[string]any)
		if ok && project["id"] == f.project && project["role"] == "viewer" {
			viewerGrant = true
		}
	}
	if !viewerGrant {
		t.Fatal("actual167 normal viewer invitation grant unavailable")
	}
	viewerActor, err := f.s.AuthenticateActor(context.Background(), viewerCookie.Value)
	if err != nil {
		t.Fatal("actual167 current viewer credential unavailable")
	}
	f.cookie = &ownerCookie
	f.call("GET", "/auth/session", nil, 200)
	owner := f.view
	ownerResource := f.call("POST", "/members", f.body(map[string]any{"title": "Actual167 owner", "kind": "human", "data": map[string]any{"user_id": owner.ID}}), 201)
	viewerResource := f.call("POST", "/members", f.body(map[string]any{"title": "Actual167 viewer", "kind": "human", "data": map[string]any{"user_id": viewer.ID}}), 201)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Notification preferences controlled discussion", "owner_id": ownerResource["id"]}), 201)
	taskID, ok := task["id"].(string)
	if !ok || task["version"] != float64(1) {
		t.Fatal("actual167 original task v1 unavailable")
	}
	f.cookie = &viewerCookie
	subscriptionHTTP(t, f, viewerActor, "", taskID, map[string]any{"project_id": f.project, "task_id": taskID, "expected_version": 0, "action": "follow", "idempotency_key": newID()}, 200)
	f.cookie = &ownerCookie
	f.call("GET", "/auth/session", nil, 200)

	stats := func() map[string]any {
		out := map[string]any{"outbound_http_attempts": outbound.Load(), "rejected_runtime_writes": rejectedWrites.Load()}
		for name, table := range map[string]string{"preferences": "suma_project_notification_preferences", "subscriptions": "suma_task_subscriptions", "messages": "suma_task_conversation_messages", "mentions": "suma_task_conversation_mentions", "notifications": "suma_notification_inbox", "receipts": "suma_operations", "activities": "suma_activity", "runs": "dag_runs", "nodes": "dag_nodes", "attempts": "dag_attempts", "events": "dag_events"} {
			var count int
			if queryErr := f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); queryErr != nil {
				out["database_stats_unavailable"] = true
			} else {
				out[name] = count
			}
		}
		for name, query := range map[string]string{
			"viewer_follow_notices":   "SELECT COUNT(*) FROM suma_notification_inbox WHERE tenant_id=? AND project_id=? AND task_id=? AND recipient_user_id=? AND kind='task_followed'",
			"viewer_mention_notices":  "SELECT COUNT(*) FROM suma_notification_inbox WHERE tenant_id=? AND project_id=? AND task_id=? AND recipient_user_id=? AND kind='task_mentioned'",
			"viewer_unread_notices":   "SELECT COUNT(*) FROM suma_notification_inbox WHERE tenant_id=? AND project_id=? AND task_id=? AND recipient_user_id=? AND read_at='' AND archived_at=''",
			"viewer_read_notices":     "SELECT COUNT(*) FROM suma_notification_inbox WHERE tenant_id=? AND project_id=? AND task_id=? AND recipient_user_id=? AND read_at<>''",
			"viewer_archived_notices": "SELECT COUNT(*) FROM suma_notification_inbox WHERE tenant_id=? AND project_id=? AND task_id=? AND recipient_user_id=? AND archived_at<>''",
			"viewer_follow_links":     "SELECT COUNT(*) FROM suma_task_subscription_messages WHERE tenant_id=? AND project_id=? AND task_id=? AND recipient_user_id=?",
			"viewer_mention_links":    "SELECT COUNT(*) FROM suma_task_conversation_mentions WHERE tenant_id=? AND project_id=? AND task_id=? AND recipient_user_id=?",
		} {
			var count int
			if queryErr := f.s.DB.QueryRow(query, owner.TenantID, f.project, taskID, viewer.ID).Scan(&count); queryErr != nil {
				out["target_stats_unavailable"] = true
			} else {
				out[name] = count
			}
		}
		var taskVersion, following, subscriptionVersion int
		if queryErr := f.s.DB.QueryRow("SELECT version FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?", owner.TenantID, f.project, taskID).Scan(&taskVersion); queryErr != nil {
			out["task_stats_unavailable"] = true
		} else {
			out["task_version"] = taskVersion
		}
		if queryErr := f.s.DB.QueryRow("SELECT following,version FROM suma_task_subscriptions WHERE tenant_id=? AND project_id=? AND task_id=? AND user_id=?", owner.TenantID, f.project, taskID, viewer.ID).Scan(&following, &subscriptionVersion); queryErr != nil {
			out["subscription_stats_unavailable"] = true
		} else {
			out["viewer_following"], out["viewer_subscription_version"] = following == 1, subscriptionVersion
		}
		var version, muted int
		var updated string
		if queryErr := f.s.DB.QueryRow("SELECT version,followed_discussions_muted,updated_at FROM suma_project_notification_preferences WHERE tenant_id=? AND project_id=? AND user_id=?", owner.TenantID, f.project, viewer.ID).Scan(&version, &muted, &updated); queryErr != nil && queryErr != sql.ErrNoRows {
			out["preferences_stats_unavailable"] = true
		} else {
			out["viewer_preferences"] = map[string]any{"version": version, "followed_discussions_muted": muted == 1, "updated_at": updated}
		}
		return out
	}
	initial := stats()
	for _, key := range []string{"runs", "nodes", "attempts", "events", "messages", "mentions", "preferences", "outbound_http_attempts"} {
		if fmt.Sprint(initial[key]) != "0" {
			t.Fatalf("actual167 seed expected zero %s", key)
		}
	}
	uiOrigin := "http://127.0.0.1:44017"
	platform := (&Server{Service: f.s, Engine: f.e, WebDir: filepath.Join(directory, "absent-web"), PublicURL: uiOrigin, AllowedOrigins: []string{uiOrigin}}).Handler()
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__actual167/status" {
			cookie, cookieErr := r.Cookie(ownerCookie.Name)
			if r.Method != http.MethodGet || r.URL.RawQuery != "" || cookieErr != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(ownerCookie.Value)) != 1 && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(viewerCookie.Value)) != 1 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(stats())
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && !(r.Method == http.MethodPost && (r.URL.Path == "/api/v1/notification-preferences/commands" || r.URL.Path == "/api/v1/task-conversations")) {
			rejectedWrites.Add(1)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		platform.ServeHTTP(w, r)
	}))
	defer host.Close()
	var sequence int
	var databaseName, databasePath string
	if err = f.s.DB.QueryRow("PRAGMA database_list").Scan(&sequence, &databaseName, &databasePath); err != nil {
		t.Fatal("actual167 original SQLite path unavailable")
	}
	notificationPreferences167Write(t, filepath.Join(private, "descriptor.json"), map[string]any{
		"stage": "T-NOTIFICATION-PREFERENCES-167", "origin": host.URL, "ui_origin": uiOrigin, "pid": os.Getpid(), "project_id": f.project, "task_id": taskID, "task_version": 1,
		"viewer_resource_id": viewerResource["id"], "owner_resource_id": ownerResource["id"],
		"actors":        map[string]any{"owner": map[string]any{"user_id": owner.ID, "organization_id": owner.TenantID, "project_role": "owner", "cookie": map[string]any{"name": ownerCookie.Name, "value": ownerCookie.Value}}, "viewer": map[string]any{"user_id": viewer.ID, "organization_id": viewer.TenantID, "project_role": "viewer", "cookie": map[string]any{"name": viewerCookie.Name, "value": viewerCookie.Value}}},
		"initial_stats": initial, "database_source": databasePath, "data_directory_source": f.s.DataDir,
		"seed": map[string]any{"method": "newFixture and normal public handler setup/invite/accept/member/task/subscription calls; not GUI", "fake_accounts": 2, "viewer_role": "viewer", "task_version": 1, "viewer_subscription_version": 1, "cloud_provider_model_runner": "none configured or started"},
	})
	t.Log("owned167 real HTTP/SQLite host ready; two controlled browser credentials remain private")
	deadline := time.NewTimer(20 * time.Minute)
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
		t.Fatal("actual167 existing private snapshot retained")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err = f.s.DB.ExecContext(ctx, "VACUUM INTO ?", snapshot); err != nil {
		t.Fatal("actual167 consistent private snapshot export failed")
	}
	if err = os.Chmod(snapshot, 0600); err != nil {
		t.Fatal("actual167 private snapshot permissions unavailable")
	}
	data, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal("actual167 private snapshot hash unavailable")
	}
	digest := sha256.Sum256(data)
	notificationPreferences167Write(t, filepath.Join(private, "host-final.json"), map[string]any{"stats": final, "host_closed": true, "host_timeout": timedOut, "snapshot": map[string]any{"path": snapshot, "bytes": len(data), "sha256": hex.EncodeToString(digest[:]), "method": "Parameterized VACUUM INTO absent private target after HTTP close before t.TempDir cleanup"}, "original_database_source": databasePath, "original_data_directory_source": f.s.DataDir, "original_sources_cleanup_expected": true})
	if timedOut {
		t.Fatal("actual167 host deadline reached; private snapshot preserved")
	}
}

type notificationPreferences167Transport func(*http.Request) (*http.Response, error)

func (f notificationPreferences167Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func notificationPreferences167Gate(t *testing.T, root, directory string) {
	t.Helper()
	var gate struct {
		Stage             string `json:"stage"`
		RuntimeAuthorized bool   `json:"runtime_authorized"`
		Files             []struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	data, err := os.ReadFile(filepath.Join(directory, "run-authorized.json"))
	if err != nil || json.Unmarshal(data, &gate) != nil || gate.Stage != "T-NOTIFICATION-PREFERENCES-167" || !gate.RuntimeAuthorized || len(gate.Files) < 1 || len(gate.Files) > 100 {
		t.Fatal("actual167 exact final source/runtime gate required")
	}
	for _, input := range gate.Files {
		absolute := filepath.Join(root, filepath.FromSlash(input.Path))
		if input.Path == "" || filepath.IsAbs(input.Path) || !strings.HasPrefix(absolute, root+string(filepath.Separator)) {
			t.Fatal("actual167 source gate must stay in repository")
		}
		bytes, readErr := os.ReadFile(absolute)
		digest := sha256.Sum256(bytes)
		if readErr != nil || len(input.SHA256) != 64 || hex.EncodeToString(digest[:]) != input.SHA256 {
			t.Fatal("actual167 source gate changed before seed")
		}
	}
}

func notificationPreferences167Write(t *testing.T, file string, value any) {
	t.Helper()
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("actual167 original private record retained or cannot be created")
	}
	if err = json.NewEncoder(f).Encode(value); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatal("actual167 private record persistence unavailable")
	}
}
