package platform

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
)

func TestRunStreamCursorRejectsAmbiguousOrUnsafeValues(t *testing.T) {
	for _, tc := range []struct {
		query, header string
		want          int64
		bad           bool
	}{
		{"", "", 0, false}, {"after=0", "0", 0, false}, {"after=91", "", 91, false},
		{"", "9007199254740991", 9007199254740991, false}, {"after=1", "2", 0, true},
		{"after=-1", "", 0, true}, {"after=1.5", "", 0, true}, {"after=+1", "", 0, true},
		{"after=9007199254740992", "", 0, true}, {"after=1&after=1", "", 0, true}, {"after=", "", 0, true},
		{"token=secret", "", 0, true}, {"access_token=secret", "", 0, true},
	} {
		r := httptest.NewRequest("GET", "/?"+tc.query, nil)
		if tc.header != "" {
			r.Header.Set("Last-Event-ID", tc.header)
		}
		got, err := runStreamCursor(r)
		if (err != nil) != tc.bad || !tc.bad && got != tc.want {
			t.Fatalf("query %q header %q => %d %v", tc.query, tc.header, got, err)
		}
	}
}

func streamRun(t *testing.T, f *fixture) dag.Run {
	t.Helper()
	ctx := context.Background()
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{ID: newID(), Name: "stream", Version: 1, Nodes: []dag.Node{{ID: "one", Name: "one", Type: "approval"}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func streamRequest(t *testing.T, server *httptest.Server, f *fixture, run string, after int64) (*http.Response, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/v1/runs/"+run+"/events/stream?project_id="+f.project+"&after="+strconv.FormatInt(after, 10), nil)
	req.AddCookie(f.cookie)
	response, err := server.Client().Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	return response, cancel
}

func nextStreamEvent(t *testing.T, reader *bufio.Reader) dag.Event {
	t.Helper()
	var id int64
	var event string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if strings.HasPrefix(line, "id: ") {
			id, _ = strconv.ParseInt(strings.TrimPrefix(line, "id: "), 10, 64)
		}
		if strings.HasPrefix(line, "event: ") {
			event = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") && event == "run_event" {
			var ev dag.Event
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
				t.Fatal(err)
			}
			if ev.Sequence != id {
				t.Fatalf("id %d sequence %d", id, ev.Sequence)
			}
			return ev
		}
	}
}

func TestRunStreamActualHTTPReplayResumeAndSessionRevocation(t *testing.T) {
	f := newFixture(t)
	run := streamRun(t, f)
	persisted, err := f.e.Events(context.Background(), f.user.TenantID, f.project, run.ID, 0)
	if err != nil || len(persisted) < 1 {
		t.Fatalf("events %v %v", persisted, err)
	}
	server := httptest.NewUnstartedServer(f.handler)
	server.Config.WriteTimeout = 30 * time.Second
	server.Start()
	defer server.Close()
	response, cancel := streamRequest(t, server, f, run.ID, 0)
	if response.StatusCode != 200 || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status %d", response.StatusCode)
	}
	reader := bufio.NewReader(response.Body)
	for _, want := range persisted {
		got := nextStreamEvent(t, reader)
		if got.Sequence != want.Sequence || got.RunID != run.ID {
			t.Fatalf("got %#v want %#v", got, want)
		}
	}
	cancel()
	response.Body.Close()
	// A new server instance has no process event cache, and resumes from durable SQL.
	restarted := httptest.NewServer((&Server{Service: f.s, Engine: f.e}).Handler())
	defer restarted.Close()
	resumed, stop := streamRequest(t, restarted, f, run.ID, persisted[0].Sequence)
	defer stop()
	defer resumed.Body.Close()
	resumedReader := bufio.NewReader(resumed.Body)
	for _, want := range persisted[1:] {
		if got := nextStreamEvent(t, resumedReader); got.Sequence != want.Sequence {
			t.Fatalf("duplicate or gap %d want %d", got.Sequence, want.Sequence)
		}
	}
	if err = f.s.Logout(context.Background(), f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	rest, err := io.ReadAll(resumedReader)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rest), "event: run_event") || !strings.Contains(string(rest), "stream_error") {
		t.Fatalf("revocation leaked events or missing visible error: %s", rest)
	}
}

