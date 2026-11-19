package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

type sqlserverProvider struct{}

func init() {
	Register("sqlserver", &sqlserverProvider{})
}

func (p *sqlserverProvider) DSN(host string, port int, user, password, dbName string, extraParams map[string]string) string {
	// A named instance ("server\instance") must not go into url.URL.Host:
	// url.URL.String() escapes the backslash to %5C and the driver's
	// url.Parse then rejects the DSN. The driver instead expects the
	// instance as the URL path (sqlserver://host/instance). With an
	// instance and no explicit port, omit the port so the driver resolves
	// the instance's dynamic port via SQL Browser (a forced 1433 would
	// bypass the Browser and almost always fail).
	u := &url.URL{Scheme: "sqlserver", User: url.UserPassword(user, password)}
	if server, instance, found := strings.Cut(host, `\`); found {
		u.Path = "/" + instance
		host = server
	} else if port <= 0 {
		port = 1433
	}
	if port > 0 {
		u.Host = fmt.Sprintf("%s:%d", host, port)
	} else {
		u.Host = host
	}
	q := u.Query()
	if dbName != "" {
		q.Set("database", dbName)
	}
	q.Set("encrypt", "disable")
	q.Set("dial timeout", "10")
	q.Set("connection timeout", "30")
	for k, v := range extraParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (p *sqlserverProvider) DriverName() string {
	return "sqlserver"
}

func (p *sqlserverProvider) Quote(name string) string {
	return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}

// splitQualifiedName splits a possibly schema-qualified identifier on its
// LAST dot: "sales.employees" → ("sales", "employees"); "employees" →
// ("", "employees"). The last-dot rule keeps database names that legally
// contain a dot intact ("my.db.sales" → ("my.db", "sales")). Identifiers
// whose base part contains a dot are out of scope for the tree encoding
//.
func splitQualifiedName(name string) (qualifier, base string) {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[:i], name[i+1:]
	}
	return "", name
}

// splitDbNode splits a tree node name into database and optional schema
// qualifier. Bare database nodes ("shop") yield schema "" — this is the
// dbName-side counterpart of splitQualifiedName, where a bare name means
// "database only", not "schema-less table".
func splitDbNode(dbName string) (db, schema string) {
	db, schema = splitQualifiedName(dbName)
	if db == "" {
		db = schema
		schema = ""
	}
	return db, schema
}

// QualifyTable quotes a table identifier that may arrive schema-qualified
// ("sales.employees" from the schema-grouped tree) or bare ("employees" from
// the degraded flat listing). Dialects whose tables are never qualified keep
// the Provider-interface default of plain Quote.
func (p *sqlserverProvider) QualifyTable(name string) string {
	schema, table := splitQualifiedName(name)
	if schema == "" {
		return p.Quote(table)
	}
	return p.Quote(schema) + "." + p.Quote(table)
}

// qualifyWithNodeSchema qualifies a bare table name with the schema carried
// by a composite "db.schema" node name. CREATE-like operations (CreateTable,
// CopyTable's target) must land inside the schema node the user opened, not
// the login's default schema.
func (p *sqlserverProvider) qualifyWithNodeSchema(dbName, tableName string) string {
	_, nodeSchema := splitDbNode(dbName)
	if nodeSchema == "" {
		return tableName // bare database node: legacy default-schema behavior
	}
	if schema, _ := splitQualifiedName(tableName); schema != "" {
		return tableName // already qualified
	}
	return nodeSchema + "." + tableName
}

func (p *sqlserverProvider) PrepareExec(db execer, dbName string) error {
	if dbName == "" {
		return nil
	}
	// Composite "db.schema" node names carry a schema qualifier that a USE
	// statement cannot express; switch the database part only.
	useDb, _ := splitDbNode(dbName)
	_, err := db.ExecContext(context.Background(), fmt.Sprintf("USE %s", p.Quote(useDb)))
	return err
}

// DefaultTableQuery and PagedTableQuery page arbitrary table rows with a
// ROW_NUMBER wrap instead of OFFSET/FETCH: the latter is SQL Server 2012+
// syntax and a SQL Server 2008 server rejects it outright. The synthetic
// __RN__ column stays visible here — unlike the DBBrowseTable path there
// is no post-scan strip, so users who run the pre-filled SQL by hand see
// it; it keeps the query self-contained and 2005+ compatible. Like the
// previous ORDER BY (SELECT NULL), rows carry no stable order, so pages
// may repeat or skip rows across queries — parity with the OFFSET
// version.
func (p *sqlserverProvider) pagedSelect(table string, limit, offset int) string {
	return fmt.Sprintf(
		`SELECT * FROM (SELECT q_.*, ROW_NUMBER() OVER (ORDER BY (SELECT NULL)) AS __RN__ FROM (%s) q_) __paged WHERE __RN__ > %d AND __RN__ <= %d`,
		"SELECT * FROM "+table, offset, offset+limit,
	)
}

func (p *sqlserverProvider) DefaultTableQuery(dbName, tableName string, limit, offset int) string {
	return p.pagedSelect(p.qualifiedTable(tableName), limit, offset)
}

func (p *sqlserverProvider) PagedTableQuery(dbName, tableName string, limit, offset int) string {
	return p.pagedSelect(p.qualifiedTable(tableName), limit, offset)
}

func (p *sqlserverProvider) qualifiedTable(tableName string) string {
	return p.QualifyTable(tableName)
}

// ── CRUD ──
//
// InsertRow/UpdateRow/DeleteRow go through execPrepared so that:
//   - USE [dbName] and the actual statement run on the same pooled conn
//     (DB-10, AUDIT-database-10). Previously each CRUD call issued a
//     `USE [db];\n...` multi-statement batch; the driver handles it but
//     the pool could still split work across conns for the trailing USE.
//   - context propagation is consistent with the other providers.
//
// withUse is intentionally not prepended here — execPrepared already calls
// PrepareExec, which issues USE [dbName] on the pinned connection.

func (p *sqlserverProvider) InsertRow(db *sql.DB, dbName, tableName string, values map[string]any) error {
	cols := sortedKeys(values)
	quotedCols := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	args := make([]any, 0, len(cols))
	for i, c := range cols {
		quotedCols[i] = p.Quote(c)
		placeholders[i] = fmt.Sprintf("@p%d", i+1)
		args = append(args, values[c])
	}
	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		p.qualifiedTable(tableName), strings.Join(quotedCols, ", "), strings.Join(placeholders, ", "))
	return execPrepared(p, db, dbName, sql, args)
}

func (p *sqlserverProvider) UpdateRow(db *sql.DB, dbName, tableName string, set, where map[string]any) error {
	args := make([]any, 0, len(set)+len(where))
	phIdx := 1

	setParts := make([]string, 0, len(set))
	for _, c := range sortedKeys(set) {
		if set[c] == nil {
			setParts = append(setParts, fmt.Sprintf("%s = NULL", p.Quote(c)))
		} else {
			setParts = append(setParts, fmt.Sprintf("%s = @p%d", p.Quote(c), phIdx))
			args = append(args, set[c])
			phIdx++
		}
	}

	whereParts := make([]string, 0, len(where))
	for _, c := range sortedKeys(where) {
		if where[c] == nil {
			whereParts = append(whereParts, fmt.Sprintf("%s IS NULL", p.Quote(c)))
		} else {
			whereParts = append(whereParts, fmt.Sprintf("%s = @p%d", p.Quote(c), phIdx))
			args = append(args, where[c])
			phIdx++
		}
	}

	sql := fmt.Sprintf("UPDATE %s SET %s", p.qualifiedTable(tableName), strings.Join(setParts, ", "))
	if len(whereParts) > 0 {
		sql += " WHERE " + strings.Join(whereParts, " AND ")
	}
	return execPrepared(p, db, dbName, sql, args)
}

func (p *sqlserverProvider) DeleteRow(db *sql.DB, dbName, tableName string, where map[string]any) error {
	args := make([]any, 0, len(where))
	phIdx := 1
	whereParts := make([]string, 0, len(where))
	for _, c := range sortedKeys(where) {
		if where[c] == nil {
			whereParts = append(whereParts, fmt.Sprintf("%s IS NULL", p.Quote(c)))
		} else {
			whereParts = append(whereParts, fmt.Sprintf("%s = @p%d", p.Quote(c), phIdx))
			args = append(args, where[c])
			phIdx++
		}
	}

	sql := fmt.Sprintf("DELETE FROM %s", p.qualifiedTable(tableName))
	if len(whereParts) > 0 {
		sql += " WHERE " + strings.Join(whereParts, " AND ")
	}
	return execPrepared(p, db, dbName, sql, args)
}

// ── Capabilities ──

func (p *sqlserverProvider) GetCapabilities() DBCapabilities {
	return DBCapabilities{
		"supportsOnUpdate":           false,
		"supportsCollation":          true,
		"supportsComment":            false,
		"autoIncrementForcesNotNull": true,
		"columnTypes":                sqlserverTypes,
		"intTypes":                   sqlserverIntTypes,
		// inner-schema dialect flags consumed by the frontend
		// (no dbType checks there). undroppableSchemas drives hiding the
		// schema-delete menu item for schemas the server refuses to drop.
		"supportsSchemas":    true,
		"defaultSchema":      "dbo",
		"undroppableSchemas": []string{"dbo"},
	}
}

// ── Schema discovery ──

// GetDatabases returns the bare user-database names; the tree fetches each
// database's schemas via GetSchemas and renders the second level.

func (p *sqlserverProvider) GetDatabases(db *sql.DB) ([]string, error) {
	results, err := queryStrings(db, `
		SELECT name FROM sys.databases
		WHERE name NOT IN ('master','tempdb','model','msdb')
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

// GetSchemas lists every user schema of a database, including schemas that
// do not own any table or view yet — a schema the user just created must
// appear in the tree before it holds a table . Fixed
// database roles (db_*) and system schemas are filtered out; dbo stays,
// matching SSMS expectations. dbName is a REAL database name (possibly
// containing dots) from GetDatabases, so it must not go through withUse —
// that splits composite NODE names from the tree.
func (p *sqlserverProvider) GetSchemas(db *sql.DB, dbName string) ([]string, error) {
	schemaSQL := `
		SELECT s.name AS schema_name
		FROM sys.schemas s
		WHERE s.name NOT IN ('sys', 'guest', 'INFORMATION_SCHEMA')
		  AND s.name NOT LIKE 'db[_]%'
		ORDER BY s.name`
	results, err := queryStrings(db, fmt.Sprintf("USE %s;\n%s", p.Quote(dbName), schemaSQL))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(results))
	for _, row := range results {
		names = append(names, row["schema_name"])
	}
	return names, nil
}

func (p *sqlserverProvider) GetTables(db *sql.DB, dbName string) ([]TableInfo, error) {
	// A composite "db.schema" node lists only its own schema's
	// objects, and object names come back schema-qualified ("sales.orders")
	// so every downstream binding keeps working without signature changes.
	// A bare dbName (degraded node) keeps the legacy flat listing.
	base, args, _ := p.tableListBase(dbName)
	query := base + "\n\t\tORDER BY table_name"

	results, err := queryStrings(db, p.withUse(dbName, query), args...)
	if err != nil {
		return nil, fmt.Errorf("get tables: %w", err)
	}
	infos := make([]TableInfo, 0, len(results))
	for _, row := range results {
		tp := "table"
		if row["type_desc"] == "VIEW" {
			tp = "view"
		}
		infos = append(infos, TableInfo{
			Name:    row["table_name"],
			Type:    tp,
			Comment: row["table_comment"],
		})
	}
	return infos, nil
}

// tableListBase builds the tables-UNION-views listing body shared by
// GetTables and GetTablePage. When dbName carries a schema qualifier
// ("db.schema"), only that schema's objects are listed and
// table_name comes back schema-qualified; a bare dbName keeps the legacy
// flat listing with unqualified names (degraded/permission-fallback nodes).
func (p *sqlserverProvider) tableListBase(dbName string) (string, []any, int) {
	_, schema := splitDbNode(dbName)
	var args []any
	// Build each UNION half from its own alias so the views half can never
	// reference the tables alias (regression: t.name leaked into the views
	// SELECT and broke every schema-filtered listing).
	half := func(from, alias, kind string) string {
		sel := alias + ".name AS table_name"
		join, where := "", "WHERE "+alias+".is_ms_shipped = 0"
		if schema != "" {
			sel = "s.name + N'.' + " + alias + ".name AS table_name"
			join = "\n\t\tJOIN sys.schemas s ON s.schema_id = " + alias + ".schema_id" +
				"\n\t\t  AND s.name = @p1"
			where += " AND s.name = @p1"
		}
		return `
		SELECT ` + sel + `, ` + kind + ` AS type_desc,
		       ISNULL(CONVERT(nvarchar(4000), ep.value), '') AS table_comment
		FROM ` + from + `
		LEFT JOIN sys.extended_properties ep
		  ON ep.major_id = ` + alias + `.object_id AND ep.minor_id = 0 AND ep.name = N'MS_Description'` + join + `
		` + where
	}
	if schema != "" {
		args = append(args, schema)
	}
	base := half("sys.tables t", "t", "'TABLE'") + `
		UNION ALL` + half("sys.views v", "v", "'VIEW'")
	return base, args, len(args) + 1
}

func (p *sqlserverProvider) GetTablePage(db *sql.DB, dbName string, query string, offset, limit int) ([]TableInfo, int, error) {
	base, baseArgs, next := p.tableListBase(dbName)

	countArgs := append([]any{}, baseArgs...)
	countSQL := "SELECT COUNT(*) AS n FROM (" + base + ") t"
	if query != "" {
		countSQL += fmt.Sprintf(" WHERE table_name LIKE @p%d", next)
		countArgs = append(countArgs, "%"+query+"%")
	}
	countRows, err := queryStrings(db, p.withUse(dbName, countSQL), countArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("count tables: %w", err)
	}
	total := 0
	if len(countRows) > 0 {
		total, _ = strconv.Atoi(countRows[0]["n"])
	}

	// ROW_NUMBER() paging instead of OFFSET/FETCH: the latter is SQL
	// Server 2012+ syntax and a SQL Server 2008 server rejects it, leaving
	// the tree stuck on "loading all tables" on old instances.
	// ROW_NUMBER() OVER works on SQL Server 2005+ and, unlike the data-grid
	// path, the row number can be stripped here because the column list is
	// known and the rn order (ORDER BY table_name) is deterministic.
	pageInner := "SELECT table_name, type_desc, table_comment, ROW_NUMBER() OVER (ORDER BY table_name) AS __rn FROM (" + base + ") t"
	pageArgs := append([]any{}, baseArgs...)
	if query != "" {
		pageInner += fmt.Sprintf(" WHERE table_name LIKE @p%d", next)
		pageArgs = append(pageArgs, "%"+query+"%")
	}
	pageSQL := fmt.Sprintf(
		"SELECT table_name, type_desc, table_comment FROM (%s) __paged WHERE __rn BETWEEN @p%d AND @p%d",
		pageInner, len(pageArgs)+1, len(pageArgs)+2,
	)
	pageArgs = append(pageArgs, offset+1, offset+limit)
	results, err := queryStrings(db, p.withUse(dbName, pageSQL), pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("get tables page: %w", err)
	}
	infos := make([]TableInfo, 0, len(results))
	for _, row := range results {
		tp := "table"
		if row["type_desc"] == "VIEW" {
			tp = "view"
		}
		infos = append(infos, TableInfo{
			Name:    row["table_name"],
			Type:    tp,
			Comment: row["table_comment"],
		})
	}
	return infos, total, nil
}

func (p *sqlserverProvider) GetTableSchema(db *sql.DB, dbName, tableName string) (*SchemaResult, error) {
	schema, table := splitQualifiedName(tableName)
	colWhere, colArgs := "WHERE TABLE_NAME = @p1", []any{table}
	if schema != "" {
		colWhere += " AND TABLE_SCHEMA = @p2"
		colArgs = append(colArgs, schema)
	}
	colRows, err := queryStrings(db, p.withUse(dbName, `
		SELECT COLUMN_NAME, DATA_TYPE, IS_NULLABLE, COLUMN_DEFAULT,
		       CHARACTER_MAXIMUM_LENGTH, NUMERIC_PRECISION, NUMERIC_SCALE
		FROM INFORMATION_SCHEMA.COLUMNS
		` + colWhere + `
		ORDER BY ORDINAL_POSITION`), colArgs...)
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	pkRows, err := queryStrings(db, p.withUse(dbName, `
		SELECT kcu.COLUMN_NAME
		FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
		JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu
		  ON tc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME
		  AND tc.TABLE_NAME = kcu.TABLE_NAME
		WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY'
		  AND tc.TABLE_NAME = @p1
		  AND tc.TABLE_SCHEMA = @p2`), table, schema)
	pkCols := make(map[string]bool)
	if err == nil {
		for _, row := range pkRows {
			pkCols[row["COLUMN_NAME"]] = true
		}
	}

	columns := make([]ColumnInfo, 0, len(colRows))
	for _, row := range colRows {
		nullable := row["IS_NULLABLE"] == "YES"
		defVal := row["COLUMN_DEFAULT"]
		defaultType := "none"
		if defVal == "" {
			if nullable {
				defaultType = "null"
				defVal = "NULL"
			}
		} else {
			defVal = strings.Trim(defVal, "()")
			defaultType = "value"
		}

		dataType := row["DATA_TYPE"]
		typeStr := sqlserverFormatType(dataType, row["CHARACTER_MAXIMUM_LENGTH"], row["NUMERIC_PRECISION"], row["NUMERIC_SCALE"])

		columns = append(columns, ColumnInfo{
			Name:        row["COLUMN_NAME"],
			Type:        typeStr,
			Nullable:    nullable,
			DefaultVal:  defVal,
			DefaultType: defaultType,
			IsPrimary:   pkCols[row["COLUMN_NAME"]],
		})
	}

	idxWhere, idxArgs := "WHERE t.name = @p1", []any{table}
	if schema != "" {
		idxWhere += " AND s.name = @p2"
		idxArgs = append(idxArgs, schema)
	}
	idxRows, err := queryStrings(db, p.withUse(dbName, `
		SELECT i.name AS index_name, i.is_unique, i.is_primary_key, c.name AS column_name
		FROM sys.indexes i
		JOIN sys.index_columns ic ON i.object_id = ic.object_id AND i.index_id = ic.index_id
		JOIN sys.columns c ON ic.object_id = c.object_id AND ic.column_id = c.column_id
		JOIN sys.tables t ON i.object_id = t.object_id
		JOIN sys.schemas s ON s.schema_id = t.schema_id
		` + idxWhere + `
		ORDER BY i.name, ic.key_ordinal`), idxArgs...)
	if err != nil {
		return nil, fmt.Errorf("get indexes: %w", err)
	}

	idxMap := make(map[string]*IndexInfo)
	var idxOrder []string
	for _, row := range idxRows {
		name := row["index_name"]
		if _, ok := idxMap[name]; !ok {
			idxMap[name] = &IndexInfo{
				Name:      name,
				Columns:   []string{},
				Unique:    row["is_unique"] == "1" || row["is_unique"] == "true",
				IsPrimary: row["is_primary_key"] == "1" || row["is_primary_key"] == "true",
			}
			idxOrder = append(idxOrder, name)
		}
		idxMap[name].Columns = append(idxMap[name].Columns, row["column_name"])
	}

	indexes := make([]IndexInfo, 0, len(idxOrder))
	for _, name := range idxOrder {
		indexes = append(indexes, *idxMap[name])
	}

	return &SchemaResult{Columns: columns, Indexes: indexes}, nil
}

// ── DDL: Database ──

// CreateDatabase accepts a composite "db.schema" name from the new-database
// dialog: the schema part is optional and created inside the
// new database; empty means "use the login's default schema" (usually dbo),
// which the tree resolves via SCHEMA_NAME() on the next refresh. The dialog
// always sends "name.schema" — a trailing dot when the schema is empty — so
// the last segment is unambiguously the schema and dot-named database names
// ("my.db") stay legal. A pre-existing database is tolerated when a schema
// is requested: only the missing schema is created; both already present is
// reported as a duplicate.
func (p *sqlserverProvider) CreateDatabase(db *sql.DB, dbName string) error {
	useDb, schema := splitDbNode(dbName)
	var dbCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sys.databases WHERE name = @p1`, useDb).Scan(&dbCount); err != nil {
		return err
	}
	if dbCount == 0 {
		if _, err := db.Exec(fmt.Sprintf("CREATE DATABASE %s", p.Quote(useDb))); err != nil {
			return err
		}
	} else if schema == "" {
		return fmt.Errorf("database %s already exists", useDb)
	}
	if schema == "" {
		return nil
	}
	if dbCount > 0 {
		// Pre-existing database: is the requested schema already there?
		var schemaCount int
		if err := db.QueryRow(p.withUse(useDb, `SELECT COUNT(*) FROM sys.schemas WHERE name = @p1`), schema).Scan(&schemaCount); err != nil {
			return err
		}
		if schemaCount > 0 {
			return fmt.Errorf("schema %s already exists in database %s", schema, useDb)
		}
	}
	// CREATE SCHEMA must be the only statement in its batch and must run
	// inside the database — execPrepared pins USE + CREATE SCHEMA to one
	// pooled connection so the schema cannot land in master.
	return execPrepared(p, db, useDb, fmt.Sprintf("CREATE SCHEMA %s", p.Quote(schema)), nil)
}

// DropSchema drops the schema part of a composite "db.schema" node name.
// T-SQL refuses to drop a schema that still owns objects; that driver error
// is surfaced as-is so the user knows to empty the schema first.
func (p *sqlserverProvider) DropSchema(db *sql.DB, dbName string) error {
	useDb, schema := splitDbNode(dbName)
	if schema == "" {
		return fmt.Errorf("node %q carries no schema qualifier", dbName)
	}
	return execPrepared(p, db, useDb, fmt.Sprintf("DROP SCHEMA %s", p.Quote(schema)), nil)
}

func (p *sqlserverProvider) DropDatabase(db *sql.DB, dbName string) error {
	// The tree hands out composite "db.schema" node names; a database can
	// only be dropped by its bare name.
	useDb, _ := splitDbNode(dbName)
	_, _ = db.Exec(fmt.Sprintf("ALTER DATABASE %s SET SINGLE_USER WITH ROLLBACK IMMEDIATE", p.Quote(useDb)))
	_, err := db.Exec(fmt.Sprintf("DROP DATABASE %s", p.Quote(useDb)))
	return err
}

// ── DDL: Table ──

func (p *sqlserverProvider) CreateTable(db *sql.DB, dbName, tableName string) error {
	// A new table created inside a "db.schema" node must land in that
	// schema, not the login's default schema.
	name := p.qualifyWithNodeSchema(dbName, tableName)
	_, err := db.Exec(p.withUse(dbName, fmt.Sprintf("CREATE TABLE %s (id INT IDENTITY(1,1) PRIMARY KEY)", p.QualifyTable(name))))
	return err
}

func (p *sqlserverProvider) DropTable(db *sql.DB, dbName, tableName string) error {
	_, err := db.Exec(p.withUse(dbName, fmt.Sprintf("DROP TABLE %s", p.qualifiedTable(tableName))))
	return err
}

func (p *sqlserverProvider) DropView(db *sql.DB, dbName, viewName string) error {
	_, err := db.Exec(p.withUse(dbName, fmt.Sprintf("DROP VIEW %s", p.qualifiedTable(viewName))))
	return err
}

func (p *sqlserverProvider) TruncateTable(db *sql.DB, dbName, tableName string) error {
	_, err := db.Exec(p.withUse(dbName, fmt.Sprintf("TRUNCATE TABLE %s", p.qualifiedTable(tableName))))
	return err
}

// CopyTable clones structure and data via SELECT INTO (no indexes/constraints).
// The clone lands in the same schema node the source lives in.
func (p *sqlserverProvider) CopyTable(db *sql.DB, dbName, tableName, newTableName string) error {
	target := p.qualifyWithNodeSchema(dbName, newTableName)
	_, err := db.Exec(p.withUse(dbName, fmt.Sprintf("SELECT * INTO %s FROM %s", p.QualifyTable(target), p.QualifyTable(tableName))))
	return err
}

// ── DDL: Column ──

func (p *sqlserverProvider) AddColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
	_, err := db.Exec(p.withUse(dbName, p.buildColumnSQL("ADD", tableName, col)))
	return err
}

func (p *sqlserverProvider) ModifyColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
	q := p.qualifiedTable
	stmts := []string{}

	stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s",
		q(tableName), p.Quote(col.Name), col.Type))

	if col.Nullable {
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s NULL",
			q(tableName), p.Quote(col.Name), col.Type))
	} else {
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s NOT NULL",
			q(tableName), p.Quote(col.Name), col.Type))
	}

	for _, s := range stmts {
		if _, err := db.Exec(p.withUse(dbName, s)); err != nil {
			return err
		}
	}
	return nil
}

func (p *sqlserverProvider) DropColumn(db *sql.DB, dbName, tableName, colName string) error {
	// Drop any default constraint bound to the column first.
	rows, err := db.Query(p.withUse(dbName, `
		SELECT dc.name
		FROM sys.default_constraints dc
		JOIN sys.columns c ON dc.parent_object_id = c.object_id AND dc.parent_column_id = c.column_id
		JOIN sys.tables t ON c.object_id = t.object_id
		WHERE t.name = @p1 AND c.name = @p2`), tableName, colName)
	if err != nil {
		return fmt.Errorf("find default constraints: %w", err)
	}
	defer rows.Close()

	var constraints []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		constraints = append(constraints, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	qt := p.qualifiedTable(tableName)
	for _, name := range constraints {
		if _, err := db.Exec(p.withUse(dbName, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", qt, p.Quote(name)))); err != nil {
			return fmt.Errorf("drop default constraint %s: %w", name, err)
		}
	}

	_, err = db.Exec(p.withUse(dbName, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", qt, p.Quote(colName))))
	return err
}

// ── DDL: Index ──

func (p *sqlserverProvider) AddIndex(db *sql.DB, dbName, tableName string, idx IndexDef) error {
	q := p.Quote
	qt := p.qualifiedTable
	if idx.IsPrimary {
		cols := make([]string, len(idx.Columns))
		for i, c := range idx.Columns {
			cols[i] = q(c)
		}
		_, err := db.Exec(p.withUse(dbName, fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY (%s)", qt(tableName), strings.Join(cols, ", "))))
		return err
	}
	uniqueStr := ""
	if idx.Unique {
		uniqueStr = "UNIQUE "
	}
	cols := make([]string, len(idx.Columns))
	for i, c := range idx.Columns {
		cols[i] = q(c)
	}
	_, err := db.Exec(p.withUse(dbName, fmt.Sprintf("CREATE %sINDEX %s ON %s (%s)", uniqueStr, q(idx.Name), qt(tableName), strings.Join(cols, ", "))))
	return err
}

func (p *sqlserverProvider) DropIndex(db *sql.DB, dbName, tableName, idxName string, isPrimary bool, autoIncCols []string) error {
	q := p.Quote
	qt := p.qualifiedTable
	if isPrimary {
		_, err := db.Exec(p.withUse(dbName, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", qt(tableName), q(idxName))))
		return err
	}
	_, err := db.Exec(p.withUse(dbName, fmt.Sprintf("DROP INDEX %s ON %s", q(idxName), qt(tableName))))
	return err
}

// ── Helpers ──

// withUse prepends a USE [dbName] statement to the query so that it
// always runs against the correct database, regardless of which connection
// the pool picks. This is the SQL Server equivalent of MySQL's
// SHOW FULL TABLES FROM `dbName` — both schema-discovery queries and
// parameterised CRUD use it because SQL Server's sys.* / INFORMATION_SCHEMA
// views are database-scoped.
func (p *sqlserverProvider) withUse(dbName, query string) string {
	if dbName == "" {
		return query
	}
	// Composite "db.schema" node names carry a schema
	// qualifier a USE statement cannot express — switch the database part only.
	useDb, _ := splitDbNode(dbName)
	return fmt.Sprintf("USE %s;\n%s", p.Quote(useDb), query)
}

func (p *sqlserverProvider) buildColumnSQL(action, tableName string, col ColumnDef) string {
	q := p.Quote
	parts := []string{q(col.Name), col.Type}

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
			parts = append(parts, fmt.Sprintf("DEFAULT '%s'", strings.ReplaceAll(col.DefaultVal, "'", "''")))
		} else {
			parts = append(parts, "DEFAULT ''")
		}
	case "auto":
		parts = append(parts, "IDENTITY(1,1)")
	}

	return fmt.Sprintf("ALTER TABLE %s %s %s", p.qualifiedTable(tableName), action, strings.Join(parts, " "))
}

func sqlserverFormatType(dataType, charMaxLen, numericPrecision, numericScale string) string {
	switch strings.ToUpper(dataType) {
	case "VARCHAR", "NVARCHAR", "CHAR", "NCHAR", "VARBINARY", "BINARY":
		if charMaxLen != "" && charMaxLen != "-1" {
			return fmt.Sprintf("%s(%s)", dataType, charMaxLen)
		} else if charMaxLen == "-1" {
			return fmt.Sprintf("%s(MAX)", dataType)
		}
	case "DECIMAL", "NUMERIC":
		if numericPrecision != "" && numericScale != "" {
			return fmt.Sprintf("%s(%s,%s)", dataType, numericPrecision, numericScale)
		} else if numericPrecision != "" {
			return fmt.Sprintf("%s(%s)", dataType, numericPrecision)
		}
	}
	return dataType
}

var sqlserverTypes = []string{
	"BIT",
	"TINYINT", "SMALLINT", "INT", "BIGINT",
	"FLOAT", "REAL", "DECIMAL(10,2)", "NUMERIC(10,2)", "MONEY", "SMALLMONEY",
	"CHAR(1)", "VARCHAR(255)", "VARCHAR(MAX)", "NVARCHAR(255)", "NVARCHAR(MAX)",
	"TEXT", "NTEXT",
	"BINARY(1)", "VARBINARY(255)", "VARBINARY(MAX)",
	"DATE", "TIME", "DATETIME", "DATETIME2", "SMALLDATETIME", "DATETIMEOFFSET",
	"UNIQUEIDENTIFIER",
}

var sqlserverIntTypes = []string{
	"TINYINT", "SMALLINT", "INT", "BIGINT",
}

func (p *sqlserverProvider) DumpTable(db *sql.DB, dbName, tableName string, opts DumpOptions) (string, error) {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if err := p.PrepareExec(conn, dbName); err != nil {
		return "", err
	}

	var b strings.Builder
	quotedTable := p.QualifyTable(tableName)
	// INFORMATION_SCHEMA.TABLE_SCHEMA is the schema, not the database. The
	// old code passed dbName here, which never matched and silently produced
	// An empty CREATE TABLE / zero-column INSERT dump.
	dumpSchema, dumpTable := splitQualifiedName(tableName)

	if opts.Structure {
		createSQL, derr := sqlserverCreateTableSQL(conn, dumpSchema, dumpTable, quotedTable)
		if derr != nil {
			return "", derr
		}
		b.WriteString("--\n-- Structure for ")
		b.WriteString(tableName)
		b.WriteString("\n--\nDROP TABLE IF EXISTS ")
		b.WriteString(quotedTable)
		b.WriteString(";\n")
		b.WriteString(createSQL)
		if !strings.HasSuffix(createSQL, ";") {
			b.WriteByte(';')
		}
		b.WriteString("\n\n")
	}

	if opts.Data {
		cols, colTypes, derr := sqlserverTableColumns(conn, dumpSchema, dumpTable)
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
					b.WriteString(dumpSqlserverValue(v, colTypes[i]))
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

// sqlserverCreateTableSQL builds a CREATE TABLE statement from INFORMATION_SCHEMA.
// schema is the TABLE_SCHEMA filter ("" = no filter, legacy flat listing).
func sqlserverCreateTableSQL(conn *sql.Conn, schema, tableName, quotedTable string) (string, error) {
	colWhere, colArgs := "WHERE TABLE_NAME = ?", []any{tableName}
	if schema != "" {
		colWhere += " AND TABLE_SCHEMA = ?"
		colArgs = append(colArgs, schema)
	}
	rows, err := conn.QueryContext(context.Background(), `
		SELECT COLUMN_NAME, DATA_TYPE, IS_NULLABLE, COLUMN_DEFAULT,
		       CHARACTER_MAXIMUM_LENGTH, NUMERIC_PRECISION, NUMERIC_SCALE
		FROM INFORMATION_SCHEMA.COLUMNS
		` + colWhere + `
		ORDER BY ORDINAL_POSITION`, colArgs...)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var b strings.Builder
	b.WriteString("CREATE TABLE ")
	b.WriteString(quotedTable)
	b.WriteString(" (\n")
	first := true
	for rows.Next() {
		var name, dataType, isNullable string
		var colDefault sql.NullString
		var charMaxLen, numPrec, numScale sql.NullString
		if err := rows.Scan(&name, &dataType, &isNullable, &colDefault, &charMaxLen, &numPrec, &numScale); err != nil {
			return "", err
		}
		if !first {
			b.WriteString(",\n")
		}
		first = false
		b.WriteString("  [")
		b.WriteString(name)
		b.WriteString("] ")
		b.WriteString(sqlserverFormatType(dataType, charMaxLen.String, numPrec.String, numScale.String))
		if colDefault.Valid && colDefault.String != "" {
			b.WriteString(" DEFAULT ")
			b.WriteString(colDefault.String)
		}
		if strings.ToUpper(isNullable) == "NO" {
			b.WriteString(" NOT NULL")
		} else {
			b.WriteString(" NULL")
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	b.WriteString("\n)")
	return b.String(), nil
}

// sqlserverTableColumns returns column names and raw type tokens.
func sqlserverTableColumns(conn *sql.Conn, schema, tableName string) ([]string, []string, error) {
	colWhere, colArgs := "WHERE TABLE_NAME = ?", []any{tableName}
	if schema != "" {
		colWhere += " AND TABLE_SCHEMA = ?"
		colArgs = append(colArgs, schema)
	}
	rows, err := conn.QueryContext(context.Background(), `
		SELECT COLUMN_NAME, DATA_TYPE
		FROM INFORMATION_SCHEMA.COLUMNS
		` + colWhere + `
		ORDER BY ORDINAL_POSITION`, colArgs...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var names, types []string
	for rows.Next() {
		var name, ty string
		if err := rows.Scan(&name, &ty); err != nil {
			return nil, nil, err
		}
		names = append(names, name)
		types = append(types, strings.ToUpper(ty))
	}
	return names, types, rows.Err()
}

// dumpSqlserverValue renders a scanned cell as a T-SQL literal. Unicode text
// uses the N'..' prefix so nvarchar columns keep non-ASCII; varbinary becomes
// a bare 0x.. literal (T-SQL has no _binary keyword).
func dumpSqlserverValue(v any, typeToken string) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case []byte:
		return "0x" + hexLower(val)
	case string:
		return "N'" + strings.ReplaceAll(val, "'", "''") + "'"
	case bool:
		if val {
			return "1"
		}
		return "0"
	case time.Time:
		return "'" + val.Format("2006-01-02 15:04:05.999999") + "'"
	default:
		return scanToString(v)
	}
}
