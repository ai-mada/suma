package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/platform"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestToolMatchesInventoryCLIReadsExactAvailableProfile(t *testing.T) {
	for _, worker := range []string{"runner-one", "pool-runner-two"} {
		t.Run(worker, func(t *testing.T) {
			profile := toolMatchCLIProfile(t)
			other := profile
			other.CommandID = "other-command"
			const user = "1234567890abcdef1234567890abcdef"
			const organization = "fedcba0987654321fedcba0987654321"
			const project = "project-one"
			var requests []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.Method+" "+r.URL.Path)
				if r.URL.Query().Get("project_id") != project || r.Header.Get("Authorization") != "Bearer test-session" || r.Header.Get("X-Suma-Expected-User") != user || r.Header.Get("X-Suma-Expected-Organization") != organization {
					t.Error("request lost original project or caller")
				}
				switch r.Method + " " + r.URL.Path {
				case "GET /api/v1/tool-discovery":
					if r.URL.Query().Get("worker_id") != worker || len(r.URL.Query()) != 2 {
						t.Error("inventory request changed exact worker or added arguments")
					}
					page := testMatchInventory(project, worker, tc.Observation{Profile: other, Available: true}, tc.Observation{Profile: profile, Available: true})
					_ = json.NewEncoder(w).Encode(page)
				case "POST /api/v1/tool-profiles/matches":
					var raw map[string]json.RawMessage
					if json.NewDecoder(r.Body).Decode(&raw) != nil || len(raw) != 3 || len(r.URL.Query()) != 1 {
						t.Error("match read acquired extra fields or write controls")
					}
					encoded, _ := json.Marshal(raw)
					var request platform.ToolProfileMatchRequest
					if json.Unmarshal(encoded, &request) != nil || request.ProjectID != project || request.WorkerID != worker || !tc.EqualProfile(request.Profile, profile) {
						t.Error("match request did not contain the selected complete observation")
					}
					_ = json.NewEncoder(w).Encode(platform.ToolProfileMatchList{
						ProjectID: project, WorkerID: worker, Profile: profile, Total: 1,
						Items: []platform.ToolProfile{{ID: "existing-one", ProjectID: project, WorkerID: worker, Version: 2, State: "paused", Profile: profile}},
					})
				default:
					t.Error("unexpected request", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			var out, stderr bytes.Buffer
			args := []string{"--control", server.URL, "--project", project, "--token", "test-session", "--expected-user", user, "--expected-organization", organization, "tools", "matches", "--worker", worker, "--command", profile.CommandID}
			code := Execute(context.Background(), args, nil, &out, &stderr)
			if code != 0 || !strings.Contains(out.String(), "existing-one") || !reflect.DeepEqual(requests, []string{"GET /api/v1/tool-discovery", "POST /api/v1/tool-profiles/matches"}) {
				t.Fatalf("exact tool lookup did not complete once: exit=%d requests=%v output=%q error=%q", code, requests, out.String(), stderr.String())
			}
		})
	}
}

