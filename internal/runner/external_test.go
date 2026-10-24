package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExternalBridgeUsesConfiguredEndpointStableDispatchAndObservedState(t *testing.T) {
	t.Setenv("SUMA_BRIDGE_TEST_TOKEN", "bridge-token")
	var dispatchIDs []string
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer bridge-token" {
			t.Error("bridge auth missing")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && r.URL.Path == "/runs" {
			var b struct {
				DispatchID string `json:"dispatch_id"`
				Workflow   string `json:"workflow"`
			}
			_ = json.NewDecoder(r.Body).Decode(&b)
			dispatchIDs = append(dispatchIDs, b.DispatchID)
			if b.Workflow != "release" {
				t.Error("workflow missing")
			}
			_, _ = w.Write([]byte(`{"id":"remote1","state":"running"}`))
			return
		}
		calls++
		_, _ = w.Write([]byte(`{"id":"remote1","state":"succeeded","output":"release done"}`))
	}))
	defer server.Close()
	cfg := ExecConfig{External: map[string]ExternalSpec{"temporal-bridge": {URL: server.URL, TokenEnv: "SUMA_BRIDGE_TEST_TOKEN", PollMilliseconds: 1}}}
	a := Assignment{RunID: "r", NodeID: "n", Attempt: 2, Fence: 3, Type: "external_workflow", Config: map[string]any{"plugin": "temporal-bridge", "workflow": "release", "input": map[string]any{"release": "v1"}, "endpoint": "https://untrusted.example"}}
	result := Execute(context.Background(), a, cfg)
	if result.ExitCode != 0 || !contains(result.Output, "release done") || result.OutcomeUnknown {
		t.Fatalf("bridge failure %+v", result)
	}
	_ = Execute(context.Background(), a, cfg)
	if len(dispatchIDs) != 2 || dispatchIDs[0] == "" || dispatchIDs[0] != dispatchIDs[1] || calls != 2 {
		t.Fatal("dispatch identity or polling incorrect")
	}
	a.Config["plugin"] = "not-configured"
	result = Execute(context.Background(), a, cfg)
	if result.ExitCode == 0 || len(dispatchIDs) != 2 {
		t.Fatal("unconfigured plugin dispatched")
	}
}

func TestExternalCancellationRequiresObservedRemoteStop(t *testing.T) {
	t.Setenv("SUMA_BRIDGE_TEST_TOKEN", "bridge-token")
	cancelled := false
	terminal := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/runs/r1/cancel" {
			cancelled = true
			w.WriteHeader(202)
			return
		}
		state := "running"
		if terminal && cancelled {
			state = "cancelled"
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "r1", "state": state})
	}))
	defer server.Close()
	cfg := ExecConfig{External: map[string]ExternalSpec{"bridge": {URL: server.URL, TokenEnv: "SUMA_BRIDGE_TEST_TOKEN", PollMilliseconds: 1, CancelWaitMilliseconds: 30}}}
	a := Assignment{RunID: "r", NodeID: "n", Attempt: 1, Fence: 1, Type: "external_workflow", Config: map[string]any{"plugin": "bridge", "workflow": "test"}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	r := Execute(ctx, a, cfg)
	if !cancelled || r.Stopped || !r.OutcomeUnknown {
		t.Fatalf("invented remote stop %+v", r)
	}
	terminal = true
	cancelled = false
	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	r = Execute(ctx2, a, cfg)
	if !r.Stopped || r.OutcomeUnknown {
		t.Fatalf("observed cancellation not recorded %+v", r)
	}
}
