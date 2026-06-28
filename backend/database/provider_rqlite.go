package database

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/rqlite/gorqlite/stdlib"
)

type rqliteProvider struct{}

func init() {
	Register("rqlite", &rqliteProvider{})
}

func (p *rqliteProvider) DSN(host string, port int, user, password, dbName string) string {
	addr := host
	if port > 0 {
		addr = fmt.Sprintf("%s:%d", host, port)
	}
	if user != "" && password != "" {
		return fmt.Sprintf("http://%s:%s@%s/", user, password, addr)
	}
	return fmt.Sprintf("http://%s/", addr)
}

func (p *rqliteProvider) DriverName() string {
	return "rqlite"
}

func (p *rqliteProvider) Quote(name string) string {
	return `"` + name + `"`
}

func (p *rqliteProvider) PrepareExec(db execer, dbName string) error {
	return nil
}

func (p *rqliteProvider) DefaultTableQuery(dbName, tableName string, limit int) string {
	return fmt.Sprintf("SELECT * FROM %s LIMIT %d", p.Quote(tableName), limit)
}

func (p *rqliteProvider) InsertRow(db *sql.DB, dbName, tableName string, values map[string]any) error {
	cols := sortedKeys(values)
	quotedCols := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	args := make([]any, 0, len(cols))
	for i, c := range cols {
		quotedCols[i] = p.Quote(c)
		placeholders[i] = "?"
		args = append(args, values[c])
	}
	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		p.Quote(tableName), strings.Join(quotedCols, ", "), strings.Join(placeholders, ", "))
	return execPrepared(p, db, dbName, sql, args)
}

func (p *rqliteProvider) UpdateRow(db *sql.DB, dbName, tableName string, set, where map[string]any) error {
	args := make([]any, 0, len(set)+len(where))
	phIdx := 1

	setParts := make([]string, 0, len(set))
	for _, c := range sortedKeys(set) {
		if set[c] == nil {
			setParts = append(setParts, fmt.Sprintf("%s = NULL", p.Quote(c)))
		} else {
			setParts = append(setParts, fmt.Sprintf("%s = ?", p.Quote(c)))
			args = append(args, set[c])
			phIdx++
		}
	}

	whereParts := make([]string, 0, len(where))
	for _, c := range sortedKeys(where) {
		if where[c] == nil {
			whereParts = append(whereParts, fmt.Sprintf("%s IS NULL", p.Quote(c)))
		} else {
			whereParts = append(whereParts, fmt.Sprintf("%s = ?", p.Quote(c)))
			args = append(args, where[c])
			phIdx++
		}
	}

	sql := fmt.Sprintf("UPDATE %s SET %s", p.Quote(tableName), strings.Join(setParts, ", "))
	if len(whereParts) > 0 {
		sql += " WHERE " + strings.Join(whereParts, " AND ")
	}
	return execPrepared(p, db, dbName, sql, args)
}

func (p *rqliteProvider) DeleteRow(db *sql.DB, dbName, tableName string, where map[string]any) error {
	args := make([]any, 0, len(where))
	phIdx := 1
	whereParts := make([]string, 0, len(where))
	for _, c := range sortedKeys(where) {
		if where[c] == nil {
			whereParts = append(whereParts, fmt.Sprintf("%s IS NULL", p.Quote(c)))
		} else {
			whereParts = append(whereParts, fmt.Sprintf("%s = ?", p.Quote(c)))
			args = append(args, where[c])
			phIdx++
		}
	}

	sql := fmt.Sprintf("DELETE FROM %s", p.Quote(tableName))
	if len(whereParts) > 0 {
		sql += " WHERE " + strings.Join(whereParts, " AND ")
	}
	return execPrepared(p, db, dbName, sql, args)
}

func (p *rqliteProvider) GetCapabilities() DBCapabilities {
	return DBCapabilities{
		"supportsOnUpdate":       false,
		"supportsCollation":      false,
		"supportsComment":        false,
		"supportsModifyColumn":   false,
		"supportsPrimaryKey":     false,
		"supportsCreateDatabase": false,
		"columnTypes":            rqliteTypes,
		"intTypes":               rqliteIntTypes,
	}
}

// ── Schema discovery ──

func (p *rqliteProvider) GetDatabases(db *sql.DB) ([]string, error) {
	return []string{"main"}, nil
}

func (p *rqliteProvider) GetTables(db *sql.DB, dbName string) ([]TableInfo, error) {
	results, err := queryStrings(db, "SELECT name, type FROM sqlite_master WHERE type IN ('table', 'view')")
	if err != nil {
		return nil, fmt.Errorf("get tables: %w", err)
	}
	infos := make([]TableInfo, 0, len(results))
	for _, row := range results {
		name := row["name"]
		if name == "sqlite_sequence" {
			continue
		}
		tp := strings.ToLower(row["type"])
		infos = append(infos, TableInfo{Name: name, Type: tp})
	}
	return infos, nil
}