func TestRunStreamActualHTTPSuccessfulTerminalEventHasObjectMetadata(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	workflow, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{
		ID: newID(), Name: "successful stream", Version: 1,
		Nodes: []dag.Node{{ID: "done", Name: "done", Type: "noop"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, workflow.ID, newID())
	if err != nil || run.State != "succeeded" {
		t.Fatalf("real noop run state %q: %v", run.State, err)
	}
	var finalSequence int64
	var persistedMetadata string
	if err = f.s.DB.QueryRow(`SELECT sequence,data FROM dag_events WHERE tenant_id=? AND project_id=? AND run_id=? AND event_type='run_succeeded'`, f.user.TenantID, f.project, run.ID).Scan(&finalSequence, &persistedMetadata); err != nil {
		t.Fatal(err)
	}
	if persistedMetadata != "null" {
		t.Fatalf("expected actual terminal nil metadata persisted as null, got %s", persistedMetadata)
	}
	server := httptest.NewServer(f.handler)
	defer server.Close()
	response, cancel := streamRequest(t, server, f, run.ID, 0)
	defer cancel()
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("stream status %d", response.StatusCode)
	}
	scanner := bufio.NewScanner(response.Body)
	var frameID, eventName, data string
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "id: "):
			frameID = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			eventName = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "":
			if eventName == "run_event" {
				var wire struct {
					Sequence int64           `json:"sequence"`
					RunID    string          `json:"run_id"`
					Type     string          `json:"type"`
					Data     json.RawMessage `json:"data"`
				}
				if err = json.Unmarshal([]byte(data), &wire); err != nil {
					t.Fatal(err)
				}
				if wire.Type == "run_succeeded" {
					if wire.Sequence != finalSequence || frameID != strconv.FormatInt(finalSequence, 10) || wire.RunID != run.ID {
						t.Fatalf("terminal frame identity mismatch: %s", data)
					}
					if string(wire.Data) != "{}" {
						t.Fatalf("terminal SSE metadata must be an object, got %s", wire.Data)
					}
					// Public normalization must leave the immutable persisted fact intact.
					if err = f.s.DB.QueryRow(`SELECT data FROM dag_events WHERE sequence=?`, finalSequence).Scan(&persistedMetadata); err != nil || persistedMetadata != "null" {
						t.Fatalf("persisted terminal metadata changed to %q: %v", persistedMetadata, err)
					}
					return
				}
			}
			frameID, eventName, data = "", "", ""
		}
	}
	t.Fatalf("successful terminal event did not reach the client: %v", scanner.Err())
}

func TestRunStreamRejectsCrossProjectRunnerAndMissingRun(t *testing.T) {
	f := newFixture(t)
	run := streamRun(t, f)
	for _, tc := range []struct {
		path  string
		token string
		want  int
	}{
		{"/runs/" + run.ID + "/events/stream?project_id=" + newID(), "", 404},
		{"/runs/" + newID() + "/events/stream?project_id=" + f.project, "", 404},
	} {
		f.call("GET", tc.path, nil, tc.want)
	}
	token, err := f.s.CreateToken(context.Background(), f.user, "runner", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`INSERT INTO suma_token_scopes(token_hash,kind,project_id)VALUES(?,?,?)`, digest(token), "runner", f.project); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/v1/runs/"+run.ID+"/events/stream?project_id="+f.project, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("runner status %d", w.Code)
	}
}

func TestRunStreamCursorBelongsToCurrentRunAndHeadersAgree(t *testing.T) {
	f := newFixture(t)
	one, two := streamRun(t, f), streamRun(t, f)
	events, err := f.e.Events(context.Background(), f.user.TenantID, f.project, one.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	base := "/runs/" + two.ID + "/events/stream?project_id=" + f.project
	f.call("GET", base+"&after="+strconv.FormatInt(events[0].Sequence, 10), nil, 400)
	f.call("GET", base+"&after=9007199254740991", nil, 400)
	request := httptest.NewRequest("GET", "/api/v1"+base+"&after=0", nil)
	request.AddCookie(f.cookie)
	request.Header.Set("Last-Event-ID", "1")
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, request)
	if w.Code != 400 {
		t.Fatalf("conflict returned %d", w.Code)
	}
	request = httptest.NewRequest("GET", base, nil)
	request.Header.Add("Last-Event-ID", "0")
	request.Header.Add("Last-Event-ID", "0")
	if _, err = runStreamCursor(request); err == nil {
		t.Fatal("duplicate Last-Event-ID accepted")
	}
}

func TestRunStreamActualHTTPKeepsLiveUpdatesAndReleasesOnCancel(t *testing.T) {
	f := newFixture(t)
	run := streamRun(t, f)
	events, _ := f.e.Events(context.Background(), f.user.TenantID, f.project, run.ID, 0)
	server := httptest.NewServer(f.handler)
	defer server.Close()
	response, cancel := streamRequest(t, server, f, run.ID, events[len(events)-1].Sequence)
	if _, err := f.e.Command(context.Background(), f.user.TenantID, f.project, run.ID, "pause", newID(), run.Version, nil); err != nil {
		t.Fatal(err)
	}
	got := nextStreamEvent(t, bufio.NewReader(response.Body))
	if got.Sequence <= events[len(events)-1].Sequence || got.RunID != run.ID {
		t.Fatalf("live event %#v", got)
	}
	cancel()
	response.Body.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		runStreams.Lock()
		count := runStreams.actors[f.user.TenantID+":"+f.user.ID]
		runStreams.Unlock()
		if count == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("cancelled connection retained its resource slot")
}