func TestToolMatchesInventoryCLIFailsClosed(t *testing.T) {
	const project, worker = "project-one", "runner-one"
	profile := toolMatchCLIProfile(t)
	base := testMatchInventory(project, worker, tc.Observation{Profile: profile, Available: true})
	withoutItems := func(p *platform.ToolDiscovery) { p.Items = []tc.Observation{} }
	for _, test := range []struct {
		name   string
		mutate func(*platform.ToolDiscovery)
		code   string
	}{
		{"unreported", func(p *platform.ToolDiscovery) {
			p.State, p.ObservedAt, p.ExpiresAt = "unreported", "", ""
			withoutItems(p)
		}, "inventory_unusable"},
		{"stale", func(p *platform.ToolDiscovery) { p.State, p.AsOf = "stale", p.ExpiresAt; withoutItems(p) }, "inventory_unusable"},
		{"expired in transit", func(p *platform.ToolDiscovery) {
			until, _ := time.Parse(time.RFC3339Nano, p.ExpiresAt)
			p.AsOf = until.Add(-time.Nanosecond).Format(time.RFC3339Nano)
		}, "inventory_unusable"},
		{"no exact command", withoutItems, "inventory_command_not_found"},
		{"unavailable", func(p *platform.ToolDiscovery) {
			p.Items = []tc.Observation{{Profile: profile, Available: false, Reason: "executable no longer readable"}}
		}, "inventory_command_unavailable"},
		{"different scope", func(p *platform.ToolDiscovery) { p.WorkerID = "another-runner" }, "response"},
	} {
		t.Run(test.name, func(t *testing.T) {
			page := base
			test.mutate(&page)
			var reads int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/api/v1/tool-discovery" {
					t.Error("unusable inventory reached match endpoint", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				reads++
				_ = json.NewEncoder(w).Encode(page)
			}))
			defer server.Close()
			var out, stderr bytes.Buffer
			args := []string{"--control", server.URL, "--project", project, "--language", "zh-CN", "tools", "matches", "--worker", worker, "--command", profile.CommandID}
			code := Execute(context.Background(), args, nil, &out, &stderr)
			if code != 1 || reads != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), test.code) {
				t.Fatalf("unusable inventory did not return the expected diagnostic: exit=%d reads=%d output=%q error=%q", code, reads, out.String(), stderr.String())
			}
			if test.code != "response" && (!strings.Contains(stderr.String(), "tools inventory") || !strings.Contains(stderr.String(), "上报")) {
				t.Fatal("Chinese diagnostic did not explain how to inspect the original inventory", stderr.String())
			}
		})
	}
}

func TestToolMatchesInventoryCLIRejectsAmbiguousOptionsAndShowsHelp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid options reached the control plane", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	defer server.Close()
	base := []string{"--control", server.URL, "--project", "project-one", "--language", "en-US", "tools"}
	for _, flags := range [][]string{
		{"--worker", "runner-one"},
		{"--command", "probe"},
		{"--file", "-", "--worker", "runner-one", "--command", "probe"},
		{"--file=", "--worker", "runner-one", "--command", "probe"},
		{"--worker", "runner-one", "--worker", "other", "--command", "probe"},
		{"--worker", "runner-one", "--command", "probe", "--command", "other"},
		{"--worker", "bad/worker", "--command", "probe"},
		{"--worker", "runner-one", "--command", "bad/command"},
	} {
		var out, stderr bytes.Buffer
		args := append(append([]string{}, base...), "matches")
		code := Execute(context.Background(), append(args, flags...), nil, &out, &stderr)
		if code != 2 || out.Len() != 0 || !strings.Contains(stderr.String(), "--worker") {
			t.Fatalf("ambiguous options did not fail as usage: %v exit=%d error=%q", flags, code, stderr.String())
		}
	}
	for _, language := range []string{"en-US", "zh-CN"} {
		var out, stderr bytes.Buffer
		args := []string{"--language", language, "tools", "inventory", "--help"}
		code := Execute(context.Background(), args, nil, &out, &stderr)
		if code != 0 || !strings.Contains(out.String(), "tools inventory --worker") || !strings.Contains(out.String(), "tools matches --worker") {
			t.Fatalf("missing localized help: %s exit=%d output=%q error=%q", language, code, out.String(), stderr.String())
		}
	}
}

func testMatchInventory(project, worker string, items ...tc.Observation) platform.ToolDiscovery {
	observed := time.Now().UTC()
	return platform.ToolDiscovery{
		ProjectID: project, WorkerID: worker, State: "fresh",
		AsOf: observed.Format(time.RFC3339Nano), ObservedAt: observed.Format(time.RFC3339Nano),
		ExpiresAt: observed.Add(time.Minute).Format(time.RFC3339Nano), Items: items,
	}
}
