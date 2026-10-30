package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ai-mada/suma/internal/store"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

type cloudHistoryRoundTripper func(*http.Request) (*http.Response, error)

func (f cloudHistoryRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Uses actual HTTP/SSE and the production ACP parser; only the fixed vendor
// origins are routed to this local fixture.
func cloudHistoryFixture(t *testing.T, history []string, hook func(), loadReply ...func(json.RawMessage) (int, string)) (*cloudFixture, wc.CloudSubmission, *atomic.Int32) {
	t.Helper()
	h := newCloudFixture(t, nil)
	sub, err := h.w.CreateCloudTask(context.Background(), h.actor, h.f.project, h.request)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.f.s.InitializeWorkBuddyCloudCollaboration(context.Background()); err != nil {
		t.Fatal(err)
	}
	var streams sync.Map
	var connectionSerial atomic.Int32
	posts := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi/v2/tasks/cloud-task" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"task_id":"cloud-task","status":"waiting_input","link":"https://acp.workbuddy.cn/sessions/cloud-task","token":"fixture-history-ticket"}`)
			return
		}
		if r.URL.Path != "/sessions/cloud-task" {
			t.Error("unexpected history path", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fixture-history-ticket" {
			t.Error("wrong history credential")
		}
		if r.Method == "GET" {
			connectionID := fmt.Sprintf("history-connection-%d", connectionSerial.Add(1))
			events := make(chan string, 1024)
			streams.Store(connectionID, events)
			defer streams.Delete(connectionID)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Acp-Connection-Id", connectionID)
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			for {
				select {
				case event := <-events:
					fmt.Fprint(w, "data: "+event+"\n\n")
					w.(http.Flusher).Flush()
				case <-r.Context().Done():
					return
				}
			}
		}
		posts.Add(1)
		stream, valid := streams.Load(r.Header.Get("Acp-Connection-Id"))
		if !valid {
			t.Error("wrong connection")
			w.WriteHeader(400)
			return
		}
		events := stream.(chan string)
		var packet map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&packet) != nil {
			w.WriteHeader(400)
			return
		}
		var method string
		_ = json.Unmarshal(packet["method"], &method)
		status := 202
		switch method {
		case "initialize":
			events <- `{"jsonrpc":"2.0","id":` + string(packet["id"]) + `,"result":{"protocolVersion":1,"agentCapabilities":{"loadSession":true}}}`
		case "session/load":
			if hook != nil {
				hook()
			}
			for _, raw := range history {
				events <- raw
			}
			reply := `{"jsonrpc":"2.0","id":` + string(packet["id"]) + `,"result":{}}`
			if len(loadReply) > 0 {
				status, reply = loadReply[0](packet["id"])
			}
			events <- reply
		default:
			t.Error("history sent action", method)
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	endpoint, _ := url.Parse(srv.URL)
	base := srv.Client().Transport
	h.w.app.HTTPClient = &http.Client{Transport: cloudHistoryRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "acp.workbuddy.cn" || r.URL.Path == "/openapi/v2/tasks/cloud-task" {
			clone := r.Clone(r.Context())
			u := *r.URL
			u.Scheme, u.Host = endpoint.Scheme, endpoint.Host
			clone.URL = &u
			return base.RoundTrip(clone)
		}
		return base.RoundTrip(r)
	})}
	return h, sub, posts
}

func cloudHistoryText(kind, text string) string {
	return `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"cloud-task","update":{"sessionUpdate":"` + kind + `","content":{"type":"text","text":` + stringify(text) + `}}}}`
}
func cloudHistoryRequest(h *cloudFixture, s wc.CloudSubmission) wc.CloudHistoryLoadRequest {
	return wc.CloudHistoryLoadRequest{ProjectID: h.f.project, ExpectedSubmissionVersion: s.Version, ExpectedVersion: 0, IdempotencyKey: newID()}
}

