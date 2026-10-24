package store

import "testing"

func TestDriverAliasesAndUnsupported(t *testing.T) {
	for driver, want := range map[string]string{"": "sqlite", "sqlite3": "sqlite", "mysql": "oceanbase", "pgx": "postgres", "bogus": ""} {
		if got := Dialect(driver); got != want {
			t.Fatalf("Dialect(%q)=%q", driver, got)
		}
	}
	if _, err := Open("bogus", ""); err == nil {
		t.Fatal("unknown driver accepted")
	}
}
func TestSQLitePragmasAndMemory(t *testing.T) {
	db, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var enabled int
	if err = db.QueryRow("PRAGMA foreign_keys").Scan(&enabled); err != nil || enabled != 1 {
		t.Fatalf("foreign keys %d %v", enabled, err)
	}
	var timeout int
	if err = db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil || timeout < 10000 {
		t.Fatalf("busy timeout %d %v", timeout, err)
	}
}
