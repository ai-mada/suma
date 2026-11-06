package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/platform"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func inventoryCLIPage() platform.ToolDiscovery {
	return platform.ToolDiscovery{
		ProjectID: "project-one", WorkerID: "pool-runner-one", State: "fresh",
		AsOf: "2026-11-06T00:00:10Z", ObservedAt: "2026-11-06T00:00:00Z", ExpiresAt: "2026-11-06T00:01:00Z",
		Items: []tc.Observation{{Available: true, Profile: tc.Profile{
			CommandID: "harness-one", Adapter: "harness", Protocol: "dsh-multica-jsonl-v1",
			ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "1.0",
		}}},
	}
}

func TestToolInventoryCLIReadPreservesExactViewAndReport(t *testing.T) {
	page := inventoryCLIPage()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/tool-discovery" || len(q) != 2 || q.Get("project_id") != page.ProjectID || q.Get("worker_id") != page.WorkerID {
			t.Errorf("unexpected inventory request: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer fixture-inventory-session" ||
			r.Header.Get("X-Suma-Expected-User") != strings.Repeat("c", 32) ||
			r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("d", 32) {
			t.Error("request dropped its original authorized view")
		}
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer server.Close()
	c, err := newClient(server.URL, page.ProjectID, "fixture-inventory-session", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.setOriginalView(strings.Repeat("c", 32), strings.Repeat("d", 32)); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := toolInventoryCommand(context.Background(), c, []string{"--worker", page.WorkerID}, &out); err != nil {
		t.Fatal(err)
	}
	var got platform.ToolDiscovery
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || !reflect.DeepEqual(got, page) {
		t.Fatalf("inventory changed the report: %s, %v", out.String(), err)
	}
	before := time.Now()
	_, deadline, err := readToolInventory(context.Background(), c, page.WorkerID)
	if err != nil || deadline.Before(before.Add(50*time.Second)) || deadline.After(time.Now().Add(50*time.Second)) {
		t.Fatalf("deadline should use the server-relative remaining 50s: %v, %v", deadline, err)
	}
}

func TestToolInventoryCLIValidatesScopeFreshnessAndShape(t *testing.T) {
	for _, tt := range []struct {
		name string
		edit func(*platform.ToolDiscovery)
		ok   bool
	}{
		{"fresh empty", func(p *platform.ToolDiscovery) { p.Items = []tc.Observation{} }, true},
		{"unreported", func(p *platform.ToolDiscovery) {
			p.State, p.ObservedAt, p.ExpiresAt = "unreported", "", ""
			p.Items = []tc.Observation{}
		}, true},
		{"invalid stored time stale", func(p *platform.ToolDiscovery) {
			p.State, p.ObservedAt, p.ExpiresAt = "stale", "", ""
			p.Items = []tc.Observation{}
		}, true},
		{"future stale", func(p *platform.ToolDiscovery) {
			p.State, p.AsOf = "stale", "2026-11-05T23:59:59Z"
			p.Items = []tc.Observation{}
		}, true},
		{"expired stale", func(p *platform.ToolDiscovery) {
			p.State, p.AsOf = "stale", p.ExpiresAt
			p.Items = []tc.Observation{}
		}, true},
		{"wrong project", func(p *platform.ToolDiscovery) { p.ProjectID = "other" }, false},
		{"wrong node", func(p *platform.ToolDiscovery) { p.WorkerID = "other" }, false},
		{"fresh at expiry", func(p *platform.ToolDiscovery) { p.AsOf = p.ExpiresAt }, false},
		{"fresh before observation", func(p *platform.ToolDiscovery) { p.AsOf = "2026-11-05T23:59:59Z" }, false},
		{"invented expiry", func(p *platform.ToolDiscovery) { p.ExpiresAt = "2026-11-06T00:01:01Z" }, false},
		{"stale with items", func(p *platform.ToolDiscovery) { p.State, p.AsOf = "stale", p.ExpiresAt }, false},
		{"unreported with timestamp", func(p *platform.ToolDiscovery) { p.State, p.Items = "unreported", []tc.Observation{} }, false},
		{"null items", func(p *platform.ToolDiscovery) { p.Items = nil }, false},
		{"invalid observation", func(p *platform.ToolDiscovery) { p.Items[0].Profile.Protocol = "text-v1" }, false},
		{"duplicate command", func(p *platform.ToolDiscovery) { p.Items = append(p.Items, p.Items[0]) }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			page := inventoryCLIPage()
			project, worker := page.ProjectID, page.WorkerID
			tt.edit(&page)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(page)
			}))
			defer server.Close()
			c, err := newClient(server.URL, project, "", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_, deadline, err := readToolInventory(context.Background(), c, worker)
			wantDeadline := tt.ok && page.State == "fresh"
			if (err == nil) != tt.ok || !deadline.IsZero() != wantDeadline {
				t.Fatalf("unexpected report result: deadline=%v err=%v", deadline, err)
			}
		})
	}
}

func TestToolInventoryCLIRejectsAmbiguousWorkerAndPreservesRefusal(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"current caller refused"}}`))
	}))
	defer server.Close()
	c, err := newClient(server.URL, "project-one", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	for _, args := range [][]string{nil, {"--worker", "bad/worker"}, {"--worker", "worker-one", "--worker", "worker-two"}, {"--worker", "worker-one", "unexpected"}} {
		err := toolInventoryCommand(context.Background(), c, args, &out)
		if f, ok := err.(*failure); !ok || f.Code != "usage" {
			t.Fatalf("unexpected result for invalid arguments %q: %v", args, err)
		}
	}
	err = toolInventoryCommand(context.Background(), c, []string{"--worker", "pool-runner-one"}, &out)
	if f, ok := err.(*failure); !ok || f.Status != http.StatusForbidden || f.Code != "forbidden" || requests.Load() != 1 || out.Len() != 0 {
		t.Fatalf("authorization refusal was masked or output was written: %v, requests=%d, %q", err, requests.Load(), out.String())
	}
}
