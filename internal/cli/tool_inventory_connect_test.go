package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/platform"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestToolInventoryConnectLinkRetainsExactPublicLocator(t *testing.T) {
	page := inventoryCLIPage()
	page.ObservedAt = "2026-11-06T00:00:00.123456789Z"
	page.ExpiresAt = "2026-11-06T00:01:00.123456789Z"
	profile := page.Items[0].Profile
	profile.CommandID = "Harness-One"
	profile.ProfileSHA256 = strings.Repeat("e", 64)
	page.Items = append(page.Items, tc.Observation{Available: true, Profile: profile})
	const token = "fixture-inventory-connect-session"
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/tool-discovery" || len(q) != 2 ||
			q.Get("project_id") != page.ProjectID || q.Get("worker_id") != page.WorkerID ||
			r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer server.Close()
	c, err := newClient(server.URL, page.ProjectID, token, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{server.URL, "https://workbench.example:8443"} {
		t.Run(origin, func(t *testing.T) {
			args := []string{"--worker", page.WorkerID, "--connect-command", profile.CommandID}
			if origin != server.URL {
				args = append(args, "--workbench-origin", origin+"/")
			}
			var out bytes.Buffer
			if err := toolInventoryCommand(context.Background(), c, args, &out); err != nil {
				t.Fatal(err)
			}
			var result map[string]string
			if err := json.Unmarshal(out.Bytes(), &result); err != nil || len(result) != 1 ||
				strings.Contains(out.String(), token) || strings.Contains(out.String(), `\u0026`) {
				t.Fatalf("link output contains extra data: %s, %v", out.String(), err)
			}
			link, err := url.Parse(result["url"])
			if err != nil {
				t.Fatal(err)
			}
			expectedPanel := strings.Join([]string{"tool-inventory-connect", page.ProjectID, page.WorkerID, profile.CommandID, profile.ProfileSHA256, page.ObservedAt}, ":")
			q := link.Query()
			if link.Scheme+"://"+link.Host != origin || link.Path != "/" || link.User != nil || link.Fragment != "" ||
				len(q) != 4 || q.Get("project") != page.ProjectID || q.Get("view") != "settings" || q.Get("sub") != "tools" || q.Get("panel") != expectedPanel {
				t.Fatalf("link did not preserve exact origin or pin: %s", link)
			}
		})
	}
	if reads.Load() != 2 {
		t.Fatalf("expected one inventory read per link, got %d", reads.Load())
	}
}

func TestToolInventoryConnectLinkRefusesUnusableReportsAndOrigins(t *testing.T) {
	for _, test := range []struct {
		name      string
		alter     func(*platform.ToolDiscovery)
		extraArgs []string
		token     string
		omit      bool
		code      string
	}{
		{name: "stale", alter: func(p *platform.ToolDiscovery) {
			p.State, p.AsOf, p.Items = "stale", p.ExpiresAt, []tc.Observation{}
		}, code: "inventory_unusable"},
		{name: "expired in transit", alter: func(p *platform.ToolDiscovery) {
			p.AsOf = "2026-11-06T00:00:59.999999999Z"
		}, code: "inventory_unusable"},
		{name: "unavailable", alter: func(p *platform.ToolDiscovery) {
			p.Items[0].Available, p.Items[0].Reason = false, "executable not available"
		}, code: "inventory_command_unavailable"},
		{name: "missing", alter: func(p *platform.ToolDiscovery) {
			p.Items[0].Profile.CommandID = "another-command"
		}, code: "inventory_command_not_found"},
		{name: "wrong scope", alter: func(p *platform.ToolDiscovery) {
			p.ProjectID = "another-project"
		}, code: "response"},
		{name: "duplicate command argument", extraArgs: []string{"--connect-command", "another-command"}, code: "usage"},
		{name: "explicit origin without command", extraArgs: []string{"--workbench-origin", "https://workbench.example"}, omit: true, code: "usage"},
		{name: "origin with credentials", extraArgs: []string{"--workbench-origin", "https://user:password@workbench.example"}, code: "usage"},
		{name: "origin with path", extraArgs: []string{"--workbench-origin", "https://workbench.example/login"}, code: "usage"},
		{name: "reflected credential", token: "harness-one", code: "response"},
	} {
		t.Run(test.name, func(t *testing.T) {
			page := inventoryCLIPage()
			project, worker, command := page.ProjectID, page.WorkerID, page.Items[0].Profile.CommandID
			var reads atomic.Int32
			wantReads := int32(0)
			if test.alter != nil {
				test.alter(&page)
			}
			if test.alter != nil || test.token != "" {
				wantReads = 1
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reads.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/tool-discovery" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				_ = json.NewEncoder(w).Encode(page)
			}))
			defer server.Close()
			c, err := newClient(server.URL, project, test.token, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			args := []string{"--worker", worker}
			if !test.omit {
				args = append(args, "--connect-command", command)
			}
			var out bytes.Buffer
			err = toolInventoryCommand(context.Background(), c, append(args, test.extraArgs...), &out)
			if f, ok := err.(*failure); !ok || f.Code != test.code || out.Len() != 0 || reads.Load() != wantReads {
				t.Fatalf("expected %s and %d read(s), got %v, %d read(s), output=%q", test.code, wantReads, err, reads.Load(), out.String())
			}
		})
	}
}
