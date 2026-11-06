package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestKnowledgeHelixSpecImport(t *testing.T) {
	for _, name := range []string{"SUMA_TOKEN", "SUMA_EXPECTED_USER", "SUMA_EXPECTED_ORGANIZATION"} {
		t.Setenv(name, "")
	}
	var requests atomic.Int32
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer control.Close()
	dir := t.TempDir()
	run := func(locale string, global []string, args ...string) (int, string, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		flags := append([]string{"--control", control.URL, "--language", locale}, global...)
		code := Execute(context.Background(), append(flags, args...), nil, &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}
	write := func(name, raw string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	body := "\n# 调度与恢复\n\n\x60\x60\x60sh\nverify-run\n\x60\x60\x60\n"
	filename := write("SPEC.MD", "\uFEFF"+strings.ReplaceAll(body, "\n", "\r\n"))
	code, out, stderr := run("en-US", []string{"--project", "project-a"}, "knowledge", "import-helix-spec", "--file", filename)
	var draft struct {
		Kind            string            `json:"kind"`
		Title           string            `json:"title"`
		Summary         string            `json:"summary"`
		Content         string            `json:"content"`
		ProjectID       string            `json:"project_id"`
		ExpectedVersion *int              `json:"expected_version"`
		Sources         []json.RawMessage `json:"sources"`
	}
	if code != 0 || stderr != "" || json.Unmarshal([]byte(out), &draft) != nil ||
		draft.Kind != "spec" || draft.Title != "调度与恢复" || draft.Summary != "" || draft.Content != body ||
		draft.ProjectID != "project-a" || draft.ExpectedVersion == nil || *draft.ExpectedVersion != 0 ||
		draft.Sources == nil || len(draft.Sources) != 0 || strings.Contains(out, "idempotency_key") {
		t.Fatalf("unexpected preview: code %d, stdout %q, stderr %q", code, out, stderr)
	}

	for _, tc := range []struct {
		name, file, body, errorKey string
	}{
		{"other filename", "tasks.md", "# valid", "invalid"},
		{"empty", "spec.md", " \u0085\n", "invalid"},
		{"invalid UTF-8", "spec.md", "# bad\xff", "invalid"},
		{"NUL", "spec.md", "# bad\x00", "invalid"},
		{"draft size", "spec.md", strings.Repeat("甲", 5334), "too_large"},
		{"file size", "spec.md", strings.Repeat("\r\n", maxKnowledgeMarkdownBytes/2+1), "too_large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := write(tc.file, tc.body)
			code, out, stderr := run("zh-CN", nil, "knowledge", "import-helix-spec", "--file", path)
			if code != 2 || out != "" || !strings.Contains(stderr, "cli.knowledge.import_helix_spec_"+tc.errorKey) || strings.Contains(stderr, path) {
				t.Fatalf("unexpected error: code %d, stdout %q, stderr %q", code, out, stderr)
			}
		})
	}
	filename = write("spec.md", "# "+strings.Repeat("甲", 67)+"\nkeep this text")
	code, out, stderr = run("en-US", nil, "knowledge", "import-helix-spec", "--file", filename)
	var fields map[string]json.RawMessage
	if code != 0 || stderr != "" || json.Unmarshal([]byte(out), &fields) != nil ||
		string(fields["title"]) != `"HelixHarness spec"` || fields["project_id"] != nil {
		t.Fatalf("unexpected unbound fallback: code %d, stdout %q, stderr %q", code, out, stderr)
	}
	if requests.Load() != 0 {
		t.Fatalf("preview made %d HTTP requests", requests.Load())
	}
}
