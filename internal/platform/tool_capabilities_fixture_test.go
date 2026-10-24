package platform

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

// Unit fixtures declare their actual configured commands; production workers
// never use this helper or an unrestricted capability shortcut.
func toolFixtureCapabilities(t *testing.T, f *fixture) []string {
	t.Helper()
	rows, err := f.s.DB.Query(`SELECT spec FROM dag_nodes`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	var add func(dag.Node)
	add = func(n dag.Node) {
		if n.Type == "command" || n.Type == "agent" || n.Type == "test" {
			for _, c := range n.Command {
				seen["command:"+c] = true
			}
		} else {
			seen[n.Type] = true
		}
		for _, key := range []string{"template", "compensation"} {
			if v, ok := n.Config[key]; ok {
				b, _ := json.Marshal(v)
				var child dag.Node
				if json.Unmarshal(b, &child) == nil {
					add(child)
				}
			}
		}
	}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var n dag.Node
		if err = json.Unmarshal([]byte(raw), &n); err != nil {
			t.Fatal(err)
		}
		add(n)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
