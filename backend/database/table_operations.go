package database

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

type TableFilter struct {
	Column string `json:"column"`
	Op     string `json:"op"` // contains, equals, startsWith, isNull, isNotNull
	Value  string `json:"value"`
}

type TableSort struct {
	Column string `json:"column"`
	Desc   bool   `json:"desc"`
}

type BrowseOptions struct {
	Filters []TableFilter `json:"filters"`
	Sorts   []TableSort   `json:"sorts"`
	Limit   int           `json:"limit"`
	Offset  int           `json:"offset"`
}

type RowUpdate struct {
	Set   map[string]any `json:"set"`
	Where map[string]any `json:"where"`
}

type CSVColumnMapping struct {
	SourceIndex int    `json:"sourceIndex"`
	Target      string `json:"target"`
}

type CSVPreview struct {
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}

func browseTableColumns(p Provider, db *sql.DB, dbName, tableName string) (map[string]bool, error) {
	schema, err := p.GetTableSchema(db, dbName, tableName)
	if err != nil {
		return nil, err
	}
	if schema == nil || len(schema.Columns) == 0 {
		return nil, fmt.Errorf("table %q has no discoverable columns", tableName)
	}
	cols := make(map[string]bool, len(schema.Columns))
	for _, col := range schema.Columns {
		cols[col.Name] = true
	}
	return cols, nil
}

func requireColumn(cols map[string]bool, name string) error {
	if !cols[name] {
		return fmt.Errorf("unknown column %q", name)
	}
	return nil
}

func placeholder(driver string, index int) string {
	switch driver {
	case "postgres":
		return fmt.Sprintf("$%d", index)
	case "sqlserver":
		return fmt.Sprintf("@p%d", index)
	case "oracle":
		return fmt.Sprintf(":%d", index)
	default:
		return "?"
	}
}

func textCast(driver, quotedColumn string) string {
	switch driver {
	case "mysql":
		return "CAST(" + quotedColumn + " AS CHAR)"
	case "sqlserver":
		return "CAST(" + quotedColumn + " AS NVARCHAR(MAX))"
	case "oracle":
		return "CAST(" + quotedColumn + " AS VARCHAR2(4000))"
	default:
		return "CAST(" + quotedColumn + " AS TEXT)"
	}
}

func likeLiteral(s string) string {
	s = strings.ReplaceAll(s, "!", "!!")
	s = strings.ReplaceAll(s, "%", "!%")
	return strings.ReplaceAll(s, "_", "!_")
}

func buildBrowseSQL(p Provider, tableName string, cols map[string]bool, opts BrowseOptions) (string, []any, error) {
	if opts.Limit <= 0 || opts.Limit > 1000 || opts.Offset < 0 || len(opts.Filters) > 20 || len(opts.Sorts) > 10 {
		return "", nil, errors.New("invalid browse options")
	}
	var args []any
	var predicates []string
	for _, f := range opts.Filters {
		if err := requireColumn(cols, f.Column); err != nil {
			return "", nil, err
		}
		col := p.Quote(f.Column)
		switch f.Op {
		case "isNull":
			predicates = append(predicates, col+" IS NULL")
		case "isNotNull":
			predicates = append(predicates, col+" IS NOT NULL")
		case "equals":
			args = append(args, f.Value)
			predicates = append(predicates, col+" = "+placeholder(p.DriverName(), len(args)))
		case "contains", "startsWith":
			value := likeLiteral(f.Value)
			if f.Op == "contains" {
				value = "%" + value + "%"
			} else {
				value += "%"
			}
			args = append(args, value)
			predicates = append(predicates, "LOWER("+textCast(p.DriverName(), col)+") LIKE LOWER("+placeholder(p.DriverName(), len(args))+") ESCAPE '!'")
		default:
			return "", nil, fmt.Errorf("unsupported filter operator %q", f.Op)
		}
	}
	var order []string
	seen := map[string]bool{}
	for _, s := range opts.Sorts {
		if err := requireColumn(cols, s.Column); err != nil {
			return "", nil, err
		}
		if seen[s.Column] {
			return "", nil, fmt.Errorf("duplicate sort column %q", s.Column)
		}
		seen[s.Column] = true
		direction := " ASC"
		if s.Desc {
			direction = " DESC"
		}
		order = append(order, p.Quote(s.Column)+direction)
	}
	query := "SELECT * FROM " + p.Quote(tableName)
	if len(predicates) > 0 {
		query += " WHERE " + strings.Join(predicates, " AND ")
	}
	if len(order) > 0 {
		query += " ORDER BY " + strings.Join(order, ", ")
	} else if p.DriverName() == "sqlserver" {
		query += " ORDER BY (SELECT NULL)"
	}
	// Fetch one extra row to determine whether a next page exists.
	limit := opts.Limit + 1
	switch p.DriverName() {
	case "sqlserver":
		query += fmt.Sprintf(" OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", opts.Offset, limit)
	case "oracle":
		// OFFSET ... FETCH NEXT requires 12c+; ROWNUM wrapping also works on
		// 11g. The synthetic row-number column is stripped after the scan.
		query = oraclePagingWrap(query, opts.Offset, limit)
	default:
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, opts.Offset)
	}
	return query, args, nil
}

