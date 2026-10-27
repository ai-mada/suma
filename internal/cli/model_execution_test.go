package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelExecutionOptionsCLI(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/api/v1/model-execution/options" || r.URL.Query().Get("project_id") != "project-one" || r.URL.Query().Get("tool_profile_id") != "tool-one" || r.URL.Query().Get("expected_tool_version") != "2" {
			t.Error("model read scope changed", r.Method, r.URL)
		}
		_, _ = w.Write([]byte(`{"items":[],"supported":false}`))
	}))
	defer s.Close()
	base := []string{"--control", s.URL, "--project", "project-one", "models", "options"}
	var out, stderr strings.Builder
	if code := Execute(context.Background(), append(append([]string{}, base...), "--tool", "tool-one", "--expected-tool-version", "2"), strings.NewReader(""), &out, &stderr); code != 0 || calls != 1 {
		t.Fatal(code, stderr.String(), calls)
	}
	for _, args := range [][]string{{}, {"--tool", "tool-one", "--expected-tool-version", "2e0"}, {"--tool", "tool-one", "--expected-tool-version", "2", "--tool", "other"}, {"--tool", "tool-one", "--expected-tool-version", "2", "--confirm"}} {
		out.Reset()
		stderr.Reset()
		if code := Execute(context.Background(), append(append([]string{}, base...), args...), strings.NewReader(""), &out, &stderr); code != 2 {
			t.Fatal(code, stderr.String())
		}
	}
	if calls != 1 {
		t.Fatal("invalid model request reached API", calls)
	}
}
