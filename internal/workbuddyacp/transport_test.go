package workbuddyacp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/workbuddy"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixtureAccess() workbuddy.SessionAccess {
	return workbuddy.SessionAccess{Task: workbuddy.Task{ID: "task_one"}, SessionID: "task_one", Available: true, ACPURL: "https://acp.workbuddy.cn/sessions/task_one", Ticket: "fixture-ticket"}
}
func fixtureClient(t *testing.T, handler http.HandlerFunc) *http.Client {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	u, _ := url.Parse(s.URL)
	base := s.Client().Transport
	return &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != fixtureAccess().ACPURL {
			t.Error("transport escaped fixed official endpoint")
		}
		if r.Method == http.MethodPost && r.GetBody != nil {
			t.Error("POST replay hook crossed transport boundary")
		}
		cloned := r.Clone(r.Context())
		target := *r.URL
		target.Scheme, target.Host = u.Scheme, u.Host
		cloned.URL = &target
		return base.RoundTrip(cloned)
	})}
}
func streamHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Acp-Connection-Id", "fixture-connection")
	w.WriteHeader(200)
	w.(http.Flusher).Flush()
}
func mustConnect(t *testing.T, client *http.Client) *Connection {
	t.Helper()
	c, e := Connect(context.Background(), fixtureAccess(), client)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { c.Close() })
	return c
}
func assertFailure(t *testing.T, err error, unknown bool) {
	t.Helper()
	var failure *Error
	if !errors.As(err, &failure) || failure.EffectUnknown != unknown {
		t.Fatalf("wrong safe failure: %T %v", err, err)
	}
}

