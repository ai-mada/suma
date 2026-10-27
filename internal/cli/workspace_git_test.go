package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkspaceGitCLIOnlyScopedReads(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Query().Get("project_id") != "project-one" {
			t.Error(r.Method, r.URL)
		}
		if calls == 1 && r.URL.Path != "/api/v1/workspace/git" {
			t.Error(r.URL)
		}
		if calls == 2 && (r.URL.Path != "/api/v1/workspace/git/diff" || r.URL.Query().Get("path") != "src/你好.ts" || r.URL.Query().Get("source") != "index") {
			t.Error(r.URL)
		}
		_, _ = w.Write([]byte(`{"available":false,"files":[]}`))
	}))
	defer server.Close()
	base := []string{"--control", server.URL, "--project", "project-one", "workspace"}
	var output, stderr strings.Builder
	for _, args := range [][]string{{"git"}, {"diff", "--path", "src/你好.ts", "--source", "index"}} {
		if code := Execute(context.Background(), append(append([]string{}, base...), args...), strings.NewReader(""), &output, &stderr); code != 0 {
			t.Fatal(code, stderr.String())
		}
	}
	for _, args := range [][]string{{"git", "--confirm"}, {"diff"}, {"diff", "--path", "../secrets"}, {"diff", "--path", "x", "--source", "other"}, {"diff", "--path", ".env"}, {"diff", "--path", "/absolute"}} {
		output.Reset()
		stderr.Reset()
		if code := Execute(context.Background(), append(append([]string{}, base...), args...), strings.NewReader(""), &output, &stderr); code != 2 {
			t.Fatal(code, stderr.String())
		}
	}
	if calls != 2 {
		t.Fatal("Invalid input reached transport", calls)
	}
}
