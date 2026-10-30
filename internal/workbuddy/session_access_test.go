package workbuddy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const sessionFixtureID = "2076261663968759808"
const sessionFixtureURL = "https://acp.workbuddy.cn/sessions/" + sessionFixtureID
const sessionFixtureTicket = "sandbox-secret-ticket"

func TestSessionAccessExactTaskAndSecretProjection(t *testing.T) {
	var calls atomic.Int32
	client, _ := clientFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.RequestURI() != tasksPath+"/"+sessionFixtureID || r.Header.Get("Authorization") != "Bearer "+fixtureToken || r.Header.Get("Accept") != "application/json" || r.Header.Get("Idempotency-Key") != "" {
			t.Error("request escaped the fixed authenticated task read")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"task_id":%q,"status":"future_state","link":%q,"token":%q,"expire_at":1786087200,"sandboxLink":"https://untrusted.example/private","sandboxDataLink":"https://other.example/private"}`, sessionFixtureID, sessionFixtureURL, sessionFixtureTicket)
	})
	got, err := client.GetSessionAccess(context.Background(), sessionFixtureID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Task.ID != sessionFixtureID || got.SessionID != sessionFixtureID || got.Task.Status != "future_state" || !got.Available || got.ACPURL != sessionFixtureURL || got.Ticket != sessionFixtureTicket || got.ExpiresAt == nil || got.ExpiresAt.Unix() != 1786087200 || calls.Load() != 1 {
		t.Fatal("inaccurate session access projection")
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{string(raw), fmt.Sprintf("%v", got), fmt.Sprintf("%+v", got), fmt.Sprintf("%#v", &got), fmt.Sprintf("%s", got), fmt.Sprintf("%q", got), fmt.Sprintf("%+v", map[string]any{"access": got})} {
		for _, secret := range []string{sessionFixtureTicket, sessionFixtureURL, "untrusted.example", "other.example", fixtureToken} {
			if strings.Contains(rendered, secret) {
				t.Fatal("projection leaked connection material")
			}
		}
	}
}

func TestSessionAccessPendingAndUnknownExpiry(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		available    bool
	}{
		{"not_provisioned", "", false},
		{"link_only", `,"link":"` + sessionFixtureURL + `"`, false},
		{"ticket_only", `,"token":"` + sessionFixtureTicket + `"`, false},
		{"complete_without_expiry", `,"link":"` + sessionFixtureURL + `","token":"` + sessionFixtureTicket + `"`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := clientFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"task_id":%q,"status":"CREATING"%s}`, sessionFixtureID, tc.fields)
			})
			got, err := client.GetSessionAccess(context.Background(), sessionFixtureID)
			if err != nil || got.Available != tc.available || got.ExpiresAt != nil || got.SessionID != sessionFixtureID {
				t.Fatalf("pending/unknown distinction lost: %v", err)
			}
			if !tc.available && (got.ACPURL != "" || got.Ticket != "") {
				t.Fatal("returned partial connection credentials")
			}
		})
	}
}

func TestSessionAccessRejectsUntrustedOrWrongSessionLinks(t *testing.T) {
	links := []string{
		"https://acp.workbuddy.cn/sessions/other", "https://acp.workbuddy.cn.evil.example/sessions/" + sessionFixtureID,
		"https://evil.example/sessions/" + sessionFixtureID, "http://acp.workbuddy.cn/sessions/" + sessionFixtureID,
		"https://acp.workbuddy.cn:443/sessions/" + sessionFixtureID, "https://user@acp.workbuddy.cn/sessions/" + sessionFixtureID,
		sessionFixtureURL + "?token=secret", sessionFixtureURL + "#fragment", sessionFixtureURL + "/",
		"https://acp.workbuddy.cn/sessions/%32" + sessionFixtureID[1:], "https://acp.workbuddy.cn/a/../sessions/" + sessionFixtureID,
		"https://65225-xxx.ap-guangzhou.agentos-run.net/acp", "http://127.0.0.1:9999/acp", "file:///session", "https://acp.workbuddy.cn\\@evil.example/acp",
	}
	for i, link := range links {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			var calls int
			client, _ := clientFixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != tasksPath+"/"+sessionFixtureID {
					t.Error("followed supplied connection URL")
				}
				w.Header().Set("Content-Type", "application/json")
				raw, _ := json.Marshal(map[string]any{"task_id": sessionFixtureID, "status": "working", "link": link, "token": sessionFixtureTicket})
				_, _ = w.Write(raw)
			})
			got, err := client.GetSessionAccess(context.Background(), sessionFixtureID)
			code := "invalid_response"
			if i == 1 || i == 2 || i == 11 {
				code = "unsupported_session_endpoint"
			}
			requireError(t, err, code, false)
			if got.Available || got.Ticket != "" || calls != 1 {
				t.Fatal("invalid endpoint retained credentials or triggered another request")
			}
		})
	}
}

