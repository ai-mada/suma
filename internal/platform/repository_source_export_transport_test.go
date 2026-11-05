package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func TestRepositorySourceExportProviderCredentialIsolation(t *testing.T) {
	const raw = `[{"id":"repo","tenant_id":"tenant","project_id":"project","target_aliases":["delivery"],"origin":"http://127.0.0.1:12345","token_env":"CALLBACK","reconcile_token_env":"RECONCILE","source_read_token_env":"READ","source_export_token_env":"EXPORT"}]`
	secrets := map[string]string{"CALLBACK": strings.Repeat("c", 32), "RECONCILE": strings.Repeat("r", 32), "READ": strings.Repeat("m", 32), "EXPORT": strings.Repeat("e", 32)}
	getenv := func(key string) string { return secrets[key] }
	providers, err := LoadRepositoryProviders(raw, getenv)
	if err != nil || len(providers) != 1 || providers[0].SourceExportToken != secrets["EXPORT"] {
		t.Fatal("dedicated source export credential was not configured", err)
	}
	public, err := json.Marshal(providers[0])
	if err != nil || strings.Contains(string(public), secrets["EXPORT"]) {
		t.Fatal("source export credential appeared in the serialized provider configuration", err)
	}
	for _, existing := range []string{"CALLBACK", "RECONCILE", "READ", "MISSING"} {
		if _, err := LoadRepositoryProviders(strings.Replace(raw, `"EXPORT"`, `"`+existing+`"`, 1), getenv); err == nil {
			t.Fatalf("accepted unsafe source export secret name %q", existing)
		}
	}
	providers, err = LoadRepositoryProviders(strings.Replace(raw, `,"source_export_token_env":"EXPORT"`, "", 1), getenv)
	if err != nil || len(providers) != 1 || providers[0].SourceExportToken != "" {
		t.Fatal("source export should remain disabled when its credential is omitted", err)
	}
}

func TestFetchRepositorySourceExportBindsBytesToRequest(t *testing.T) {
	request := ri.RepositorySourceExportRequest{
		Protocol: ri.RepositorySourceExportProtocol, TenantID: "tenant", ProjectID: "project", ProviderID: "repo",
		TargetAlias: "delivery", TargetRef: "refs/heads/delivery", TargetConfigSHA256: strings.Repeat("a", 64),
		ExpectedHead: strings.Repeat("b", 40), ExpectedBase: strings.Repeat("c", 40),
	}
	requestSHA, err := ri.RepositorySourceExportDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	const body = "fixed binary transport fixture\x00\xff"
	cfg := RepositoryProviderConfig{
		ID: "repo", TenantID: "tenant", ProjectID: "project", TargetAliases: []string{"delivery"},
		TokenHash: digest(strings.Repeat("c", 32)), ReconcileToken: strings.Repeat("r", 32),
		SourceReadToken: strings.Repeat("m", 32), SourceExportToken: strings.Repeat("e", 32),
	}
	for _, tc := range []struct {
		name   string
		alter  func(http.Header)
		status int
		want   int
	}{
		{name: "exact"},
		{name: "different request", alter: func(h http.Header) { h.Set("X-Suma-Source-Request-SHA256", strings.Repeat("f", 64)) }, want: 502},
		{name: "wrong body hash", alter: func(h http.Header) { h.Set("X-Suma-Bundle-SHA256", strings.Repeat("f", 64)) }, want: 502},
		{name: "duplicate hash", alter: func(h http.Header) { h.Add("X-Suma-Bundle-SHA256", digest(body)) }, want: 502},
		{name: "wrong content type", alter: func(h http.Header) { h.Set("Content-Type", "application/json") }, want: 502},
		{name: "compressed bytes", alter: func(h http.Header) { h.Set("Content-Encoding", "gzip") }, want: 502},
		{name: "oversized response", alter: func(h http.Header) { h.Set("Content-Length", strconv.FormatInt(cc.MaxBundleBytes+1, 10)) }, want: 502},
		{name: "truncated response", alter: func(h http.Header) { h.Set("Content-Length", strconv.Itoa(len(body)+1)) }, want: 502},
		{name: "stale source", status: 409, want: 409},
		{name: "busy", status: 429, want: 429},
		{name: "stopped", status: 503, want: 503},
		{name: "redirect", status: 307, alter: func(h http.Header) { h.Set("Location", "/redirect-target") }, want: 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var got ri.RepositorySourceExportRequest
				if r.URL.Path != "/source-exports" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+cfg.SourceExportToken || json.NewDecoder(r.Body).Decode(&got) != nil || got != request {
					t.Error("provider received an unexpected source export request")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				h := w.Header()
				h.Set("Content-Type", "application/x-git-bundle")
				h.Set("Content-Length", strconv.Itoa(len(body)))
				h.Set("X-Suma-Bundle-SHA256", digest(body))
				h.Set("X-Suma-Source-Request-SHA256", requestSHA)
				if tc.alter != nil {
					tc.alter(h)
				}
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
				_, _ = w.Write([]byte(body))
			}))
			defer provider.Close()
			configured := cfg
			configured.Origin = provider.URL
			got, err := fetchRepositorySourceExport(context.Background(), configured, request)
			if calls.Load() != 1 {
				t.Fatal("provider request was retried or redirected")
			}
			if tc.want == 0 {
				if err != nil || string(got.Bundle) != body || got.BundleSHA256 != digest(body) || got.RequestSHA256 != requestSHA {
					t.Fatal("exact export was not verified", err)
				}
				return
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Status != tc.want || len(got.Bundle) != 0 {
				t.Fatal("invalid export bytes were accepted or failed with an unexpected status", err)
			}
		})
	}
}
