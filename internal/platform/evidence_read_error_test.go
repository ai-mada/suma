package platform

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestEvidenceReadInfrastructureErrorsAreNotInvalidAcceptance(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		name string
		read func() error
	}{
		{"task", func() error {
			return f.s.checkTaskEvidenceReadTx(ctx, tx, f.user, f.project, "unreadable-task", map[string]bool{}, map[string]bool{}, false)
		}},
		{"run", func() error {
			return f.s.checkEvidenceReadTx(ctx, tx, f.user.TenantID, f.project, "unreadable-run", 0, map[string]bool{}, false)
		}},
	} {
		t.Run(check.name, func(t *testing.T) {
			if err := check.read(); !errors.Is(err, sql.ErrTxDone) {
				t.Fatalf("infrastructure error was replaced by invalid acceptance: %T %v", err, err)
			}
		})
	}
}
