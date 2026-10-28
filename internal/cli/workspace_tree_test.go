package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkspaceTreeCLIPageRemainsScopedRead(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		if r.Method != "GET" || r.URL.Path != "/api/v1/workspace/tree" || q.Get("project_id") != "project-one" || q.Get("directory") != "src/示例" || q.Get("limit") != "20" || q.Get("before") != "owned-cursor" {
			t.Error("directory read changed scope", r.Method, r.URL)
		}
		_, _ = w.Write([]byte(`{"entries":[],"next_cursor":""}`))
	}))
	defer server.Close()
	base := []string{"--control", server.URL, "--project", "project-one", "workspace", "files"}
	var output, stderr strings.Builder
	if code := Execute(context.Background(), append(append([]string{}, base...), "--directory", "src/示例", "--limit", "20", "--before", "owned-cursor"), strings.NewReader(""), &output, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	for _, args := range [][]string{{"--directory", "../outside"}, {"--directory", ".env"}, {"--limit", "201"}, {"--limit", "0"}, {"--before", "line\nbreak"}, {"unexpected"}} {
		stderr.Reset()
		if code := Execute(context.Background(), append(append([]string{}, base...), args...), strings.NewReader(""), &output, &stderr); code != 2 {
			t.Fatal(code, stderr.String())
		}
	}
	if calls != 1 {
		t.Fatal("invalid directory parameters reached the server", calls)
	}
}
