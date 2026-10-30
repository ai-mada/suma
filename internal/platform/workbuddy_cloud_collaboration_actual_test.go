package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// This opt-in test is an actual local HTTP host, not a normal test-suite host.
// The existing cloudHistoryFixture remains the only provider/SSE fixture.
func TestWorkBuddyCloudCollaborationActualHTTPFixture(t *testing.T) {
	if os.Getenv("SUMA_CLOUD_COLLAB_ACTUAL") != "1" {
		t.Skip("owned actual164 host requires an explicit runtime gate")
	}
	working, err := os.Getwd()
	if err != nil {
		t.Fatal("actual164 working directory unavailable")
	}
	for {
		if _, err = os.Stat(filepath.Join(working, "go.mod")); err == nil {
			break
		}
		next := filepath.Dir(working)
		if next == working {
			t.Fatal("actual164 repository root unavailable")
		}
		working = next
	}
	directory := filepath.Join(working, ".cache", "workbuddy-cloud-collab-164", "actual")
	requested, err := filepath.Abs(os.Getenv("SUMA_CLOUD_COLLAB_ACTUAL_DIR"))
	if err != nil || filepath.Clean(requested) != filepath.Clean(directory) {
		t.Fatal("actual164 directory must be the exact owned cache directory")
	}
	if _, err = os.Stat(filepath.Join(directory, "run-authorized.json")); err != nil {
		t.Fatal("actual164 source freeze and runtime gate required")
	}
	private := filepath.Join(directory, "private")
	if err = os.MkdirAll(private, 0700); err != nil {
		t.Fatal("actual164 private directory unavailable")
	}
	descriptor := filepath.Join(private, "descriptor.json")
	if _, err = os.Stat(descriptor); !os.IsNotExist(err) {
		t.Fatal("actual164 original descriptor exists; inspect instead of reseeding")
	}
	history := make([]string, 55)
	for i := range history {
		kind := "agent_message_chunk"
		if i == 0 {
			kind = "user_message_chunk"
		}
		history[i] = cloudHistoryText(kind, fmt.Sprintf("164 original session observation %02d — 原会话合成文字", i+1))
	}
	var loads atomic.Int32
	h, sub, posts := cloudHistoryFixture(t, history, func() { loads.Add(1) })
	a, result := cloudResultFixture(t, h)
	result.WorkBuddyCloudWait.SubmissionID = sub.ID
	result.WorkBuddyCloudWait.SubmissionVersion = sub.Version
	if err = completeCloudFixture(t, h, a, result); err != nil {
		t.Fatal("actual164 original external wait result unavailable")
	}
	initialRun, err := h.f.e.GetRun(context.Background(), h.f.user.TenantID, h.f.project, sub.RunID)
	if err != nil || len(initialRun.Nodes) != 1 || initialRun.Nodes[0].State != "waiting" {
		t.Fatal("actual164 original waiting run unavailable")
	}
	var sessionAccess, attach, initialize, loadPost, unexpected atomic.Int32
	client := *h.w.app.HTTPClient
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.Transport = cloudHistoryRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && r.URL.Path == "/openapi/v2/tasks/cloud-task" {
			sessionAccess.Add(1)
		}
		if r.URL.Path == "/sessions/cloud-task" {
			if r.Method == http.MethodGet {
				attach.Add(1)
			} else if r.Method == http.MethodPost {
				// Inspect only the method enum in memory. Restore identical bytes
				// for the existing fixture; no packet/header/query is persisted.
				raw, readErr := io.ReadAll(io.LimitReader(r.Body, 1<<20))
				r.Body.Close()
				if readErr != nil || len(raw) >= 1<<20 {
					return nil, fmt.Errorf("actual164 protocol counter unavailable")
				}
				copyRequest := r.Clone(r.Context())
				copyRequest.Body = io.NopCloser(bytes.NewReader(raw))
				var packet struct {
					Method string `json:"method"`
				}
				if json.Unmarshal(raw, &packet) != nil {
					return nil, fmt.Errorf("actual164 protocol counter invalid")
				}
				switch packet.Method {
				case "initialize":
					initialize.Add(1)
				case "session/load":
					loadPost.Add(1)
				default:
					unexpected.Add(1)
				}
				r = copyRequest
			}
		}
		return base.RoundTrip(r)
	})
	h.w.app.HTTPClient = &client
	stats := func() map[string]any {
		out := map[string]any{"provider_creates": h.creates.Load(), "rest_queries": h.queries.Load(), "session_access": sessionAccess.Load(), "attach": attach.Load(), "initialize": initialize.Load(), "load": loadPost.Load(), "load_hook": loads.Load(), "acp_posts": posts.Load(), "unexpected_rpc": unexpected.Load()}
		for name, table := range map[string]string{"heads": "suma_workbuddy_cloud_history_heads", "loads": "suma_workbuddy_cloud_history_loads", "entries": "suma_workbuddy_cloud_history_entries"} {
			var count int
			if queryErr := h.f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); queryErr != nil {
				out["database_stats_unavailable"] = true
			} else {
				out[name] = count
			}
		}
		if run, runErr := h.f.e.GetRun(context.Background(), h.f.user.TenantID, h.f.project, sub.RunID); runErr == nil && len(run.Nodes) == 1 {
			out["run_version"], out["run_state"] = run.Version, run.State
			out["node_state"], out["node_reason"] = run.Nodes[0].State, run.Nodes[0].Reason
			out["attempt"], out["fence"] = run.Nodes[0].Attempt, run.Nodes[0].Fence
		} else {
			out["run_stats_unavailable"] = true
		}
		return out
	}
	uiOrigin := "http://127.0.0.1:43989"
	platform := (&Server{Service: h.f.s, Engine: h.f.e, WorkBuddy: h.w, WebDir: filepath.Join(directory, "ui"), PublicURL: uiOrigin, AllowedOrigins: []string{uiOrigin}}).Handler()
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/__actual164/status" {
			platform.ServeHTTP(w, r)
			return
		}
		cookie, cookieErr := r.Cookie(h.f.cookie.Name)
		if cookieErr != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(h.f.cookie.Value)) != 1 || r.Method != http.MethodGet {
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
		t.Fatal("actual164 database source unavailable")
	}
	cloudActual164PrivateWrite(t, descriptor, map[string]any{"origin": host.URL, "ui_origin": uiOrigin, "pid": os.Getpid(), "project_id": h.f.project, "run_id": sub.RunID, "node_id": sub.NodeID, "submission_id": sub.ID, "submission_version": sub.Version, "cookie": map[string]any{"name": h.f.cookie.Name, "value": h.f.cookie.Value, "httpOnly": true}, "current_owner": map[string]any{"user_id": h.f.view.ID, "organization_id": h.f.view.TenantID}, "initial_stats": stats(), "database_source": databasePath, "data_directory_source": h.f.s.DataDir, "source_lifetime": "Existing helper t.TempDir; deleted by test cleanup after consistent private snapshot export", "history_entries": len(history)})
	cloudActual164PrivateWrite(t, filepath.Join(private, "restore-metadata.json"), map[string]any{"encryption_key_base64": base64.StdEncoding.EncodeToString(h.w.key), "client_id": h.w.app.ClientID, "client_secret": h.w.app.ClientSecret, "app_digest": h.w.appDigest, "base_url": h.w.app.BaseURL, "redirect_uri": h.w.app.RedirectURI, "fixture_only": true})
	t.Log("owned164 HTTP fixture ready; credentials remain in the private descriptor")
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
	host.Close() // App/proxy close first; no live platform request remains at export.
	final := stats()
	snapshot := filepath.Join(private, "snapshot.sqlite")
	if _, err = os.Stat(snapshot); !os.IsNotExist(err) {
		t.Fatal("actual164 original snapshot exists; refusing replacement")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err = h.f.s.DB.ExecContext(ctx, "VACUUM INTO ?", snapshot); err != nil {
		t.Fatal("actual164 consistent snapshot export failed")
	}
	if err = os.Chmod(snapshot, 0600); err != nil {
		t.Fatal("actual164 private snapshot permissions unavailable")
	}
	snapshotBytes, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal("actual164 private snapshot hash unavailable")
	}
	snapshotSHA := sha256.Sum256(snapshotBytes)
	cloudActual164PrivateWrite(t, filepath.Join(private, "host-final.json"), map[string]any{"stats": final, "host_closed": true, "host_timeout": timedOut, "snapshot": map[string]any{"path": snapshot, "bytes": len(snapshotBytes), "sha256": hex.EncodeToString(snapshotSHA[:]), "method": "SQLite VACUUM INTO parameterized absent target; original WAL database still open"}, "original_database_source": databasePath, "original_data_directory_source": h.f.s.DataDir, "original_sources_cleanup_expected": true})
	if timedOut {
		t.Fatal("actual164 host deadline reached; private snapshot preserved")
	}
}

func cloudActual164PrivateWrite(t *testing.T, file string, value any) {
	t.Helper()
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("actual164 private record already exists or cannot be created")
	}
	if err = json.NewEncoder(f).Encode(value); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatal("actual164 private record could not be persisted")
	}
}
