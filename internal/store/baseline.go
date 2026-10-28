package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// InitialDataContract identifies the one supported initial persistent contract,
// including JSON payload semantics. It is independent of the product version.
const InitialDataContract = "suma-initial-gateway-v1"

// ErrInitialSchema prevents initialization from interpreting unregistered data as
// a current installation. Initialization never upgrades, resets or deletes data.
var ErrInitialSchema = errors.New("database does not declare the current suma initial data contract; preserve it and use a separate empty database")

// InitializeBaseline declares a component's current schema before its idempotent
// DDL runs. It is a schema declaration, not an upgrade ledger or readiness flag.
// A restart may finish interrupted current DDL; existing unregistered domain
// tables are rejected before any write, so no historical shape is guessed.
func InitializeBaseline(ctx context.Context, db *sql.DB, dialect, component, ownedPrefix, version string) error {
	if component == "" || ownedPrefix == "" || version == "" {
		return fmt.Errorf("initial schema declaration is incomplete")
	}
	if version != InitialDataContract || component != "platform" && component != "dag" {
		return fmt.Errorf("%w: unsupported component or data contract", ErrInitialSchema)
	}
	placeholder := "?"
	if dialect == "postgres" {
		placeholder = "$1"
	}
	read := func() (string, error) {
		var found string
		err := db.QueryRowContext(ctx, "SELECT version FROM suma_schema_baselines WHERE component="+placeholder, component).Scan(&found)
		return found, err
	}
	declared := func() (bool, error) {
		present, err := baselineTableExists(ctx, db, dialect, "suma_schema_baselines")
		if err != nil || !present {
			return false, err
		}
		// Validate the whole shared contract before either component runs DDL.
		// A current platform marker cannot mask an incompatible DAG or unknown
		// namespace. These declarations are never rewritten during startup.
		rows, err := db.QueryContext(ctx, `SELECT component,version FROM suma_schema_baselines`)
		if err != nil {
			return false, err
		}
		for rows.Next() {
			var name, contract string
			if err = rows.Scan(&name, &contract); err != nil {
				rows.Close()
				return false, err
			}
			if name != "platform" && name != "dag" || contract != InitialDataContract {
				rows.Close()
				return false, fmt.Errorf("%w: component %s", ErrInitialSchema, name)
			}
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return false, err
		}
		if err = rows.Close(); err != nil {
			return false, err
		}
		found, err := read()
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if found != version {
			return false, fmt.Errorf("%w: component %s", ErrInitialSchema, component)
		}
		return true, nil
	}
	if ok, err := declared(); err != nil || ok {
		return err
	}
	present, err := baselineOwnedTablesExist(ctx, db, dialect, ownedPrefix)
	if err != nil {
		return err
	}
	if present {
		// Another current initializer may have declared the schema and created
		// its first table since our initial read. Re-read before rejecting it.
		if ok, err := declared(); err != nil || ok {
			return err
		}
		return fmt.Errorf("%w: component %s", ErrInitialSchema, component)
	}
	if _, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS suma_schema_baselines(component VARCHAR(80) PRIMARY KEY,version VARCHAR(30) NOT NULL)`); err != nil {
		return err
	}
	query := `INSERT INTO suma_schema_baselines(component,version) VALUES(?,?) ON CONFLICT(component) DO NOTHING`
	if dialect == "postgres" {
		query = `INSERT INTO suma_schema_baselines(component,version) VALUES($1,$2) ON CONFLICT(component) DO NOTHING`
	} else if dialect == "oceanbase" {
		query = `INSERT IGNORE INTO suma_schema_baselines(component,version) VALUES(?,?)`
	}
	if _, err = db.ExecContext(ctx, query, component, version); err != nil {
		return err
	}
	// A concurrent initializer may have declared another schema. Verify the
	// durable winner instead of treating an ignored insert as agreement.
	if ok, err := declared(); err != nil || ok {
		return err
	}
	return fmt.Errorf("%w: component %s", ErrInitialSchema, component)
}

// Inspect the entire component namespace, including interrupted initializations
// that created only a non-anchor table. The shared declarations are metadata,
// so they must not count as existing platform business tables.
func baselineOwnedTablesExist(ctx context.Context, db *sql.DB, dialect, prefix string) (bool, error) {
	var query string
	switch dialect {
	case "sqlite":
		query = `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND substr(name,1,?)=? AND name<>'suma_schema_baselines'`
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_type='BASE TABLE' AND left(table_name,$1)=$2 AND table_name<>'suma_schema_baselines'`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' AND LEFT(table_name,?)=? AND table_name<>'suma_schema_baselines'`
	default:
		return false, fmt.Errorf("unsupported initial schema dialect %q", dialect)
	}
	var count int
	err := db.QueryRowContext(ctx, query, len(prefix), prefix).Scan(&count)
	return count > 0, err
}

func baselineTableExists(ctx context.Context, db *sql.DB, dialect, table string) (bool, error) {
	var query string
	switch dialect {
	case "sqlite":
		query = `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_type='BASE TABLE' AND table_name=$1`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' AND table_name=?`
	default:
		return false, fmt.Errorf("unsupported initial schema dialect %q", dialect)
	}
	var count int
	err := db.QueryRowContext(ctx, query, table).Scan(&count)
	return count > 0, err
}
