package modelgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	mc "github.com/ai-mada/suma/internal/modelcontract"
)

func bindingFixture(t *testing.T, base string, output int64) mc.Binding {
	t.Helper()
	endpoint, _ := mc.EndpointDigest(base)
	d := mc.Deployment{ID: "gateway118", Revision: 1, Protocol: mc.ProviderProtocol, Model: "coding118", EndpointOrigin: strings.TrimSuffix(base, "/v1"), EndpointConfigSHA256: endpoint, ContextWindow: 100}
	d.SHA256 = mc.DeploymentDigest(d)
	b, err := mc.NewBinding(d, mc.Parameters{MaxSteps: 3, MaxOutputTokens: output}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func callGateway(t *testing.T, connection Connection, step int, body string) (int, []byte) {
	t.Helper()
	r, _ := http.NewRequest("POST", connection.BaseURL+"/chat/completions", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+connection.Token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Suma-Model-Step", fmt.Sprint(step))
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, raw
}

const requestFixture = `{"model":"coding118","messages":[{"role":"user","content":"Implement the task"}],"stream":false,"max_tokens":10}`

func TestGatewayDurableAdmissionClampsAndReplaysWithoutProviderRetry(t *testing.T) {
	var calls atomic.Int32
	dir := filepath.Join(t.TempDir(), "model")
	var binding mc.Binding
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		raw, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		expected := float64(10)
		output := 3
		if n == 2 {
			expected, output = 7, 7
		}
		if r.Header.Get("Authorization") != "Bearer owned-provider-secret" || payload["model"] != "coding118" || payload["max_tokens"] != expected {
			t.Error("provider received unfrozen model/allowance/credential")
		}
		saved, err := ReadSnapshot(dir, strings.Repeat("a", 64), binding.BindingSHA256)
		if err != nil || len(saved.Records) != int(n) || saved.Records[n-1].State != "started" || saved.Records[n-1].RequestSHA256 != digest(raw) {
			t.Error("chargeable request escaped before its exact durable admission")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"response%d","model":"coding118","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":%d}}`, n, output)
	}))
	defer provider.Close()
	binding = bindingFixture(t, provider.URL+"/v1", 10)
	g, err := Open(Config{Context: context.Background(), RequestID: strings.Repeat("a", 64), Directory: dir, Binding: binding, BaseURL: provider.URL + "/v1", APIKey: "owned-provider-secret"})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	status, first := callGateway(t, g.Connection(), 0, requestFixture)
	if status != 200 {
		t.Fatal(status, string(first))
	}
	status, replay := callGateway(t, g.Connection(), 0, requestFixture)
	if status != 200 || string(first) != string(replay) || calls.Load() != 1 {
		t.Fatal("lost response replay made a second provider operation")
	}
	status, _ = callGateway(t, g.Connection(), 1, requestFixture)
	if status != 200 || calls.Load() != 2 {
		t.Fatal("remaining allowance was not forwarded")
	}
	status, replay = callGateway(t, g.Connection(), 0, requestFixture)
	if status != 200 || string(first) != string(replay) || calls.Load() != 2 {
		t.Fatal("older completed operation could not replay its private artifact")
	}
	snapshot, err := g.Close()
	usage, complete := mc.AggregateEvidenceUsage(binding, Evidence(snapshot))
	if err != nil || !complete || usage == nil || *usage.InputTokens != 4 || *usage.OutputTokens != 10 {
		t.Fatal("host accounting lost exact known counters", snapshot, err)
	}
	encoded, _ := json.Marshal(snapshot)
	if strings.Contains(string(encoded), "owned-provider-secret") || strings.Contains(string(encoded), g.Connection().Token) {
		t.Fatal("durable accounting receipt contains credentials")
	}
}

func TestGatewayMissingUsageKeepsAllowanceAndClosedInput(t *testing.T) {
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"id":"response118","model":"coding118","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`)
	}))
	defer provider.Close()
	b := bindingFixture(t, provider.URL+"/v1", 10)
	g, err := Open(Config{Context: context.Background(), RequestID: strings.Repeat("b", 64), Directory: filepath.Join(t.TempDir(), "model"), Binding: b, BaseURL: provider.URL + "/v1", APIKey: "owned-provider-secret"})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	status, _ := callGateway(t, g.Connection(), 0, requestFixture)
	if status != 200 {
		t.Fatal("complete response without usage was invented as a transport failure")
	}
	status, _ = callGateway(t, g.Connection(), 1, requestFixture)
	if status == 200 || calls.Load() != 1 {
		t.Fatal("missing usage refunded the entire output grant")
	}
	s, _ := g.Close()
	u, complete := mc.AggregateEvidenceUsage(b, Evidence(s))
	if complete || u != nil && (u.InputTokens != nil || u.OutputTokens != nil) || s.Records[0].UsageState != "unreported" {
		t.Fatal("missing usage became measured zero or complete billing")
	}
	for name, raw := range map[string]string{"duplicate": `{"model":"coding118","model":"coding118"}`, "parameter": strings.Replace(requestFixture, `"stream":false`, `"temperature":0,"stream":false`, 1), "foreign": strings.Replace(requestFixture, "coding118", "other118", 1)} {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := prepareRequest([]byte(raw), b, 10); err == nil {
				t.Fatal("closed model request widened")
			}
		})
	}
}

func TestGatewayStreamsContentButPersistsBeforeTerminal(t *testing.T) {
	var persisted atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"sse118\",\"model\":\"coding118\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"actual stream\"},\"finish_reason\":null}]}\n\n")
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, "data: {\"id\":\"sse118\",\"model\":\"coding118\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"id\":\"sse118\",\"model\":\"coding118\",\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3}}\n\ndata: [DONE]\n\n")
	}))
	defer provider.Close()
	b := bindingFixture(t, provider.URL+"/v1", 10)
	g, err := Open(Config{Context: context.Background(), RequestID: strings.Repeat("c", 64), Directory: filepath.Join(t.TempDir(), "model"), Binding: b, BaseURL: provider.URL + "/v1", APIKey: "owned-provider-secret", Persist: func(s Snapshot) error {
		if len(s.Records) == 1 && s.Records[0].State == "completed" {
			persisted.Store(true)
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	status, raw := callGateway(t, g.Connection(), 0, strings.Replace(requestFixture, `"stream":false`, `"stream":true`, 1))
	if status != 200 || !strings.Contains(string(raw), "actual stream") || !strings.Contains(string(raw), "[DONE]") || !persisted.Load() {
		t.Fatal("streaming content/terminal did not cross its durable boundary")
	}
	s, _ := g.Close()
	u, complete := mc.AggregateEvidenceUsage(b, Evidence(s))
	if !complete || u == nil || *u.OutputTokens != 3 {
		t.Fatal("streamed counters were not authoritative")
	}
}
