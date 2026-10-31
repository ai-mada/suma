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
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/store"
)

const notificationVisual167SnapshotSHA = "0aec237e02768f6f41e50b17d47bb1fba36d0e0aa1da5546d5e5615a05814bcd"

// This opt-in visual continuation reads a copy of the already sealed actual167
// database. It has no setup, initialization, Engine, login or mutation endpoint.
func TestNotificationPreferencesVisualReadOnly(t *testing.T) {
	if os.Getenv("SUMA_NOTIFICATION_PREFS_VISUAL") != "1" {
		t.Skip("visual167 requires an independent final source/runtime gate")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal("visual167 working directory unavailable")
	}
	for {
		if _, err = os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatal("visual167 repository root unavailable")
		}
		root = next
	}
	directory := filepath.Join(root, ".cache", "notification-preferences-167", "visual")
	requested, err := filepath.Abs(os.Getenv("SUMA_NOTIFICATION_PREFS_VISUAL_DIR"))
	if err != nil || filepath.Clean(requested) != filepath.Clean(directory) {
		t.Fatal("visual167 exact owned directory required")
	}
	temporary, err := filepath.Abs(os.TempDir())
	if err != nil || !strings.HasPrefix(temporary, directory+string(filepath.Separator)) {
		t.Fatal("visual167 TEMP/TMP must stay in its owned directory")
	}
	notificationVisual167Gate(t, root, directory)
	private := filepath.Join(directory, "private")
	if err = os.MkdirAll(private, 0700); err != nil {
		t.Fatal("visual167 private directory unavailable")
	}
	for _, file := range []string{filepath.Join(private, "descriptor.json"), filepath.Join(private, "restored.sqlite"), filepath.Join(directory, "host.stop")} {
		if _, err = os.Stat(file); !os.IsNotExist(err) {
			t.Fatal("visual167 prior state retained; implicit restart refused")
		}
	}
	original := filepath.Join(root, ".cache", "notification-preferences-167", "actual", "private")
	snapshot := filepath.Join(original, "snapshot.sqlite")
	data, err := os.ReadFile(snapshot)
	if err != nil || len(data) != 2863104 || notificationVisual167SHA(data) != notificationVisual167SnapshotSHA {
		t.Fatal("visual167 original sealed snapshot mismatch")
	}
	var previous struct {
		ProjectID string `json:"project_id"`
		TaskID    string `json:"task_id"`
		Actors    map[string]struct {
			UserID         string `json:"user_id"`
			OrganizationID string `json:"organization_id"`
			Cookie         struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"cookie"`
		} `json:"actors"`
	}
	descriptorBytes, err := os.ReadFile(filepath.Join(original, "descriptor.json"))
	if err != nil || json.Unmarshal(descriptorBytes, &previous) != nil || !taskSubscriptionID(previous.ProjectID) || !taskSubscriptionID(previous.TaskID) {
		t.Fatal("visual167 preserved controlled descriptor unavailable")
	}
	viewer, ok := previous.Actors["viewer"]
	if !ok || !taskSubscriptionID(viewer.UserID) || !taskSubscriptionID(viewer.OrganizationID) || viewer.Cookie.Name != "suma_session" || viewer.Cookie.Value == "" {
		t.Fatal("visual167 preserved viewer credential unavailable")
	}
	restored := filepath.Join(private, "restored.sqlite")
	copyFile, err := os.OpenFile(restored, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("visual167 original restore copy retained")
	}
	_, writeErr := copyFile.Write(data)
	syncErr, closeErr := copyFile.Sync(), copyFile.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		t.Fatal("visual167 restore copy persistence unavailable")
	}
	copied, err := os.ReadFile(restored)
	if err != nil || notificationVisual167SHA(copied) != notificationVisual167SnapshotSHA {
		t.Fatal("visual167 restore copy digest mismatch")
	}
	db, err := store.Open("sqlite", restored)
	if err != nil {
		t.Fatal("visual167 preserved SQLite unavailable")
	}
	t.Cleanup(func() { db.Close() })
	service := &Service{DB: db, Dialect: "sqlite", DataDir: filepath.Join(private, "unused-data")}
	before := notificationVisual167Digests(t, db)
	actor, err := service.AuthenticateActor(context.Background(), viewer.Cookie.Value)
	if err != nil || actor.Projection().ID != viewer.UserID || actor.Projection().TenantID != viewer.OrganizationID {
		t.Fatal("visual167 preserved session is not currently valid; no login fallback")
	}
	var outbound, denied, authReads, preferenceReads, inboxReads, statusReads atomic.Int64
	originalTransport := http.DefaultTransport
	http.DefaultTransport = notificationPreferences167Transport(func(*http.Request) (*http.Response, error) {
		outbound.Add(1)
		return nil, fmt.Errorf("visual167 external HTTP disabled")
	})
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	stats := func() (map[string]any, error) {
		out := map[string]any{"outbound_http_attempts": outbound.Load(), "denied_requests": denied.Load(), "auth_session_gets": authReads.Load(), "preferences_gets": preferenceReads.Load(), "inbox_gets": inboxReads.Load(), "status_gets": statusReads.Load()}
		var version, muted, taskVersion, following, subscriptionVersion int
		if queryErr := db.QueryRow("SELECT version,followed_discussions_muted FROM suma_project_notification_preferences WHERE tenant_id=? AND project_id=? AND user_id=?", viewer.OrganizationID, previous.ProjectID, viewer.UserID).Scan(&version, &muted); queryErr != nil {
			return nil, queryErr
		}
		out["preferences_version"], out["followed_discussions_muted"] = version, muted == 1
		if queryErr := db.QueryRow("SELECT version FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?", viewer.OrganizationID, previous.ProjectID, previous.TaskID).Scan(&taskVersion); queryErr != nil {
			return nil, queryErr
		}
		out["task_version"] = taskVersion
		if queryErr := db.QueryRow("SELECT version,following FROM suma_task_subscriptions WHERE tenant_id=? AND project_id=? AND task_id=? AND user_id=?", viewer.OrganizationID, previous.ProjectID, previous.TaskID, viewer.UserID).Scan(&subscriptionVersion, &following); queryErr != nil {
			return nil, queryErr
		}
		out["subscription_version"], out["following"] = subscriptionVersion, following == 1
		for name, query := range map[string]string{
			"viewer_follow_notices":  "SELECT COUNT(*) FROM suma_notification_inbox WHERE tenant_id=? AND project_id=? AND recipient_user_id=? AND kind='task_followed'",
			"viewer_mention_notices": "SELECT COUNT(*) FROM suma_notification_inbox WHERE tenant_id=? AND project_id=? AND recipient_user_id=? AND kind='task_mentioned'",
			"viewer_unread_notices":  "SELECT COUNT(*) FROM suma_notification_inbox WHERE tenant_id=? AND project_id=? AND recipient_user_id=? AND read_at='' AND archived_at=''",
		} {
			var count int
			if queryErr := db.QueryRow(query, viewer.OrganizationID, previous.ProjectID, viewer.UserID).Scan(&count); queryErr != nil {
				return nil, queryErr
			}
			out[name] = count
		}
		return out, nil
	}
	initial, err := stats()
	if err != nil || initial["preferences_version"] != 2 || initial["followed_discussions_muted"] != false || initial["viewer_follow_notices"] != 2 || initial["viewer_mention_notices"] != 1 || initial["viewer_unread_notices"] != 3 || initial["task_version"] != 1 || initial["subscription_version"] != 1 || initial["following"] != true {
		t.Fatal("visual167 preserved business state mismatch")
	}
	uiOrigin := "http://127.0.0.1:44027"
	platform := (&Server{Service: service, PublicURL: uiOrigin, AllowedOrigins: []string{uiOrigin}, WebDir: filepath.Join(directory, "absent-web")}).Handler()
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			denied.Add(1)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/api/v1/auth/session":
			authReads.Add(1)
		case "/api/v1/notification-preferences":
			preferenceReads.Add(1)
		case "/api/v1/notification-inbox":
			inboxReads.Add(1)
		case "/__visual167/status":
			statusReads.Add(1)
			cookie, cookieErr := r.Cookie(viewer.Cookie.Name)
			if cookieErr != nil || r.URL.RawQuery != "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(viewer.Cookie.Value)) != 1 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			currentActor, authErr := service.AuthenticateActor(r.Context(), cookie.Value)
			if authErr != nil || currentActor.Projection().ID != viewer.UserID || currentActor.Projection().TenantID != viewer.OrganizationID {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			value, readErr := stats()
			if readErr != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(value)
			return
		default:
			denied.Add(1)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		platform.ServeHTTP(w, r)
	}))
	defer host.Close()
	notificationVisual167Write(t, filepath.Join(private, "descriptor.json"), map[string]any{"stage": "T-NOTIFICATION-PREFERENCES-167-VISUAL", "origin": host.URL, "ui_origin": uiOrigin, "pid": os.Getpid(), "project_id": previous.ProjectID, "task_id": previous.TaskID, "viewer": viewer, "source_snapshot_sha256": notificationVisual167SnapshotSHA, "source_snapshot_bytes": len(data), "restored_database": restored, "before_digests": before, "initial_stats": initial, "no_seed_or_initialize": true})
	t.Log("visual167 read-only restore host ready; original session remains private")
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	timedOut, waiting := false, true
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
	final, statsErr := stats()
	after := notificationVisual167Digests(t, db)
	originalAgain, snapshotErr := os.ReadFile(snapshot)
	originalUnchanged := snapshotErr == nil && len(originalAgain) == 2863104 && notificationVisual167SHA(originalAgain) == notificationVisual167SnapshotSHA
	unchanged := reflect.DeepEqual(before, after)
	closeErr = db.Close()
	notificationVisual167Write(t, filepath.Join(private, "host-final.json"), map[string]any{"host_closed": true, "host_timeout": timedOut, "stats": final, "stats_available": statsErr == nil, "before_digests": before, "after_digests": after, "business_tables_unchanged": unchanged, "original_snapshot_unchanged": originalUnchanged, "source_snapshot_sha256": notificationVisual167SnapshotSHA, "source_snapshot_bytes": 2863104, "restored_copy_retained": true, "restored_database_closed": closeErr == nil, "no_seed_or_initialize": true})
	if timedOut || statsErr != nil || !unchanged || !originalUnchanged || closeErr != nil || outbound.Load() != 0 || denied.Load() != 0 {
		t.Fatal("visual167 final read-only invariants failed; original evidence retained")
	}
}

type notificationVisual167TableDigest struct {
	Rows   int    `json:"rows"`
	SHA256 string `json:"sha256"`
}

func notificationVisual167Digests(t *testing.T, db *sql.DB) map[string]notificationVisual167TableDigest {
	t.Helper()
	out := map[string]notificationVisual167TableDigest{}
	for _, table := range []string{"suma_project_notification_preferences", "suma_task_subscriptions", "suma_tasks", "suma_notification_inbox", "suma_notification_inbox_sequences", "suma_task_subscription_messages", "suma_task_conversation_mentions", "suma_task_conversation_messages", "suma_task_conversation_heads", "suma_operations", "suma_activity", "dag_runs", "dag_nodes", "dag_attempts", "dag_events"} {
		rows, err := db.Query("SELECT * FROM " + table)
		if err != nil {
			t.Fatal("visual167 business digest query unavailable")
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatal("visual167 business digest columns unavailable")
		}
		encoded := []string{}
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err = rows.Scan(pointers...); err != nil {
				rows.Close()
				t.Fatal("visual167 business digest row unavailable")
			}
			item, marshalErr := json.Marshal(values)
			if marshalErr != nil {
				rows.Close()
				t.Fatal("visual167 business digest encoding unavailable")
			}
			encoded = append(encoded, string(item))
		}
		readErr, closeErr := rows.Err(), rows.Close()
		if readErr != nil || closeErr != nil {
			t.Fatal("visual167 business digest read incomplete")
		}
		sort.Strings(encoded)
		header, _ := json.Marshal(columns)
		out[table] = notificationVisual167TableDigest{Rows: len(encoded), SHA256: notificationVisual167SHA([]byte(string(header) + "\n" + strings.Join(encoded, "\n")))}
	}
	return out
}

func notificationVisual167SHA(bytes []byte) string {
	digest := sha256.Sum256(bytes)
	return hex.EncodeToString(digest[:])
}

func notificationVisual167Write(t *testing.T, file string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal("visual167 private metadata encoding unavailable")
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("visual167 prior private metadata retained")
	}
	_, writeErr := f.Write(append(data, '\n'))
	syncErr, closeErr := f.Sync(), f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		t.Fatal("visual167 private metadata persistence unavailable")
	}
}

func notificationVisual167Gate(t *testing.T, root, directory string) {
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
	if err != nil || json.Unmarshal(data, &gate) != nil || gate.Stage != "T-NOTIFICATION-PREFERENCES-167-VISUAL" || !gate.RuntimeAuthorized || len(gate.Files) < 1 || len(gate.Files) > 100 {
		t.Fatal("visual167 independent final source/runtime gate required")
	}
	for _, input := range gate.Files {
		absolute := filepath.Join(root, filepath.FromSlash(input.Path))
		if input.Path == "" || filepath.IsAbs(input.Path) || !strings.HasPrefix(absolute, root+string(filepath.Separator)) {
			t.Fatal("visual167 gate input escaped repository")
		}
		bytes, readErr := os.ReadFile(absolute)
		if readErr != nil || len(input.SHA256) != 64 || notificationVisual167SHA(bytes) != input.SHA256 {
			t.Fatal("visual167 source gate changed before restore")
		}
	}
}
