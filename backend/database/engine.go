package database

import (
	"database/sql"
	"fmt"
	"strings"
)

// parseParams parses "key1=val1&key2=val2" into a map.
func parseParams(s string) map[string]string {
	m := make(map[string]string)
	for s != "" {
		var pair string
		if before, after, found := strings.Cut(s, "&"); found {
			pair, s = before, after
		} else {
			pair, s = s, ""
		}
		k, v, _ := strings.Cut(pair, "=")
		if k != "" {
			m[k] = v
		}
	}
	return m
}

// BuildDSN builds a DSN string from connection config fields.
func BuildDSN(dbType, host, user, password, dbName, extraParams string, port int) (string, error) {
	p, err := NewProvider(dbType)
	if err != nil {
		return "", err
	}
	return p.DSN(host, port, user, password, dbName, parseParams(extraParams)), nil
}

// NewDB opens a database/sql connection for the given database type and DSN.
func NewDB(dbType, dsn string) (*sql.DB, error) {
	p, err := NewProvider(dbType)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(p.DriverName(), dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dbType, err)
	}
	return db, nil
}

// queryStrings executes a SQL query and returns all rows as []map[string]string.
func queryStrings(db *sql.DB, query string, args ...any) ([]map[string]string, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []map[string]string
	for rows.Next() {
		values := make([]any, len(cols))
		valuePtrs := make([]any, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}
		row := make(map[string]string, len(cols))
		for i, col := range cols {
			row[col] = scanToString(values[i])
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// queryAny is like queryStrings but preserves nil values for proper NULL handling.
// Returns rows, column names in SQL order, and error.
func queryAny(db *sql.DB, query string, args ...any) ([]map[string]any, []string, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}

	var result []map[string]any
	for rows.Next() {
		values := make([]any, len(cols))
		valuePtrs := make([]any, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, nil, err
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = scanToAny(values[i])
		}
		result = append(result, row)
	}
	return result, cols, rows.Err()
}

func scanToAny(v any) any {
	if v == nil {
		return nil
	}
	switch s := v.(type) {
	case []byte:
		return string(s)
	default:
		return v
	}
}

func scanToString(v any) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case []byte:
		return string(s)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// toAnySlice converts []string to []any for IN clauses etc.
func toAnySlice(items []string) []any {
	res := make([]any, len(items))
	for i, item := range items {
		res[i] = item
	}
	return res
}

// buildPlaceholders returns "?,?,?" for the given count.
func buildPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}