// oraclePagingColumn is the synthetic row-number column added for Oracle
// pagination and stripped from the result before display.
const oraclePagingColumn = "__RN__"

// oraclePagingWrap paginates a complete SELECT with ROWNUM. OFFSET ... FETCH
// NEXT requires Oracle 12c+, so 11g needs this wrap instead; it works on
// modern releases as well. stripOraclePagingColumn removes the synthetic
// column from the result.
func oraclePagingWrap(query string, offset, limit int) string {
	return fmt.Sprintf(
		`SELECT * FROM (SELECT q_.*, ROWNUM %s FROM (%s) q_ WHERE ROWNUM <= %d) WHERE %s > %d`,
		`"`+oraclePagingColumn+`"`, query, offset+limit, `"`+oraclePagingColumn+`"`, offset)
}

// stripOraclePagingColumn removes the synthetic pagination column from a
// result grid (see buildBrowseSQL's oracle branch).
func stripOraclePagingColumn(result *QueryResult) {
	cols := make([]QueryResultColumn, 0, len(result.Columns))
	for _, c := range result.Columns {
		if !strings.EqualFold(c.Name, oraclePagingColumn) {
			cols = append(cols, c)
		}
	}
	result.Columns = cols
	for _, row := range result.Rows {
		delete(row, oraclePagingColumn)
	}
}

func BrowseTable(p Provider, db *sql.DB, dbName, tableName string, opts BrowseOptions) (*QueryResult, error) {
	// The column map is only needed to validate filter/sort columns. The default
	// browse (no filters, no sorts) does not reference it, so skip the expensive
	// GetTableSchema round-trip entirely — the frontend already loaded the schema
	// when the table was opened (§loadSchema). This avoids duplicate schema queries
	// that prolong the slow Oracle 11g load and stress the connection pool.
	var (
		cols map[string]bool
		err  error
	)
	if len(opts.Filters) > 0 || len(opts.Sorts) > 0 {
		cols, err = browseTableColumns(p, db, dbName, tableName)
		if err != nil {
			return nil, err
		}
	}
	query, args, err := buildBrowseSQL(p, tableName, cols, opts)
	if err != nil {
		return nil, err
	}
	result, err := executeQueryArgs(p, db, dbName, query, args)
	if err != nil {
		return nil, err
	}
	if p.DriverName() == "oracle" {
		stripOraclePagingColumn(result)
	}
	result.HasMore = len(result.Rows) > opts.Limit
	if result.HasMore {
		result.Rows = result.Rows[:opts.Limit]
	}
	return result, nil
}