func TestRunStreamActualHTTPIdleHeartbeatsAndCleanRotation(t *testing.T) {
	f := newFixture(t)
	run := streamRun(t, f)
	events, err := f.e.Events(context.Background(), f.user.TenantID, f.project, run.ID, 0)
	if err != nil || len(events) == 0 {
		t.Fatalf("persisted run events: %v %v", events, err)
	}
	server := httptest.NewUnstartedServer(f.handler)
	server.Config.WriteTimeout = 30 * time.Second
	server.Start()
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 26*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/runs/"+run.ID+"/events/stream?project_id="+f.project+"&after="+strconv.FormatInt(events[len(events)-1].Sequence, 10), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(f.cookie)
	started := time.Now()
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("stream status %d", response.StatusCode)
	}
	reader := bufio.NewReader(response.Body)
	heartbeats := 0
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			if readErr != io.EOF {
				t.Fatalf("idle stream ended after %s with %d heartbeats: %v", time.Since(started), heartbeats, readErr)
			}
			break
		}
		if strings.TrimSpace(line) == ": heartbeat" {
			heartbeats++
		}
	}
	if heartbeats < 2 {
		t.Fatalf("idle stream emitted only %d heartbeats", heartbeats)
	}
	if elapsed := time.Since(started); elapsed < 19*time.Second || elapsed > 24*time.Second {
		t.Fatalf("expected a clean 20-second rotation, got %s", elapsed)
	}
}

func TestRunStreamLimitsConnectionsAndReleasesSlots(t *testing.T) {
	f := newFixture(t)
	run := streamRun(t, f)
	server := httptest.NewServer(f.handler)
	defer server.Close()
	var responses []*http.Response
	var stops []context.CancelFunc
	defer func() {
		for i, r := range responses {
			stops[i]()
			r.Body.Close()
		}
	}()
	for range 4 {
		r, cancel := streamRequest(t, server, f, run.ID, 0)
		responses = append(responses, r)
		stops = append(stops, cancel)
		if r.StatusCode != 200 {
			t.Fatalf("within quota returned %d", r.StatusCode)
		}
	}
	r, cancel := streamRequest(t, server, f, run.ID, 0)
	defer cancel()
	defer r.Body.Close()
	if r.StatusCode != 429 || r.Header.Get("Retry-After") == "" {
		t.Fatalf("quota status %d", r.StatusCode)
	}
}

func TestRunStreamActualHTTPBackpressureHasDeadlineAndDoesNotHoldDatabase(t *testing.T) {
	f := newFixture(t)
	run := streamRun(t, f)
	tx, err := f.s.DB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"output": strings.Repeat("x", 512*1024)})
	var sequence int64
	if err = tx.QueryRow(`SELECT MAX(sequence) FROM dag_events`).Scan(&sequence); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	for range 40 {
		sequence++
		if _, err = tx.Exec(`INSERT INTO dag_events(sequence,tenant_id,project_id,run_id,node_id,event_type,at_time,data)VALUES(?,?,?,?,?,?,?,?)`, sequence, f.user.TenantID, f.project, run.ID, "one", "node_progress", now(), string(data)); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(f.handler)
	server.Config.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
		if tcp, ok := c.(*net.TCPConn); ok {
			if err := tcp.SetWriteBuffer(1024); err != nil {
				t.Error(err)
			}
		}
		return ctx
	}
	server.Start()
	defer server.Close()
	conn, err := net.DialTimeout("tcp", strings.TrimPrefix(server.URL, "http://"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if tcp, ok := conn.(*net.TCPConn); ok {
		if err = tcp.SetReadBuffer(1024); err != nil {
			t.Fatal(err)
		}
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprintf(conn, "GET /api/v1/runs/%s/events/stream?project_id=%s&after=0 HTTP/1.1\r\nHost: suma.test\r\nCookie: suma_session=%s\r\n\r\n", run.ID, f.project, f.cookie.Value)
	reader := bufio.NewReader(conn)
	for {
		line, e := reader.ReadString('\n')
		if e != nil {
			t.Fatal(e)
		}
		if line == "\r\n" {
			break
		}
	}
	// Stop reading a real socket. A concurrent governance-lock transaction must
	// still commit, and the slow writer must release its connection budget.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tx, err = f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.authorizeProjectTx(ctx, tx, f.user, f.project, false, false); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runStreams.Lock()
		count := runStreams.actors[f.user.TenantID+":"+f.user.ID]
		runStreams.Unlock()
		if count == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("slow socket writer outlived its bounded write deadline")
}