func TestTransportFixedHeadersSSEAndSingleSend(t *testing.T) {
	var posts atomic.Int32
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-ticket" || r.Header.Get("Cookie") != "" || r.Header.Get("Last-Event-ID") != "" || r.Header.Get("Idempotency-Key") != "" {
			t.Error("incorrect credential or replay headers")
		}
		if r.Method == http.MethodGet {
			if r.Header.Get("Accept") != "text/event-stream" {
				t.Error("wrong SSE accept")
			}
			streamHeaders(w)
			io.WriteString(w, ": keepalive\r\nid: event-1\r\nevent: update\r\ndata: {\r\ndata: \"ok\":true}\r\n\r\ndata:\r\n\r\n")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		posts.Add(1)
		if r.GetBody != nil {
			t.Error("POST unexpectedly acquired replay body")
		}
		if r.Method != http.MethodPost || r.Header.Get("Acp-Connection-Id") != "fixture-connection" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("wrong POST binding")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"jsonrpc":"2.0","id":1,"method":"initialize"}` {
			t.Error("changed request")
		}
		w.WriteHeader(202)
	})
	jar, _ := cookiejar.New(nil)
	official, _ := url.Parse(fixtureAccess().ACPURL)
	jar.SetCookies(official, []*http.Cookie{{Name: "private", Value: "not-forwarded"}})
	client.Jar = jar
	client.Timeout = time.Nanosecond
	c := mustConnect(t, client)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	frame, e := c.Receive(ctx)
	if e != nil || string(frame.Data) != "{\n\"ok\":true}" || frame.Event != "update" || frame.ID != "event-1" {
		t.Fatal("SSE multiline frame lost", e)
	}
	empty, e := c.Receive(ctx)
	if e != nil || len(empty.Data) != 0 || empty.Event != "message" || empty.ID != "event-1" {
		t.Fatal("empty SSE data or inherited ID lost", e)
	}
	if e = c.send(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)); e != nil || posts.Load() != 1 {
		t.Fatal("single send failed", e)
	}
	if client.Timeout != time.Nanosecond || client.Jar != jar {
		t.Fatal("caller client mutated")
	}
	if strings.Contains(fmt.Sprintf("%+v %#v", frame, frame), "ok") {
		t.Fatal("frame diagnostic exposed payload")
	}
	raw, e := json.Marshal(Frame{Data: []byte("private-frame-payload")})
	if e != nil || strings.Contains(string(raw), "Data") || strings.Contains(string(raw), "private") {
		t.Fatal("raw frame serialized")
	}
}

func TestTransportInvalidAccessNeverSends(t *testing.T) {
	for _, alter := range []func(*workbuddy.SessionAccess){func(a *workbuddy.SessionAccess) { a.Available = false }, func(a *workbuddy.SessionAccess) { a.ACPURL = "https://attacker.example/sessions/task_one" }, func(a *workbuddy.SessionAccess) { a.SessionID = "other" }, func(a *workbuddy.SessionAccess) { a.Ticket = "bad\r\nHeader: injected" }, func(a *workbuddy.SessionAccess) { at := time.Now().Add(-time.Second); a.ExpiresAt = &at }} {
		a := fixtureAccess()
		alter(&a)
		called := false
		_, e := Connect(context.Background(), a, &http.Client{Transport: fixtureTransport(func(*http.Request) (*http.Response, error) { called = true; return nil, errors.New("must not send") })})
		assertFailure(t, e, false)
		if called {
			t.Fatal("invalid access reached network")
		}
	}
}

func TestTransportFramesRemainBoundToTheirReceivingConnection(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		streamHeaders(w)
		io.WriteString(w, "id: provider-private-id\nevent: provider-private-event\ndata: {\"method\":\"session/request_permission\"}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	first, second := mustConnect(t, client), mustConnect(t, client)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	a, err := first.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Provider IDs and payloads can repeat across connections. They must not
	// authorize a permission reply through a different receiving connection.
	if a.ID != b.ID || string(a.Data) != string(b.Data) || a.source != first || b.source != second || a.source == b.source {
		t.Fatal("received frame lost its connection binding")
	}
	raw, err := json.Marshal(a)
	if err != nil || string(raw) != "{}" {
		t.Fatal("transient provider frame metadata serialized")
	}
}

func TestTransportRejectsRedirectAndAmbiguousConnection(t *testing.T) {
	for _, kind := range []string{"redirect", "content-type", "duplicate", "empty", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Acp-Connection-Id", "one")
				switch kind {
				case "redirect":
					w.Header().Set("Location", fixtureAccess().ACPURL)
					w.WriteHeader(307)
					return
				case "content-type":
					w.Header().Set("Content-Type", "application/json")
				case "duplicate":
					w.Header().Add("Acp-Connection-Id", "two")
				case "empty":
					w.Header().Del("Acp-Connection-Id")
				case "oversized":
					w.Header().Set("Acp-Connection-Id", strings.Repeat("x", 513))
				}
			})
			_, e := Connect(context.Background(), fixtureAccess(), client)
			assertFailure(t, e, false)
			if calls.Load() != 1 {
				t.Fatal("redirect/retry followed")
			}
		})
	}
}

func TestTransportMalformedFramesAndDisconnectRemainIncomplete(t *testing.T) {
	for _, body := range []string{"data: unfinished", "data: \xff\n\n", "data: " + strings.Repeat("x", (1<<20)+1) + "\n\n", "id: " + strings.Repeat("i", 4096) + "\ndata: " + strings.Repeat("x", (1<<20)-4096) + "\n\n", ": comment\n\n"} {
		client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { streamHeaders(w); io.WriteString(w, body) })
		c := mustConnect(t, client)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, e := c.Receive(ctx)
		cancel()
		assertFailure(t, e, false)
		c.Close()
	}
}

func TestTransportSendFailureIsUnknownAndNoReplay(t *testing.T) {
	var calls atomic.Int32
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			streamHeaders(w)
			<-r.Context().Done()
			return
		}
		calls.Add(1)
		w.WriteHeader(503)
		io.WriteString(w, "private-ticket-or-body")
	})
	c := mustConnect(t, client)
	e := c.send(context.Background(), []byte(`{"jsonrpc":"2.0"}`))
	assertFailure(t, e, true)
	if calls.Load() != 1 || strings.Contains(fmt.Sprintf("%v %#v", e, e), "private-ticket") {
		t.Fatal("unsafe retry or error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e = c.send(ctx, []byte(`{}`))
	assertFailure(t, e, false)
	if calls.Load() != 1 {
		t.Fatal("cancelled send reached network")
	}
}

func TestTransportCloseReleasesBackpressureAndReceive(t *testing.T) {
	ended := make(chan struct{})
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		defer close(ended)
		streamHeaders(w)
		for n := 0; n < 64; n++ {
			fmt.Fprint(w, "data: {}\n\n")
		}
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	c := mustConnect(t, client)
	closed := make(chan struct{})
	go func() { c.Close(); c.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close blocked on queued frames")
	}
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("SSE request not cancelled")
	}
	_, e := c.Receive(context.Background())
	assertFailure(t, e, false)
}

func TestTransportCancellationClosesOwnedStream(t *testing.T) {
	for _, mode := range []string{"connection", "receive"} {
		t.Run(mode, func(t *testing.T) {
			ended := make(chan struct{})
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { streamHeaders(w); <-r.Context().Done(); close(ended) })
			life, cancelLife := context.WithCancel(context.Background())
			defer cancelLife()
			c, e := Connect(life, fixtureAccess(), client)
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			read, cancelRead := context.WithCancel(context.Background())
			defer cancelRead()
			if mode == "connection" {
				cancelLife()
			} else {
				cancelRead()
			}
			_, e = c.Receive(read)
			assertFailure(t, e, false)
			select {
			case <-ended:
			case <-time.After(time.Second):
				t.Fatal("cancelled stream retained body")
			}
		})
	}
}
