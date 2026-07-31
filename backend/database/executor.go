package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// queryTimeout caps user-initiated ad-hoc queries / DDL on a single
// connection. Without this the UI cannot cancel a stuck query — the user
// has to kill the whole app. 30s is generous enough for normal browsing
// and short enough that an accidental cross-join SELECT will not freeze the
// result grid for half a minute.
var queryTimeout = 30 * time.Second

type QueryResultColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type QueryResult struct {
	Columns []QueryResultColumn `json:"columns"`
	Rows    []map[string]any    `json:"rows"`
}

type ExecResult struct {
	Affected     int64 `json:"affected"`
	LastInsertID int64 `json:"lastInsertId"`
}

func ExecuteQuery(p Provider, db *sql.DB, dbName, sqlStr string) (*QueryResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := p.PrepareExec(conn, dbName); err != nil {
		return nil, err
	}

	rows, err := conn.QueryContext(ctx, sqlStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []map[string]any
	for rows.Next() {
		values := make([]any, len(cols))
		valuePtrs := make([]any, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = scanToAny(values[i])
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	columns := make([]QueryResultColumn, 0, len(cols))
	for _, c := range cols {
		columns = append(columns, QueryResultColumn{Name: c, Type: ""})
	}

	if len(result) == 0 {
		return &QueryResult{Columns: columns, Rows: []map[string]any{}}, nil
	}
	return &QueryResult{Columns: columns, Rows: result}, nil
}

func ExecuteStatement(p Provider, db *sql.DB, dbName, sqlStr string) (*ExecResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := p.PrepareExec(conn, dbName); err != nil {
		return nil, err
	}

	result, err := conn.ExecContext(ctx, sqlStr)
	if err != nil {
		return nil, err
	}

	affected, _ := result.RowsAffected()
	lastID, _ := result.LastInsertId()

	return &ExecResult{Affected: affected, LastInsertID: lastID}, nil
}

// QueryResultToJSON serializes a QueryResult to JSON bytes.
func QueryResultToJSON(qr *QueryResult) ([]byte, error) {
	return json.Marshal(qr)
}