func buildUpdateSQL(p Provider, tableName string, cols map[string]bool, update RowUpdate) (string, []any, error) {
	if len(update.Set) == 0 || len(update.Where) == 0 {
		return "", nil, errors.New("update requires values and a primary key")
	}
	var args []any
	var assignments, predicates []string
	for _, col := range sortedKeys(update.Set) {
		if err := requireColumn(cols, col); err != nil {
			return "", nil, err
		}
		args = append(args, update.Set[col])
		assignments = append(assignments, p.Quote(col)+" = "+placeholder(p.DriverName(), len(args)))
	}
	for _, col := range sortedKeys(update.Where) {
		if err := requireColumn(cols, col); err != nil {
			return "", nil, err
		}
		if update.Where[col] == nil {
			predicates = append(predicates, p.Quote(col)+" IS NULL")
		} else {
			args = append(args, update.Where[col])
			predicates = append(predicates, p.Quote(col)+" = "+placeholder(p.DriverName(), len(args)))
		}
	}
	return "UPDATE " + p.Quote(tableName) + " SET " + strings.Join(assignments, ", ") + " WHERE " + strings.Join(predicates, " AND "), args, nil
}

func UpdateRowsAtomic(p Provider, db *sql.DB, dbName, tableName string, updates []RowUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	if len(updates) > 1000 {
		return errors.New("too many rows in one batch")
	}
	cols, err := browseTableColumns(p, db, dbName, tableName)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := p.PrepareExec(conn, dbName); err != nil {
		return err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	for i, update := range updates {
		query, args, err := buildUpdateSQL(p, tableName, cols, update)
		if err != nil {
			return fmt.Errorf("row %d: %w", i+1, err)
		}
		result, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("row %d: %w", i+1, err)
		}
		if affected, err := result.RowsAffected(); err == nil && affected != 1 {
			return fmt.Errorf("row %d: expected one matching row, got %d", i+1, affected)
		}
	}
	return tx.Commit()
}

func PreviewCSV(path string) (*CSVPreview, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	headers, err := reader.Read()
	if err != nil {
		return nil, err
	}
	if len(headers) == 0 {
		return nil, errors.New("CSV has no columns")
	}
	headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
	preview := &CSVPreview{Headers: headers, Rows: [][]string{}}
	for len(preview.Rows) < 20 {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		preview.Rows = append(preview.Rows, row)
	}
	return preview, nil
}

func ImportCSV(p Provider, db *sql.DB, dbName, tableName, path string, mapping []CSVColumnMapping) (int, error) {
	if len(mapping) == 0 {
		return 0, errors.New("select at least one target column")
	}
	cols, err := browseTableColumns(p, db, dbName, tableName)
	if err != nil {
		return 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	headers, err := reader.Read()
	if err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	for _, m := range mapping {
		if m.SourceIndex < 0 || m.SourceIndex >= len(headers) {
			return 0, fmt.Errorf("invalid CSV column index %d", m.SourceIndex)
		}
		if err := requireColumn(cols, m.Target); err != nil {
			return 0, err
		}
		if seen[m.Target] {
			return 0, fmt.Errorf("duplicate target column %q", m.Target)
		}
		seen[m.Target] = true
	}
	// The mapping order determines placeholder order, so keep it stable.
	sort.Slice(mapping, func(i, j int) bool { return mapping[i].SourceIndex < mapping[j].SourceIndex })
	quoted := make([]string, len(mapping))
	placeholders := make([]string, len(mapping))
	for i, m := range mapping {
		quoted[i] = p.Quote(m.Target)
		placeholders[i] = placeholder(p.DriverName(), i+1)
	}
	query := "INSERT INTO " + p.Quote(tableName) + " (" + strings.Join(quoted, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if err := p.PrepareExec(conn, dbName); err != nil {
		return 0, err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	count := 0
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("CSV row %d: %w", count+2, err)
		}
		args := make([]any, len(mapping))
		for i, m := range mapping {
			args[i] = row[m.SourceIndex]
		}
		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			return 0, fmt.Errorf("CSV row %d: %w", count+2, err)
		}
		count++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}
