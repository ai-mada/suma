package platform

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/store"
)

func TestTaskReviewRequestsInitialSchemaRejectsPartialAndMissingUniquenessBeforeWrites(t *testing.T) {
	for _, shape := range []string{"requests_only", "pending_only", "view", "nonunique_pending"} {
		t.Run(shape, func(t *testing.T) {
			db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "review-requests.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			statements := []string{taskReviewRequestsDDL}
			switch shape {
			case "pending_only":
				statements = []string{taskReviewPendingDDL}
			case "view":
				statements = []string{`CREATE VIEW suma_task_review_requests AS SELECT 'unique original source' AS id`, taskReviewPendingDDL}
			case "nonunique_pending":
				statements = []string{taskReviewRequestsDDL, strings.ReplaceAll(taskReviewPendingDDL, " UNIQUE", "")}
			}
			for _, statement := range statements {
				if _, err = db.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			s := &Service{DB: db, Dialect: "sqlite", DataDir: t.TempDir()}
			var before, after int
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if err = s.Initialize(context.Background()); !errors.Is(err, store.ErrInitialSchema) {
				t.Fatal("incompatible responsibility domain was accepted", shape, err)
			}
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&after); err != nil || before != after {
				t.Fatal("initial schema refusal wrote unrelated schema", shape, before, after, err)
			}
		})
	}
}
