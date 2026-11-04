package database

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

// sqliteProvider connects to a local SQLite database file. The file path is
// carried in the connection Host field (the same reuse pattern as serial
// connections reusing Host for the port name); host/port/user/password are
// otherwise unused.
type sqliteProvider struct{}

func init() {
	Register("sqlite", &sqliteProvider{})
}

// sortedStringKeys returns the sorted keys of a string map; deterministic
// ordering makes generated DSNs stable and easier to test.
func sortedStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (p *sqliteProvider) DriverName() string {
	return "sqlite"
}

// DSN returns the file path as-is when no extra parameters are given. With
// parameters, a file: URI is required for the driver to read them: each param
// becomes a "_pragma=name%3Dvalue" (the driver sends the value verbatim as a
// PRAGMA statement) unless it already starts with "_" (driver-level options
// like _txlock/_timefmt pass through verbatim).
func (p *sqliteProvider) DSN(host string, port int, user, password, dbName string, extraParams map[string]string) string {
	if len(extraParams) == 0 {
		return host
	}
	pragmas := make([]string, 0, len(extraParams)+1)
	if _, ok := extraParams["busy_timeout"]; !ok {
		pragmas = append(pragmas, "_pragma=busy_timeout%3D5000")
	}
	for _, k := range sortedStringKeys(extraParams) {
		if strings.HasPrefix(k, "_") {
			pragmas = append(pragmas, k+"="+extraParams[k])
			continue
		}
		pragmas = append(pragmas, "_pragma="+k+"%3D"+extraParams[k])
	}
	// A saved SQLite connection may contain a Windows path even when opened
	// on another platform. filepath.ToSlash only converts separators native to
	// the current OS, so normalize both forms for the URI DSN.
	return "file:" + strings.ReplaceAll(filepath.ToSlash(host), `\`, "/") + "?" + strings.Join(pragmas, "&")
}

func (p *sqliteProvider) Quote(name string) string {
	q, _ := SafePgIdent(name)
	return q
}

func (p *sqliteProvider) PrepareExec(db execer, dbName string) error {
	return nil
}

func (p *sqliteProvider) DefaultTableQuery(dbName, tableName string, limit, offset int) string {
	if offset > 0 {
		return fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", p.Quote(tableName), limit, offset)
	}
	return fmt.Sprintf("SELECT * FROM %s LIMIT %d", p.Quote(tableName), limit)
}

func (p *sqliteProvider) PagedTableQuery(dbName, tableName string, limit, offset int) string {
	return fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", p.Quote(tableName), limit, offset)
}

// ── Row-level CRUD ──

func (p *sqliteProvider) InsertRow(db *sql.DB, dbName, tableName string, values map[string]any) error {
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

func (p *sqliteProvider) UpdateRow(db *sql.DB, dbName, tableName string, set, where map[string]any) error {
	args := make([]any, 0, len(set)+len(where))

	setParts := make([]string, 0, len(set))
	for _, c := range sortedKeys(set) {
		if set[c] == nil {
			setParts = append(setParts, fmt.Sprintf("%s = NULL", p.Quote(c)))
		} else {
			setParts = append(setParts, fmt.Sprintf("%s = ?", p.Quote(c)))
			args = append(args, set[c])
		}
	}

	whereParts := make([]string, 0, len(where))
	for _, c := range sortedKeys(where) {
		if where[c] == nil {
			whereParts = append(whereParts, fmt.Sprintf("%s IS NULL", p.Quote(c)))
		} else {
			whereParts = append(whereParts, fmt.Sprintf("%s = ?", p.Quote(c)))
			args = append(args, where[c])
		}
	}

	sql := fmt.Sprintf("UPDATE %s SET %s", p.Quote(tableName), strings.Join(setParts, ", "))
	if len(whereParts) > 0 {
		sql += " WHERE " + strings.Join(whereParts, " AND ")
	}
	return execPrepared(p, db, dbName, sql, args)
}

func (p *sqliteProvider) DeleteRow(db *sql.DB, dbName, tableName string, where map[string]any) error {
	args := make([]any, 0, len(where))
	whereParts := make([]string, 0, len(where))
	for _, c := range sortedKeys(where) {
		if where[c] == nil {
			whereParts = append(whereParts, fmt.Sprintf("%s IS NULL", p.Quote(c)))
		} else {
			whereParts = append(whereParts, fmt.Sprintf("%s = ?", p.Quote(c)))
			args = append(args, where[c])
		}
	}

	sql := fmt.Sprintf("DELETE FROM %s", p.Quote(tableName))
	if len(whereParts) > 0 {
		sql += " WHERE " + strings.Join(whereParts, " AND ")
	}
	return execPrepared(p, db, dbName, sql, args)
}

func (p *sqliteProvider) GetCapabilities() DBCapabilities {
	return DBCapabilities{
		"supportsOnUpdate":       false,
		"supportsCollation":      false,
		"supportsComment":        false,
		"supportsModifyColumn":   false,
		"supportsPrimaryKey":     false,
		"supportsCreateDatabase": false,
		"columnTypes":            sqliteTypes,
		"intTypes":               sqliteIntTypes,
	}
}

// ── Schema discovery ──

func (p *sqliteProvider) GetDatabases(db *sql.DB) ([]string, error) {
	return []string{"main"}, nil
}

func (p *sqliteProvider) GetTables(db *sql.DB, dbName string) ([]TableInfo, error) {
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

func (p *sqliteProvider) GetTablePage(db *sql.DB, dbName string, query string, offset, limit int) ([]TableInfo, int, error) {
	base := `FROM sqlite_master WHERE type IN ('table', 'view') AND name != 'sqlite_sequence'`
	if query != "" {
		base += ` AND name LIKE ?`
	}
	var countArgs []any
	if query != "" {
		countArgs = append(countArgs, "%"+query+"%")
	}
	countRows, err := queryStrings(db, "SELECT COUNT(*) AS n "+base, countArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("count tables: %w", err)
	}
	total := 0
	if len(countRows) > 0 {
		total, _ = strconv.Atoi(countRows[0]["n"])
	}
	pageArgs := []any{}
	if query != "" {
		pageArgs = append(pageArgs, "%"+query+"%")
	}
	pageArgs = append(pageArgs, offset, limit)
	results, err := queryStrings(db,
		"SELECT name, type "+base+" ORDER BY name LIMIT ? OFFSET ?", pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("get tables page: %w", err)
	}
	infos := make([]TableInfo, 0, len(results))
	for _, row := range results {
		tp := strings.ToLower(row["type"])
		infos = append(infos, TableInfo{Name: row["name"], Type: tp})
	}
	return infos, total, nil
}

func (p *sqliteProvider) GetTableSchema(db *sql.DB, dbName, tableName string) (*SchemaResult, error) {
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

func (p *sqliteProvider) CreateDatabase(db *sql.DB, dbName string) error {
	return fmt.Errorf("sqlite does not support CREATE DATABASE")
}

func (p *sqliteProvider) DropDatabase(db *sql.DB, dbName string) error {
	return fmt.Errorf("sqlite does not support DROP DATABASE")
}

// ── DDL: Table ──

func (p *sqliteProvider) CreateTable(db *sql.DB, dbName, tableName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE %s (id INTEGER PRIMARY KEY AUTOINCREMENT)", q(tableName)))
	return err
}

func (p *sqliteProvider) DropTable(db *sql.DB, dbName, tableName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("DROP TABLE %s", q(tableName)))
	return err
}

func (p *sqliteProvider) DropView(db *sql.DB, dbName, viewName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("DROP VIEW %s", q(viewName)))
	return err
}

func (p *sqliteProvider) TruncateTable(db *sql.DB, dbName, tableName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("DELETE FROM %s", q(tableName)))
	return err
}

// CopyTable has no native clone in SQLite; replay the DumpTable output
// (structure + INSERTs) with the destination name substituted for the source.
func (p *sqliteProvider) CopyTable(db *sql.DB, dbName, tableName, newTableName string) error {
	dump, err := p.DumpTable(db, dbName, tableName, DumpOptions{Structure: true, Data: true})
	if err != nil {
		return err
	}
	stmts := SplitScript(dump)
	ctx := context.Background()
	for _, st := range stmts {
		// Skip the DROP guard emitted by DumpTable — the destination must be
		// created fresh or fail loudly if it already exists.
		if strings.HasPrefix(strings.ToUpper(st.SQL), "DROP ") {
			continue
		}
		// The CREATE and INSERT statements reference the source table name;
		// rename the first occurrence (CREATE "...name") and the INSERT target.
		sqlStr := sqliteRenameLeadingTableRef(st.SQL, tableName, newTableName)
		if _, err := db.ExecContext(ctx, sqlStr); err != nil {
			return err
		}
	}
	return nil
}

// sqliteRenameLeadingTableRef rewrites the quoted source table name that follows the
// leading CREATE TABLE / INSERT INTO keyword of a dump statement.
func sqliteRenameLeadingTableRef(stmt, oldName, newName string) string {
	q := strings.Index(stmt, `"`)
	if q >= 0 {
		quotedOld := `"` + oldName + `"`
		if strings.Contains(stmt, quotedOld) {
			return strings.Replace(stmt, quotedOld, `"`+newName+`"`, 1)
		}
	}
	return stmt
}

// ── DDL: Column ──

func (p *sqliteProvider) AddColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
	if col.DefaultType == "auto" {
		return fmt.Errorf("sqlite only supports AUTOINCREMENT on INTEGER PRIMARY KEY columns at table creation time; use CREATE TABLE instead")
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

func (p *sqliteProvider) ModifyColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
	return fmt.Errorf("sqlite does not support MODIFY COLUMN; rebuild the table instead")
}

func (p *sqliteProvider) DropColumn(db *sql.DB, dbName, tableName, colName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", q(tableName), q(colName)))
	return err
}

// ── DDL: Index ──

func (p *sqliteProvider) AddIndex(db *sql.DB, dbName, tableName string, idx IndexDef) error {
	q := p.Quote
	if idx.IsPrimary {
		return fmt.Errorf("sqlite does not support adding PRIMARY KEY after table creation")
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

func (p *sqliteProvider) DropIndex(db *sql.DB, dbName, tableName, idxName string, isPrimary bool, autoIncCols []string) error {
	if isPrimary {
		return fmt.Errorf("sqlite does not support dropping PRIMARY KEY")
	}
	_, err := db.Exec(fmt.Sprintf("DROP INDEX %s", p.Quote(idxName)))
	return err
}

var sqliteTypes = []string{
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

var sqliteIntTypes = []string{
	"INT", "INTEGER", "BIGINT", "SMALLINT", "TINYINT",
}

func (p *sqliteProvider) DumpTable(db *sql.DB, dbName, tableName string, opts DumpOptions) (string, error) {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	var b strings.Builder
	quotedTable := p.Quote(tableName)

	if opts.Structure {
		var createSQL string
		kind := sqliteTableKind(conn, tableName)
		err := conn.QueryRowContext(ctx,
			"SELECT sql FROM sqlite_master WHERE type=? AND name=? AND name NOT LIKE 'sqlite_%' LIMIT 1",
			kind, tableName).Scan(&createSQL)
		if err != nil && err != sql.ErrNoRows {
			return "", err
		}
		if createSQL != "" {
			b.WriteString("--\n-- Structure for ")
			b.WriteString(tableName)
			b.WriteString("\n--\nDROP ")
			if kind == "view" {
				b.WriteString("VIEW")
			} else {
				b.WriteString("TABLE")
			}
			b.WriteString(" IF EXISTS ")
			b.WriteString(quotedTable)
			b.WriteString(";\n")
			b.WriteString(createSQL)
			if !strings.HasSuffix(createSQL, ";") {
				b.WriteByte(';')
			}
			b.WriteString("\n\n")
		}
	}

	if opts.Data {
		cols, derr := sqliteTableColumnNames(conn, tableName, quotedTable)
		if derr != nil {
			return "", derr
		}
		if len(cols) > 0 {
			b.WriteString("--\n-- Data for ")
			b.WriteString(tableName)
			b.WriteString("\n--\n")
			colList := make([]string, len(cols))
			for i, c := range cols {
				colList[i] = p.Quote(c)
			}
			prefix := "INSERT INTO " + quotedTable + " (" + strings.Join(colList, ", ") + ") VALUES "

			rows, err := conn.QueryContext(ctx, "SELECT "+strings.Join(colList, ", ")+" FROM "+quotedTable)
			if err != nil {
				return "", err
			}
			defer rows.Close()

			for rows.Next() {
				vals := make([]any, len(cols))
				ptrs := make([]any, len(cols))
				for i := range vals {
					ptrs[i] = &vals[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					return "", err
				}
				b.WriteString(prefix)
				b.WriteByte('(')
				for i, v := range vals {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString(dumpSQLiteValue(v))
				}
				b.WriteString(");\n")
			}
			if err := rows.Err(); err != nil {
				return "", err
			}
			b.WriteByte('\n')
		}
	}

	return b.String(), nil
}

// sqliteTableKind returns "table" or "view" from sqlite_master.
func sqliteTableKind(conn *sql.Conn, tableName string) string {
	var t string
	err := conn.QueryRowContext(context.Background(),
		"SELECT type FROM sqlite_master WHERE name=? AND name NOT LIKE 'sqlite_%' LIMIT 1", tableName).Scan(&t)
	if err == nil && t == "view" {
		return "view"
	}
	return "table"
}

// sqliteTableColumnNames returns column names via PRAGMA table_info.
func sqliteTableColumnNames(conn *sql.Conn, tableName, quotedTable string) ([]string, error) {
	rows, err := conn.QueryContext(context.Background(), "PRAGMA table_info("+quotedTable+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

// dumpSQLiteValue renders a scanned cell as a SQLite literal. Blobs become
// x'..' hex literals; booleans and numbers pass through; everything else is
// a quoted string.
func dumpSQLiteValue(v any) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case []byte:
		return "x'" + hexLower(val) + "'"
	case string:
		return quotedString(val)
	case bool:
		if val {
			return "1"
		}
		return "0"
	default:
		return scanToString(v)
	}
}
