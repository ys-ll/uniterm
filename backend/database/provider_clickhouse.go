package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

// ClickHouse is a columnar OLAP engine with a SQL surface. It speaks its own
// native wire protocol (clickhouse-go, pure Go). Writes differ from OLTP
// engines: INSERT is first-class, but row updates/deletes are asynchronous
// mutations (or lightweight deletes on newer servers), primary keys are
// sort keys without uniqueness enforcement, and there are no foreign-key or
// unique constraints — reflected in the capability flags below.
type clickHouseProvider struct{}

func init() {
	Register("clickhouse", &clickHouseProvider{})
}

func (p *clickHouseProvider) DSN(host string, port int, user, password, dbName string, extraParams map[string]string) string {
	if port <= 0 {
		port = 9000
	}
	u := &url.URL{
		Scheme: "clickhouse",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   "/" + dbName,
	}
	q := u.Query()
	q.Set("dial_timeout", "10s")
	q.Set("read_timeout", "30s")
	for k, v := range extraParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (p *clickHouseProvider) DriverName() string {
	return "clickhouse"
}

func (p *clickHouseProvider) Quote(name string) string {
	q, _ := SafePgIdent(name)
	return q
}

// QualifyTable quotes a table identifier that may arrive database-qualified.
func (p *clickHouseProvider) QualifyTable(name string) string {
	schema, table := splitQualifiedName(name)
	if schema == "" {
		return p.Quote(table)
	}
	return p.Quote(schema) + "." + p.Quote(table)
}

func (p *clickHouseProvider) PrepareExec(db execer, dbName string) error {
	return nil
}

func (p *clickHouseProvider) DefaultTableQuery(dbName, tableName string, limit, offset int) string {
	if offset > 0 {
		return fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", p.QualifyTable(tableName), limit, offset)
	}
	return fmt.Sprintf("SELECT * FROM %s LIMIT %d", p.QualifyTable(tableName), limit)
}

func (p *clickHouseProvider) PagedTableQuery(dbName, tableName string, limit, offset int) string {
	return fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", p.QualifyTable(tableName), limit, offset)
}

func (p *clickHouseProvider) GetCapabilities() DBCapabilities {
	return DBCapabilities{
		"supportsAutoIncrement": false,
		"supportsOnUpdate":      false,
		"supportsCollation":     false,
		"supportsComment":       true,
		"supportsPrimaryKey":    false,
		"supportsModifyColumn":  true,
		"supportsSchemas":       false,
		"supportsCreateDatabase": true,
		"columnTypes":           clickHouseTypes,
		"intTypes":              clickHouseIntTypes,
	}
}

// ── Schema discovery ──

// GetDatabases lists the user databases; system and the information-schema
// aliases are hidden.
func (p *clickHouseProvider) GetDatabases(db *sql.DB) ([]string, error) {
	results, err := queryStrings(db, `
		SELECT name
		FROM system.databases
		WHERE name NOT IN ('system', 'INFORMATION_SCHEMA', 'information_schema')
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(results))
	for _, row := range results {
		names = append(names, row["name"])
	}
	return names, nil
}

// GetSchemas returns nil: ClickHouse has no inner schema level, the tree is
// database → tables.
func (p *clickHouseProvider) GetSchemas(db *sql.DB, dbName string) ([]string, error) {
	return nil, nil
}

// tableListBase builds the table listing against system.tables. Dot-prefixed
// names (`.inner.*` materialized-view storage) are internal.
func (p *clickHouseProvider) tableListBase(dbName string) (string, []any) {
	return `
		SELECT name AS table_name,
		       engine AS table_engine,
		       comment AS table_comment
		FROM system.tables
		WHERE database = ? AND name NOT LIKE '.%'`, []any{dbName}
}

func (p *clickHouseProvider) mapTableType(engine string) string {
	switch engine {
	case "View", "MaterializedView", "WindowView", "LiveView":
		return "view"
	default:
		return "table"
	}
}

func (p *clickHouseProvider) GetTables(db *sql.DB, dbName string) ([]TableInfo, error) {
	base, args := p.tableListBase(dbName)
	results, err := queryStrings(db, base+"\n\t\tORDER BY name", args...)
	if err != nil {
		return nil, fmt.Errorf("get tables: %w", err)
	}
	infos := make([]TableInfo, 0, len(results))
	for _, row := range results {
		infos = append(infos, TableInfo{
			Name:    row["table_name"],
			Type:    p.mapTableType(row["table_engine"]),
			Comment: row["table_comment"],
		})
	}
	return infos, nil
}

func (p *clickHouseProvider) GetTablePage(db *sql.DB, dbName string, query string, offset, limit int) ([]TableInfo, int, error) {
	base, args := p.tableListBase(dbName)

	countSQL := "SELECT COUNT(*) AS n FROM (" + base + ") t"
	countArgs := append([]any{}, args...)
	pageArgs := append([]any{}, args...)
	if query != "" {
		countSQL += " WHERE table_name LIKE ?"
		countArgs = append(countArgs, "%"+query+"%")
		pageSQLTail := " WHERE table_name LIKE ? ORDER BY table_name LIMIT ? OFFSET ?"
		pageArgs = append(pageArgs, "%"+query+"%", limit, offset)
		pageSQL := "SELECT table_name, table_engine, table_comment FROM (" + base + ") t" + pageSQLTail
		return p.pageFromSQL(db, countSQL, countArgs, pageSQL, pageArgs)
	}
	pageSQL := "SELECT table_name, table_engine, table_comment FROM (" + base + ") t ORDER BY table_name LIMIT ? OFFSET ?"
	pageArgs = append(pageArgs, limit, offset)
	return p.pageFromSQL(db, countSQL, countArgs, pageSQL, pageArgs)
}

func (p *clickHouseProvider) pageFromSQL(db *sql.DB, countSQL string, countArgs []any, pageSQL string, pageArgs []any) ([]TableInfo, int, error) {
	countRows, err := queryStrings(db, countSQL, countArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("count tables: %w", err)
	}
	total := 0
	if len(countRows) > 0 {
		total, _ = strconv.Atoi(countRows[0]["n"])
	}
	results, err := queryStrings(db, pageSQL, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("get tables page: %w", err)
	}
	infos := make([]TableInfo, 0, len(results))
	for _, row := range results {
		infos = append(infos, TableInfo{
			Name:    row["table_name"],
			Type:    p.mapTableType(row["table_engine"]),
			Comment: row["table_comment"],
		})
	}
	return infos, total, nil
}

func (p *clickHouseProvider) GetTableSchema(db *sql.DB, dbName, tableName string) (*SchemaResult, error) {
	database, table := splitQualifiedName(tableName)
	if database == "" {
		database = dbName
	}
	colRows, err := queryStrings(db, `
		SELECT name, type, comment, default_kind, default_expression, is_in_primary_key
		FROM system.columns
		WHERE database = ? AND table = ?
		ORDER BY position`, database, table)
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	columns := make([]ColumnInfo, 0, len(colRows))
	for _, row := range colRows {
		defVal := ""
		defaultType := "none"
		if row["default_kind"] == "DEFAULT" && row["default_expression"] != "" {
			defaultType = "value"
			defVal = row["default_expression"]
		}
		columns = append(columns, ColumnInfo{
			Name:        row["name"],
			Type:        row["type"],
			Nullable:    strings.HasPrefix(row["type"], "Nullable("),
			DefaultVal:  defVal,
			DefaultType: defaultType,
			IsPrimary:   row["is_in_primary_key"] == "1",
			Comment:     row["comment"],
		})
	}

	// Data-skipping indices; sorted-key structure is reported through the
	// primary flag above.
	idxRows, err := queryStrings(db, `
		SELECT name, expr
		FROM system.data_skipping_indices
		WHERE database = ? AND table = ?
		ORDER BY name`, database, table)
	if err != nil {
		idxRows = nil // non-fatal
	}
	idxMap := make(map[string]*IndexInfo)
	var idxOrder []string
	for _, row := range idxRows {
		name := row["name"]
		if _, ok := idxMap[name]; !ok {
			idxMap[name] = &IndexInfo{Name: name, Columns: []string{}}
			idxOrder = append(idxOrder, name)
		}
		if row["expr"] != "" {
			idxMap[name].Columns = append(idxMap[name].Columns, row["expr"])
		}
	}
	indexes := make([]IndexInfo, 0, len(idxOrder))
	for _, name := range idxOrder {
		indexes = append(indexes, *idxMap[name])
	}

	return &SchemaResult{Columns: columns, Indexes: indexes}, nil
}

// ── Row-level CRUD ──

// InsertRow runs a plain single-row INSERT — ClickHouse's native write path.
func (p *clickHouseProvider) InsertRow(db *sql.DB, dbName, tableName string, values map[string]any) error {
	cols := sortedKeys(values)
	quotedCols := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	args := make([]any, 0, len(cols))
	for i, c := range cols {
		quotedCols[i] = p.Quote(c)
		placeholders[i] = "?"
		args = append(args, values[c])
	}
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		p.QualifyTable(tableName), strings.Join(quotedCols, ", "), strings.Join(placeholders, ", "))
	return execPrepared(p, db, dbName, q, args)
}

// UpdateRow runs an ALTER TABLE ... UPDATE mutation. The mutation is applied
// asynchronously by the server on older setups; errors surface as-is.
func (p *clickHouseProvider) UpdateRow(db *sql.DB, dbName, tableName string, set, where map[string]any) error {
	var args []any
	setParts := make([]string, 0, len(set))
	for _, c := range sortedKeys(set) {
		if set[c] == nil {
			setParts = append(setParts, p.Quote(c)+" = NULL")
		} else {
			setParts = append(setParts, p.Quote(c)+" = ?")
			args = append(args, set[c])
		}
	}
	whereParts, whereArgs := p.whereClause(where)
	args = append(args, whereArgs...)
	q := fmt.Sprintf("ALTER TABLE %s UPDATE %s WHERE %s",
		p.QualifyTable(tableName), strings.Join(setParts, ", "), strings.Join(whereParts, " AND "))
	return execPrepared(p, db, dbName, q, args)
}

// DeleteRow uses the lightweight DELETE FROM syntax (servers since 23.3);
// older servers report it as unsupported and mutations-style deletion would
// need ALTER TABLE ... DELETE, applied asynchronously.
func (p *clickHouseProvider) DeleteRow(db *sql.DB, dbName, tableName string, where map[string]any) error {
	whereParts, args := p.whereClause(where)
	q := fmt.Sprintf("DELETE FROM %s WHERE %s", p.QualifyTable(tableName), strings.Join(whereParts, " AND "))
	return execPrepared(p, db, dbName, q, args)
}

func (p *clickHouseProvider) whereClause(where map[string]any) ([]string, []any) {
	var parts []string
	var args []any
	for _, c := range sortedKeys(where) {
		if where[c] == nil {
			parts = append(parts, p.Quote(c)+" IS NULL")
		} else {
			parts = append(parts, p.Quote(c)+" = ?")
			args = append(args, where[c])
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "1")
	}
	return parts, args
}

// ── DDL ──

func (p *clickHouseProvider) CreateDatabase(db *sql.DB, dbName string) error {
	_, err := db.Exec(fmt.Sprintf("CREATE DATABASE %s", p.Quote(dbName)))
	return err
}

func (p *clickHouseProvider) DropDatabase(db *sql.DB, dbName string) error {
	_, err := db.Exec(fmt.Sprintf("DROP DATABASE %s", p.Quote(dbName)))
	return err
}

// CreateTable creates a minimal MergeTree table; ClickHouse requires an
// engine, and a tuple() sort key keeps the stub neutral.
func (p *clickHouseProvider) CreateTable(db *sql.DB, dbName, tableName string) error {
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE %s (id UInt32) ENGINE = MergeTree ORDER BY tuple()", p.QualifyTable(tableName)))
	return err
}

func (p *clickHouseProvider) DropTable(db *sql.DB, dbName, tableName string) error {
	_, err := db.Exec(fmt.Sprintf("DROP TABLE %s", p.QualifyTable(tableName)))
	return err
}

func (p *clickHouseProvider) DropView(db *sql.DB, dbName, viewName string) error {
	_, err := db.Exec(fmt.Sprintf("DROP VIEW %s", p.QualifyTable(viewName)))
	return err
}

func (p *clickHouseProvider) TruncateTable(db *sql.DB, dbName, tableName string) error {
	_, err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s", p.QualifyTable(tableName)))
	return err
}

func (p *clickHouseProvider) CopyTable(db *sql.DB, dbName, tableName, newTableName string) error {
	_, err := db.Exec(fmt.Sprintf(
		"CREATE TABLE %s ENGINE = MergeTree ORDER BY tuple() AS SELECT * FROM %s",
		p.QualifyTable(newTableName), p.QualifyTable(tableName)))
	return err
}

// AddColumn adds a column. Nullability lives inside the ClickHouse type
// (Nullable(T)), so the nullable flag only suppresses NOT NULL.
func (p *clickHouseProvider) AddColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
	var parts []string
	parts = append(parts, p.Quote(col.Name), col.Type)
	if !col.Nullable {
		parts = append(parts, "NOT NULL")
	}
	if col.DefaultType == "value" {
		def, err := SafeDefaultLiteral(col.DefaultVal)
		if err != nil {
			return err
		}
		parts = append(parts, "DEFAULT "+def)
	}
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", p.QualifyTable(tableName), strings.Join(parts, " ")))
	return err
}

func (p *clickHouseProvider) ModifyColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
	q := fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s", p.QualifyTable(tableName), p.Quote(col.Name), col.Type)
	if col.DefaultType == "value" {
		def, err := SafeDefaultLiteral(col.DefaultVal)
		if err != nil {
			return err
		}
		q += " DEFAULT " + def
	}
	_, err := db.Exec(q)
	return err
}

func (p *clickHouseProvider) DropColumn(db *sql.DB, dbName, tableName, colName string) error {
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", p.QualifyTable(tableName), p.Quote(colName)))
	return err
}

// Index DDL is disabled via capabilities; these stay as explicit errors.
func (p *clickHouseProvider) AddIndex(db *sql.DB, dbName, tableName string, idx IndexDef) error {
	return fmt.Errorf("index management is not supported for this database type")
}

func (p *clickHouseProvider) DropIndex(db *sql.DB, dbName, tableName, idxName string, isPrimary bool, autoIncCols []string) error {
	return fmt.Errorf("index management is not supported for this database type")
}

// DumpTable export is not implemented yet.
func (p *clickHouseProvider) DumpTable(db *sql.DB, dbName, tableName string, opts DumpOptions) (string, error) {
	return "", &errDumpUnsupported{dbType: "clickhouse"}
}

var clickHouseTypes = []string{
	"UInt8", "UInt16", "UInt32", "UInt64",
	"Int8", "Int16", "Int32", "Int64",
	"Float32", "Float64",
	"Decimal(10,2)",
	"Bool",
	"String", "FixedString(16)", "LowCardinality(String)",
	"UUID",
	"Date", "Date32", "DateTime", "DateTime64(3)",
	"Enum8('a' = 1)", "Enum16('a' = 1)",
}

var clickHouseIntTypes = []string{
	"UInt8", "UInt16", "UInt32", "UInt64", "Int8", "Int16", "Int32", "Int64",
}
