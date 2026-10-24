package dag

import (
	"encoding/json"
	"sort"
	"testing"
)

// Test workers declare capabilities for the actual published fixture commands,
// including templates. This helper is test-only and never grants production
// workers a wildcard or changes the compiler's fail-closed rules.
func testCapabilities(t *testing.T, e *Engine) []string {
	t.Helper()
	rows, err := e.db.Query(`SELECT spec FROM dag_nodes`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	var add func(Node)
	add = func(n Node) {
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
				var child Node
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
		var n Node
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
