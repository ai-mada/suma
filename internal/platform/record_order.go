package platform

import (
	"errors"
	"time"
)

const recordOrderLayout = "2006-01-02T15:04:05.000000000Z"
const recordTimestampInvalid = "A task has an invalid UTC timestamp"

func recordTimestampError() error {
	return &APIError{Status: 409, Code: "task_timestamp_invalid", Message: recordTimestampInvalid}
}

// Current mutable task producers own UTC-Z RFC3339 timestamps. Fractions may
// contain zero through nine digits; equal instants need one fixed-width key.
// Original source fields and immutable evidence are never changed by this read.
func recordOrderKey(source string) (string, error) {
	if len(source) != 20 && (len(source) < 22 || len(source) > 30) {
		return "", recordTimestampError()
	}
	if source[len(source)-1] != 'Z' || source[10] != 'T' {
		return "", recordTimestampError()
	}
	if len(source) > 20 {
		if source[19] != '.' {
			return "", recordTimestampError()
		}
		for index := 20; index < len(source)-1; index++ {
			if source[index] < '0' || source[index] > '9' {
				return "", recordTimestampError()
			}
		}
	}
	parsed, err := time.Parse(time.RFC3339Nano, source)
	if err != nil {
		return "", recordTimestampError()
	}
	return parsed.UTC().Format(recordOrderLayout), nil
}

func validRecordOrderKey(value string) bool {
	key, err := recordOrderKey(value)
	return err == nil && len(value) == 30 && key == value
}

// This expression allows only server-owned updated_at/created_at columns. Exact string
// padding preserves nanoseconds; SQLite date functions/floats would lose them.
// Unsupported source values are validated on bounded returned/lookahead rows,
// never excluded from counts or silently replaced with another timestamp.
func recordOrderKeySQL(dialect, column string) (string, error) {
	if column != "updated_at" && column != "created_at" {
		return "", errors.New("Unsupported task timestamp ordering column")
	}
	fraction := `CASE WHEN SUBSTR(` + column + `,20,1)='.' AND LENGTH(` + column + `)>=22 THEN SUBSTR(` + column + `,21,LENGTH(` + column + `)-21) ELSE '' END`
	switch dialect {
	case "sqlite", "postgres":
		return `SUBSTR(` + column + `,1,19) || '.' || SUBSTR((` + fraction + `) || '000000000',1,9) || 'Z'`, nil
	case "oceanbase":
		return `CONCAT(SUBSTR(` + column + `,1,19),'.',SUBSTR(CONCAT(` + fraction + `,'000000000'),1,9),'Z')`, nil
	default:
		return "", errors.New("Unsupported database for exact task timestamp ordering")
	}
}
