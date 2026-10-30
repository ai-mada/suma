package platform

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/store"
)

func TestTaskConversationKnowledgeInitialSchemaRejectsPartialViewAndMissingOrdinalUniqueness(t *testing.T) {
	for _, shape := range []string{"partial", "view", "ordinal_not_unique"} {
		t.Run(shape, func(t *testing.T) {
			db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "conversation-knowledge.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			ddl := `CREATE TABLE suma_task_conversation_knowledge(message_id TEXT,knowledge_id TEXT)`
			if shape == "view" {
				ddl = `CREATE VIEW suma_task_conversation_knowledge AS SELECT 'original unique row' AS message_id`
			}
			if shape == "ordinal_not_unique" {
				ddl = strings.Replace(taskConversationKnowledgeDDL, ",UNIQUE(tenant_id,project_id,message_id,ordinal)", "", 1)
			}
			if _, err = db.Exec(ddl); err != nil {
				t.Fatal(err)
			}
			s := &Service{DB: db, Dialect: "sqlite", DataDir: t.TempDir()}
			var before, after int
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if err = s.Initialize(context.Background()); !errors.Is(err, store.ErrInitialSchema) {
				t.Fatal("incompatible initial relation accepted", shape, err)
			}
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&after); err != nil || after != before {
				t.Fatal("rejected relation changed other initial schema", shape, before, after, err)
			}
		})
	}
}
