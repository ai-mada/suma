package platform

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ai-mada/suma/internal/store"
)

func TestTaskConversationMentionsInitialSchemaRejectsPartialBeforeOtherWrites(t *testing.T) {
	for _, shape := range []string{"partial", "view"} {
		t.Run(shape, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "mentions.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			statement := `CREATE TABLE suma_task_conversation_mentions(message_id TEXT,resource_id TEXT)`
			if shape == "view" {
				statement = `CREATE VIEW suma_task_conversation_mentions AS SELECT 'unique original' AS message_id`
			}
			if _, err = db.Exec(statement); err != nil {
				t.Fatal(err)
			}
			s := &Service{DB: db, Dialect: "sqlite", DataDir: t.TempDir()}
			var before, after int
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if err = s.Initialize(ctx); !errors.Is(err, store.ErrInitialSchema) {
				t.Fatal("incompatible mention source was accepted", shape, err)
			}
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&after); err != nil || before != after {
				t.Fatal("rejected initial shape wrote unrelated schema", before, after, err)
			}
		})
	}
}
