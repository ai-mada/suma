package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUISelectsProjectAndReadsOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Fatalf("TUI wrote data: %s", r.Method)
		}
		switch r.URL.Path {
		case "/api/v1/projects":
			w.Write([]byte(`{"items":[{"id":"project-two","name":"Second project"}],"total":1}`))
		case "/api/v1/requirements":
			if r.URL.Query().Get("project_id") != "project-two" {
				t.Errorf("wrong project: %s", r.URL)
			}
			w.Write([]byte(`{"items":[{"id":"req-1","title":"Actual requirement"}],"total":1}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c, err := newClient(server.URL, "", "token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(context.Background(), c)
	updated, _ := m.Update(m.Init()())
	m = updated.(model)
	if !strings.Contains(m.View(), "Second project") {
		t.Fatal(m.View())
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	updated, _ = m.Update(cmd())
	m = updated.(model)
	if m.client.project != "project-two" || !strings.Contains(m.View(), "Actual requirement") {
		t.Fatal(m.View())
	}
}

func TestTUIIgnoresObsoleteResponses(t *testing.T) {
	m := model{generation: 2, loading: true}
	updated, _ := m.Update(loaded{generation: 1, raw: []byte(`{"items":[{"name":"stale"}]}`)})
	current := updated.(model)
	if !current.loading || len(current.items) != 0 {
		t.Fatal("obsolete response replaced current view")
	}
}

func TestTUIUsesOpaqueCursorAndReturnsToPreviousPage(t *testing.T) {
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("cursor"))
		if r.URL.Query().Has("offset") {
			t.Errorf("offset pagination is unsupported: %s", r.URL)
		}
		if r.URL.Query().Get("cursor") == "opaque+/next=" {
			w.Write([]byte(`{"items":[{"id":"p2","name":"Next"}],"total":26,"next_cursor":""}`))
		} else {
			w.Write([]byte(`{"items":[{"id":"p1","name":"First"}],"total":26,"next_cursor":"opaque+/next="}`))
		}
	}))
	defer server.Close()
	c, err := newClient(server.URL, "", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(context.Background(), c)
	updated, _ := m.Update(m.Init()())
	m = updated.(model)
	for _, key := range []string{"n", "p"} {
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m = updated.(model)
		if cmd == nil {
			t.Fatalf("no page fetch for %s", key)
		}
		updated, _ = m.Update(cmd())
		m = updated.(model)
	}
	if len(queries) != 3 || queries[0] != "" || queries[1] != "opaque+/next=" || queries[2] != "" {
		t.Fatalf("wrong cursor history: %v", queries)
	}
}
