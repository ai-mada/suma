// Package store opens the supported SQL drivers. Schema ownership belongs to each service.
package store

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
	"strings"
)

func Dialect(driver string) string {
	switch strings.ToLower(driver) {
	case "", "sqlite", "sqlite3":
		return "sqlite"
	case "mysql", "oceanbase":
		return "oceanbase"
	case "postgres", "postgresql", "pgx":
		return "postgres"
	}
	return ""
}
func Open(driver, dsn string) (*sql.DB, error) {
	dialect := Dialect(driver)
	name := dialect
	switch dialect {
	case "sqlite":
		if dsn == "" {
			dsn = "suma.db"
		}
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		dsn += sep + "_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	case "oceanbase":
		name = "mysql"
	case "postgres":
		name = "pgx"
	default:
		return nil, fmt.Errorf("unsupported database driver %q", driver)
	}
	db, err := sql.Open(name, dsn)
	if err != nil {
		return nil, err
	}
	if dialect == "sqlite" {
		db.SetMaxOpenConns(1)
	}
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