func TestWorkBuddyCloudCollaborationPersistsSafeHistoryAndReplaysWithoutAttach(t *testing.T) {
	h, sub, posts := cloudHistoryFixture(t, []string{cloudHistoryText("user_message_chunk", "same"), cloudHistoryText("agent_message_chunk", "same"), cloudHistoryText("agent_thought_chunk", "private thought"), `{"jsonrpc":"2.0","method":"_codebuddy.ai/artifact","params":{"sessionId":"cloud-task","event":"created","artifact":{"type":"plan","uri":"https://private.example/secret"}}}`}, nil)
	ctx := context.Background()
	req := cloudHistoryRequest(h, sub)
	initial, err := h.w.CloudCollaboration(ctx, h.f.actor, h.f.project, sub.ID)
	if err != nil || initial.Version != 0 || initial.LatestLoad != nil {
		t.Fatal("initial archive", err)
	}
	load, err := h.w.LoadCloudHistory(ctx, h.f.actor, h.f.project, sub.ID, req)
	if err != nil || load.State != "loaded" || load.Coverage != "unverified" || load.EntryCount != 4 || load.UnsupportedCount != 2 {
		t.Fatalf("load: %+v %v", load, err)
	}
	replay, err := h.w.LoadCloudHistory(ctx, h.f.actor, h.f.project, sub.ID, req)
	if err != nil || replay.ID != load.ID || posts.Load() != 2 {
		t.Fatal("replay reattached", err, posts.Load())
	}
	changed := req
	changed.ExpectedVersion = 1
	if _, err = h.w.LoadCloudHistory(ctx, h.f.actor, h.f.project, sub.ID, changed); err == nil || posts.Load() != 2 {
		t.Fatal("changed replay accepted")
	}
	page, err := h.w.CloudHistory(ctx, h.f.actor, h.f.project, load.ID, 0, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].Text != "same" || !page.HasMore || page.NextAfter != 1 {
		t.Fatal("first local page", err)
	}
	page, err = h.w.CloudHistory(ctx, h.f.actor, h.f.project, load.ID, 1, 100)
	if err != nil || len(page.Items) != 3 || page.Items[0].Text != "same" {
		t.Fatal("duplicate arrival omitted", err)
	}
	encoded := stringify(page)
	if strings.Contains(encoded, "private thought") || strings.Contains(encoded, "private.example") || strings.Contains(encoded, "fixture-history-ticket") {
		t.Fatal("private protocol content persisted")
	}
	var raw string
	if err = h.f.s.DB.QueryRow(`SELECT payload FROM suma_workbuddy_cloud_history_loads WHERE id=?`, load.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "private thought") || strings.Contains(raw, "fixture-history-ticket") {
		t.Fatal("raw history persisted")
	}
	// Observation growth keeps the same source and does not invalidate the archive.
	if _, err = h.w.ObserveCloudTask(ctx, h.f.actor, h.f.project, sub.ID, wc.CloudObserveRequest{ProjectID: h.f.project, ExpectedVersion: sub.Version, IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	if _, err = h.w.CloudHistory(ctx, h.f.actor, h.f.project, load.ID, 0, 50); err != nil {
		t.Fatal("observation invalidated archive", err)
	}
	other := cloudOtherWriter(t, h)
	if _, err = h.w.CloudHistory(ctx, other, h.f.project, load.ID, 0, 50); err == nil {
		t.Fatal("other writer read private history")
	}
}

func TestWorkBuddyCloudCollaborationLimitsPrefixAndPreservesLastLoaded(t *testing.T) {
	history := []string{cloudHistoryText("agent_message_chunk", "first")}
	h, sub, _ := cloudHistoryFixture(t, history, nil)
	ctx := context.Background()
	req := cloudHistoryRequest(h, sub)
	first, err := h.w.LoadCloudHistory(ctx, h.f.actor, h.f.project, sub.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	// Decode/size rejection must preserve previously authorized observations.
	history[0] = cloudHistoryText("agent_message_chunk", strings.Repeat("x", (64<<10)+1))
	view, err := h.w.CloudCollaboration(ctx, h.f.actor, h.f.project, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	req.IdempotencyKey = newID()
	req.ExpectedVersion = view.Version
	limited, err := h.w.LoadCloudHistory(ctx, h.f.actor, h.f.project, sub.ID, req)
	if err != nil || limited.State != "limited" || limited.EntryCount != 0 {
		t.Fatal("limit", limited.State, err)
	}
	view, err = h.w.CloudCollaboration(ctx, h.f.actor, h.f.project, sub.ID)
	if err != nil || view.LatestLoad.ID != limited.ID || view.LastLoaded.ID != first.ID {
		t.Fatal("failed load erased last loaded", err)
	}
}

func TestWorkBuddyCloudCollaborationRevocationDuringLoadStopsCollection(t *testing.T) {
	var h *cloudFixture
	history := []string{cloudHistoryText("agent_message_chunk", "never persisted")}
	var sub wc.CloudSubmission
	h, sub, _ = cloudHistoryFixture(t, history, func() {
		_, err := h.f.s.DB.Exec(`UPDATE suma_workbuddy_connections SET state='disconnected' WHERE id=?`, h.delegation.ConnectionID)
		if err != nil {
			t.Error(err)
		}
	})
	_, err := h.w.LoadCloudHistory(context.Background(), h.f.actor, h.f.project, sub.ID, cloudHistoryRequest(h, sub))
	if err == nil {
		t.Fatal("revoked grant published a load")
	}
	var count int
	if err = h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_workbuddy_cloud_history_entries`).Scan(&count); err != nil || count != 0 {
		t.Fatal("revoked callback persisted", count, err)
	}
}

func TestWorkBuddyCloudCollaborationSchemaRejectsPartialBeforeWrites(t *testing.T) {
	db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "partial-history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &Service{DB: db, Dialect: "sqlite"}
	if _, err := db.Exec(`CREATE TABLE suma_workbuddy_cloud_history_heads(id TEXT PRIMARY KEY,user_note TEXT);INSERT INTO suma_workbuddy_cloud_history_heads VALUES('original','preserve')`); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeWorkBuddyCloudCollaboration(context.Background()); err == nil {
		t.Fatal("partial collaboration schema accepted")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name<>'suma_workbuddy_cloud_history_heads'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial schema wrote before rejection", err)
	}
	var note string
	if err = db.QueryRow(`SELECT user_note FROM suma_workbuddy_cloud_history_heads WHERE id='original'`).Scan(&note); err != nil || note != "preserve" {
		t.Fatal("rejection modified unique original data", err)
	}
}

func TestWorkBuddyCloudCollaborationHTTPAndRPCMustBothConfirmLoad(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		result string
	}{{"http_rejected", 503, `"result":{}`}, {"rpc_rejected", 202, `"error":{"code":-32603,"message":"private remote failure"}`}} {
		t.Run(test.name, func(t *testing.T) {
			h, sub, posts := cloudHistoryFixture(t, []string{cloudHistoryText("agent_message_chunk", "observed")}, nil, func(id json.RawMessage) (int, string) {
				return test.status, `{"jsonrpc":"2.0","id":` + string(id) + `,` + test.result + `}`
			})
			load, err := h.w.LoadCloudHistory(context.Background(), h.f.actor, h.f.project, sub.ID, cloudHistoryRequest(h, sub))
			if err != nil || load.State != "interrupted" || load.ErrorCode != "workbuddy_cloud_history_interrupted" || posts.Load() != 2 {
				t.Fatal("inconsistent acknowledgements published", load.State, err)
			}
			view, err := h.w.CloudCollaboration(context.Background(), h.f.actor, h.f.project, sub.ID)
			if err != nil || view.LastLoaded != nil || strings.Contains(stringify(load), "private remote failure") {
				t.Fatal("unconfirmed load became confirmed", err)
			}
		})
	}
}

func TestWorkBuddyCloudCollaborationBoundsEntryCountAndTotalText(t *testing.T) {
	for _, test := range []struct {
		name         string
		history      []string
		count, bytes int
	}{
		{"entries", func() []string {
			out := make([]string, 501)
			for i := range out {
				out[i] = cloudHistoryText("agent_message_chunk", "x")
			}
			return out
		}(), 500, 500},
		{"bytes", func() []string {
			out := make([]string, 17)
			for i := range out {
				out[i] = cloudHistoryText("agent_message_chunk", strings.Repeat("x", 64<<10))
			}
			return out
		}(), 16, 1 << 20},
	} {
		t.Run(test.name, func(t *testing.T) {
			h, sub, _ := cloudHistoryFixture(t, test.history, nil)
			req := cloudHistoryRequest(h, sub)
			load, err := h.w.LoadCloudHistory(context.Background(), h.f.actor, h.f.project, sub.ID, req)
			if err != nil || load.State != "limited" || load.EntryCount != test.count || load.TextBytes != test.bytes {
				t.Fatal("unbounded history", load.State, load.EntryCount, load.TextBytes, err)
			}
			page, err := h.w.CloudHistory(context.Background(), h.f.actor, h.f.project, load.ID, test.count-1, 100)
			if err != nil || len(page.Items) != 1 || page.Items[0].Sequence != test.count || page.HasMore {
				t.Fatal("authorized prefix not retained", err)
			}
		})
	}
}