func (p *rqliteProvider) GetTableSchema(db *sql.DB, dbName, tableName string) (*SchemaResult, error) {
	q := p.Quote
	colRows, err := queryStrings(db, fmt.Sprintf("PRAGMA table_info(%s)", q(tableName)))
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	columns := make([]ColumnInfo, 0, len(colRows))
	for _, row := range colRows {
		nullable := row["notnull"] == "0"
		isPrimary := row["pk"] != "" && row["pk"] != "0"
		defVal := row["dflt_value"]
		defaultType := "none"
		if defVal == "" && nullable {
			defaultType = "null"
			defVal = "NULL"
		} else if defVal == "NULL" {
			defaultType = "null"
		} else if defVal != "" {
			defaultType = "value"
		}
		columns = append(columns, ColumnInfo{
			Name:        row["name"],
			Type:        row["type"],
			Nullable:    nullable,
			DefaultVal:  defVal,
			DefaultType: defaultType,
			IsPrimary:   isPrimary,
		})
	}

	// Detect AUTOINCREMENT by checking sqlite_sequence
	seqRows, err := queryStrings(db, "SELECT name FROM sqlite_sequence WHERE name = ?", tableName)
	if err == nil && len(seqRows) > 0 {
		for i := range columns {
			if columns[i].IsPrimary && strings.Contains(strings.ToUpper(columns[i].Type), "INT") {
				columns[i].DefaultType = "auto"
			}
		}
	}

	idxRows, err := queryStrings(db, fmt.Sprintf("PRAGMA index_list(%s)", q(tableName)))
	if err != nil {
		return nil, fmt.Errorf("get indexes: %w", err)
	}

	indexes := make([]IndexInfo, 0)
	for _, idx := range idxRows {
		info := IndexInfo{
			Name:   idx["name"],
			Unique: idx["unique"] == "1",
		}
		colRows, err := queryStrings(db, fmt.Sprintf("PRAGMA index_info(%s)", q(idx["name"])))
		if err == nil {
			for _, c := range colRows {
				info.Columns = append(info.Columns, c["name"])
			}
		}
		indexes = append(indexes, info)
	}

	return &SchemaResult{Columns: columns, Indexes: indexes}, nil
}

// ── DDL: Database ──

func (p *rqliteProvider) CreateDatabase(db *sql.DB, dbName string) error {
	return fmt.Errorf("rqlite does not support CREATE DATABASE")
}

func (p *rqliteProvider) DropDatabase(db *sql.DB, dbName string) error {
	return fmt.Errorf("rqlite does not support DROP DATABASE")
}

// ── DDL: Table ──

func (p *rqliteProvider) CreateTable(db *sql.DB, dbName, tableName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE %s (id INTEGER PRIMARY KEY AUTOINCREMENT)", q(tableName)))
	return err
}

func (p *rqliteProvider) DropTable(db *sql.DB, dbName, tableName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("DROP TABLE %s", q(tableName)))
	return err
}

func (p *rqliteProvider) DropView(db *sql.DB, dbName, viewName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("DROP VIEW %s", q(viewName)))
	return err
}

func (p *rqliteProvider) TruncateTable(db *sql.DB, dbName, tableName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("DELETE FROM %s", q(tableName)))
	return err
}

// ── DDL: Column ──

func (p *rqliteProvider) AddColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
	if col.DefaultType == "auto" {
		return fmt.Errorf("rqlite only supports AUTOINCREMENT on INTEGER PRIMARY KEY columns at table creation time; use CREATE TABLE instead")
	}
	q := p.Quote
	var parts []string
	parts = append(parts, q(col.Name), col.Type)

	if col.Nullable {
		parts = append(parts, "NULL")
	} else {
		parts = append(parts, "NOT NULL")
	}

	switch col.DefaultType {
	case "null":
		parts = append(parts, "DEFAULT NULL")
	case "value":
		if col.DefaultVal != "" {
			parts = append(parts, "DEFAULT "+col.DefaultVal)
		} else {
			parts = append(parts, "DEFAULT ''")
		}
	}

	sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", q(tableName), strings.Join(parts, " "))
	_, err := db.Exec(sql)
	return err
}

func (p *rqliteProvider) ModifyColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
	return fmt.Errorf("rqlite does not support MODIFY COLUMN; rebuild the table instead")
}

func (p *rqliteProvider) DropColumn(db *sql.DB, dbName, tableName, colName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", q(tableName), q(colName)))
	return err
}

// ── DDL: Index ──

func (p *rqliteProvider) AddIndex(db *sql.DB, dbName, tableName string, idx IndexDef) error {
	q := p.Quote
	if idx.IsPrimary {
		return fmt.Errorf("rqlite does not support adding PRIMARY KEY after table creation")
	}
	uniqueStr := ""
	if idx.Unique {
		uniqueStr = "UNIQUE "
	}
	cols := make([]string, len(idx.Columns))
	for i, c := range idx.Columns {
		cols[i] = q(c)
	}
	_, err := db.Exec(fmt.Sprintf("CREATE %sINDEX %s ON %s (%s)", uniqueStr, q(idx.Name), q(tableName), strings.Join(cols, ", ")))
	return err
}

func (p *rqliteProvider) DropIndex(db *sql.DB, dbName, tableName, idxName string, isPrimary bool, autoIncCols []string) error {
	if isPrimary {
		return fmt.Errorf("rqlite does not support dropping PRIMARY KEY")
	}
	_, err := db.Exec(fmt.Sprintf("DROP INDEX %s", p.Quote(idxName)))
	return err
}

var rqliteTypes = []string{
	"INTEGER", "INT", "BIGINT", "SMALLINT", "TINYINT",
	"REAL", "DOUBLE", "FLOAT",
	"DECIMAL", "DECIMAL(10,2)",
	"NUMERIC", "NUMERIC(10,2)",
	"CHAR", "CHAR(1)",
	"VARCHAR", "VARCHAR(255)",
	"TEXT", "BLOB",
	"DATE", "DATETIME", "TIMESTAMP", "TIME",
	"BOOLEAN", "JSON",
}

var rqliteIntTypes = []string{
	"INT", "INTEGER", "BIGINT", "SMALLINT", "TINYINT",
}
