package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// EnsureIndex installs only a code-owned, ordinary ascending index. Existing
// names with another definition fail explicitly; they are never dropped.
// Identifiers cannot come from SQL parameters, so every identifier is restricted.
func EnsureIndex(ctx context.Context, db *sql.DB, dialect, name, table string, columns ...string) error {
	for _, v := range append([]string{name, table}, columns...) {
		if len(v) == 0 || len(v) > 63 {
			return fmt.Errorf("invalid index identifier")
		}
		for i, c := range v {
			if !(c >= 'a' && c <= 'z' || c == '_' || i > 0 && c >= '0' && c <= '9') {
				return fmt.Errorf("invalid index identifier")
			}
		}
	}
	if len(columns) == 0 || len(columns) > 8 {
		return fmt.Errorf("invalid index column count")
	}
	read := func() ([]string, error) {
		var q string
		var args []any
		switch dialect {
		case "sqlite":
			q = `SELECT CASE WHEN p.desc<>0 OR l.partial<>0 THEN NULL ELSE p.name END FROM pragma_index_list(?) l,pragma_index_xinfo(l.name) p WHERE l.name=? AND p.key=1 ORDER BY p.seqno`
			args = []any{table, name}
		case "postgres":
			q = `SELECT CASE WHEN i.indisvalid AND i.indpred IS NULL AND i.indexprs IS NULL AND am.amname='btree' AND k.position<=i.indnkeyatts AND (i.indoption::smallint[])[k.position-1]=0 THEN a.attname ELSE NULL END FROM pg_index i JOIN pg_class x ON x.oid=i.indexrelid JOIN pg_am am ON am.oid=x.relam JOIN pg_class t ON t.oid=i.indrelid JOIN pg_namespace n ON n.oid=t.relnamespace CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY k(attnum,position) LEFT JOIN pg_attribute a ON a.attrelid=t.oid AND a.attnum=k.attnum WHERE n.nspname=current_schema() AND t.relname=$1 AND x.relname=$2 ORDER BY k.position`
			args = []any{table, name}
		case "oceanbase":
			q = `SELECT CASE WHEN sub_part IS NULL AND index_type='BTREE' AND collation='A' THEN column_name ELSE NULL END FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND index_name=? ORDER BY seq_in_index`
			args = []any{table, name}
		default:
			return nil, fmt.Errorf("unsupported index dialect")
		}
		rows, err := db.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var found []string
		for rows.Next() {
			var column sql.NullString
			if err = rows.Scan(&column); err != nil {
				return nil, err
			}
			found = append(found, column.String)
		}
		return found, rows.Err()
	}
	verify := func(found []string) error {
		if strings.Join(found, ",") != strings.Join(columns, ",") {
			return fmt.Errorf("index %s definition differs from expected columns", name)
		}
		return nil
	}
	found, err := read()
	if err != nil {
		return err
	}
	if len(found) > 0 {
		return verify(found)
	}
	prefix := "CREATE INDEX "
	if dialect != "oceanbase" {
		prefix += "IF NOT EXISTS "
	}
	_, createErr := db.ExecContext(ctx, prefix+name+" ON "+table+" ("+strings.Join(columns, ",")+")")
	// A second process may create the same index after our read. Re-read its
	// definition rather than hiding errors or relying on vendor error strings.
	found, err = read()
	if err != nil {
		return err
	}
	if len(found) == 0 && createErr != nil {
		return createErr
	}
	return verify(found)
}