func TestSessionAccessRejectsMalformedFacts(t *testing.T) {
	good := `{"task_id":"` + sessionFixtureID + `","status":"working","link":"` + sessionFixtureURL + `","token":"` + sessionFixtureTicket + `"}`
	invalid := []string{
		strings.Replace(good, sessionFixtureID, "other", 1), good + `{}`, strings.Replace(good, `"status":"working"`, `"status":"working","status":"completed"`, 1),
		strings.Replace(good, `"token":`, `"token":"first","token":`, 1), strings.Replace(good, `"link":`, `"link":null,"link":`, 1),
		strings.Replace(good, `"token":"`+sessionFixtureTicket+`"`, `"token":null`, 1), strings.Replace(good, sessionFixtureTicket, `bad\r\nticket`, 1),
		strings.Replace(good, sessionFixtureTicket, strings.Repeat("a", 8193), 1), strings.Replace(good, `"link":"`+sessionFixtureURL+`"`, `"link":42`, 1),
		good[:len(good)-1] + `,"expire_at":null}`, good[:len(good)-1] + `,"expire_at":0}`, good[:len(good)-1] + `,"expire_at":-1}`,
		good[:len(good)-1] + `,"expire_at":1.5}`, good[:len(good)-1] + `,"expire_at":"1786087200"}`, good[:len(good)-1] + `,"expire_at":253402300800}`,
		strings.Replace(good, "working", string([]byte{0xff}), 1), good[:len(good)-1] + `,"expire_at":1,"expire_at":2}`,
	}
	for i, body := range invalid {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			client, _ := clientFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			})
			got, err := client.GetSessionAccess(context.Background(), sessionFixtureID)
			requireError(t, err, "invalid_response", false)
			if got.Ticket != "" || got.ACPURL != "" {
				t.Fatal("rejected facts leaked partial credentials")
			}
		})
	}
}

func TestSessionAccessCancelledAndInvalidIDNeverSend(t *testing.T) {
	var calls atomic.Int32
	client, _ := clientFixture(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); t.Error("unexpected network request") })
	for _, id := range []string{"", "../other", "x/y", "x%2fy", "x?query", "x\n"} {
		_, err := client.GetSessionAccess(context.Background(), id)
		requireError(t, err, "invalid_request", false)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.GetSessionAccess(ctx, sessionFixtureID)
	requireError(t, err, "cancelled", false)
	if calls.Load() != 0 {
		t.Fatal("local validation touched network")
	}
}

func TestSessionAccessBoundedReadAndNoRedirect(t *testing.T) {
	for _, mode := range []string{"redirect", "oversized", "failure"} {
		t.Run(mode, func(t *testing.T) {
			var calls int
			client, _ := clientFixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				switch mode {
				case "redirect":
					w.Header().Set("Location", "/redirected")
					w.WriteHeader(302)
				case "oversized":
					_, _ = io.WriteString(w, strings.Repeat("x", maxResponseBytes+1))
				default:
					w.WriteHeader(500)
					_, _ = io.WriteString(w, "sensitive-server-error "+sessionFixtureTicket)
				}
			})
			_, err := client.GetSessionAccess(context.Background(), sessionFixtureID)
			code := "http_status"
			if mode == "oversized" {
				code = "invalid_response"
			}
			requireError(t, err, code, false)
			if calls != 1 {
				t.Fatal("unexpected redirect or retry")
			}
		})
	}
}

func TestSessionAccessExpiredTimestampRemainsAnObservedFact(t *testing.T) {
	client, _ := clientFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"task_id":%q,"status":"working","link":%q,"token":%q,"expire_at":1}`, sessionFixtureID, sessionFixtureURL, sessionFixtureTicket)
	})
	got, err := client.GetSessionAccess(context.Background(), sessionFixtureID)
	if err != nil || got.ExpiresAt == nil || !got.ExpiresAt.Equal(time.Unix(1, 0)) {
		t.Fatalf("provider expiry overwritten: %v", err)
	}
}

func TestSessionAccessRejectsPaddingOnlyCredentialsBeforeUse(t *testing.T) {
	var calls atomic.Int32
	var ticket string
	client, server := clientFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"task_id":%q,"status":"working","link":%q,"token":%q}`, sessionFixtureID, sessionFixtureURL, ticket)
	})
	for _, padding := range []string{"=", "==", "==="} {
		ticket = padding
		access, err := client.GetSessionAccess(context.Background(), sessionFixtureID)
		requireError(t, err, "invalid_response", false)
		if access.Available || access.Ticket != "" || access.ACPURL != "" {
			t.Fatal("padding-only ticket escaped session validation")
		}
		before := calls.Load()
		invalid, err := NewClient(Config{BaseURL: server.URL, Token: padding, HTTPClient: server.Client()})
		requireError(t, err, "invalid_config", false)
		if invalid != nil || calls.Load() != before {
			t.Fatal("padding-only API credential produced a client or network request")
		}
	}
	// Bearer values with content and trailing padding remain accepted on the
	// actual authenticated read, without treating padding as content itself.
	ticket = "opaque=="
	valid, err := NewClient(Config{BaseURL: server.URL, Token: "opaque==", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	access, err := valid.GetSessionAccess(context.Background(), sessionFixtureID)
	if err != nil || !access.Available || access.Ticket != ticket || calls.Load() != 4 {
		t.Fatalf("valid trailing padding was rejected: %v", err)
	}
}
