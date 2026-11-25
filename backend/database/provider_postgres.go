package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	// pgx (stdlib adapter) replaces lib/pq: same database/sql surface and
	// lib/pq-compatible DSN keywords, actively maintained, and — unlike
	// lib/pq — it exposes a custom dial function needed for proxy support.
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/sync/errgroup"
)

type postgresProvider struct{}

// postgresSchemaCache memoises GetTableSchema results per "dbName.tableName".
// The schema-tree panel walks every table on every refresh; without this cache
// each browse issues 3*N serial round-trips even when nothing changed.
var postgresSchemaCache sync.Map

func init() {
	Register("postgres", &postgresProvider{})
}

func (p *postgresProvider) DSN(host string, port int, user, password, dbName string, extraParams map[string]string) string {
	if port <= 0 {
		port = 5432
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   "/" + dbName,
	}
	q := u.Query()
	// Default to "prefer": encrypted when the server has TLS, plaintext
	// fallback when it doesn't. Callers can override via extraParams
	// (e.g. "sslmode=require" to refuse non-TLS, "sslmode=disable" to
	// force plaintext against self-signed dev servers).
	if _, ok := extraParams["sslmode"]; !ok {
		q.Set("sslmode", "prefer")
	}
	for k, v := range extraParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (p *postgresProvider) DriverName() string {
	return "pgx"
}

func (p *postgresProvider) Quote(name string) string {
	q, _ := SafePgIdent(name)
	return q
}

// QualifyTable quotes a table identifier that may arrive schema-qualified
// ("sales.orders" from the schema-grouped tree) or bare.
// Column and index names never pass through here — they stay plain Quote.
func (p *postgresProvider) QualifyTable(name string) string {
	schema, table := splitQualifiedName(name)
	if schema == "" {
		return p.Quote(table)
	}
	return p.Quote(schema) + "." + p.Quote(table)
}

// qualifyWithNodeSchema qualifies a bare table name with the schema carried
// by a composite "db.schema" node name so CREATE-like operations land inside
// the schema node the user opened, not the session search_path default.
func (p *postgresProvider) qualifyWithNodeSchema(dbName, tableName string) string {
	_, nodeSchema := splitDbNode(dbName)
	if nodeSchema == "" {
		return tableName
	}
	if schema, _ := splitQualifiedName(tableName); schema != "" {
		return tableName
	}
	return nodeSchema + "." + tableName
}

func (p *postgresProvider) PrepareExec(db execer, dbName string) error {
	return nil
}

func (p *postgresProvider) DefaultTableQuery(dbName, tableName string, limit, offset int) string {
	if offset > 0 {
		return fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", p.QualifyTable(tableName), limit, offset)
	}
	return fmt.Sprintf("SELECT * FROM %s LIMIT %d", p.QualifyTable(tableName), limit)
}

func (p *postgresProvider) PagedTableQuery(dbName, tableName string, limit, offset int) string {
	return fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", p.QualifyTable(tableName), limit, offset)
}

func (p *postgresProvider) InsertRow(db *sql.DB, dbName, tableName string, values map[string]any) error {
	cols := sortedKeys(values)
	quotedCols := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	args := make([]any, 0, len(cols))
	for i, c := range cols {
		quotedCols[i] = p.Quote(c)
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args = append(args, values[c])
	}
	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		p.QualifyTable(tableName), strings.Join(quotedCols, ", "), strings.Join(placeholders, ", "))
	return execPrepared(p, db, dbName, sql, args)
}

func (p *postgresProvider) UpdateRow(db *sql.DB, dbName, tableName string, set, where map[string]any) error {
	args := make([]any, 0, len(set)+len(where))
	phIdx := 1

	setParts := make([]string, 0, len(set))
	for _, c := range sortedKeys(set) {
		if set[c] == nil {
			setParts = append(setParts, fmt.Sprintf("%s = NULL", p.Quote(c)))
		} else {
			setParts = append(setParts, fmt.Sprintf("%s = $%d", p.Quote(c), phIdx))
			args = append(args, set[c])
			phIdx++
		}
	}

	whereParts := make([]string, 0, len(where))
	for _, c := range sortedKeys(where) {
		if where[c] == nil {
			whereParts = append(whereParts, fmt.Sprintf("%s IS NULL", p.Quote(c)))
		} else {
			whereParts = append(whereParts, fmt.Sprintf("%s = $%d", p.Quote(c), phIdx))
			args = append(args, where[c])
			phIdx++
		}
	}

	sql := fmt.Sprintf("UPDATE %s SET %s", p.QualifyTable(tableName), strings.Join(setParts, ", "))
	if len(whereParts) > 0 {
		sql += " WHERE " + strings.Join(whereParts, " AND ")
	}
	return execPrepared(p, db, dbName, sql, args)
}

func (p *postgresProvider) DeleteRow(db *sql.DB, dbName, tableName string, where map[string]any) error {
	args := make([]any, 0, len(where))
	phIdx := 1
	whereParts := make([]string, 0, len(where))
	for _, c := range sortedKeys(where) {
		if where[c] == nil {
			whereParts = append(whereParts, fmt.Sprintf("%s IS NULL", p.Quote(c)))
		} else {
			whereParts = append(whereParts, fmt.Sprintf("%s = $%d", p.Quote(c), phIdx))
			args = append(args, where[c])
			phIdx++
		}
	}

	sql := fmt.Sprintf("DELETE FROM %s", p.QualifyTable(tableName))
	if len(whereParts) > 0 {
		sql += " WHERE " + strings.Join(whereParts, " AND ")
	}
	return execPrepared(p, db, dbName, sql, args)
}

func (p *postgresProvider) GetCapabilities() DBCapabilities {
	return DBCapabilities{
		"supportsAutoIncrement":      false,
		"supportsOnUpdate":           false,
		"supportsCollation":          false,
		"autoIncrementForcesNotNull": false,
		"columnTypes":                postgresTypes,
		"intTypes":                   postgresIntTypes,
		// public is droppable in PostgreSQL, so it is NOT in
		// undroppableSchemas (unlike SQL Server's dbo).
		"supportsSchemas": true,
		"defaultSchema":   "public",
	}
}

// ── Schema discovery ──

// GetDatabases returns only the currently connected database. A PostgreSQL
// connection is bound to a single database and cannot query tables across
// databases, so exposing the whole cluster would let users open other
// databases and see "no tables". To browse a different database, open a new
// connection with that database name.
func (p *postgresProvider) GetDatabases(db *sql.DB) ([]string, error) {
	results, err := queryStrings(db, "SELECT current_database() AS datname")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(results))
	for _, row := range results {
		if row["datname"] != "" {
			names = append(names, row["datname"])
		}
	}
	return names, nil
}

// GetSchemas lists the user namespaces of the connected database
//). Non-empty return = the tree renders a second level (db → schema →
// tables). System namespaces are filtered; public sorts first.
func (p *postgresProvider) GetSchemas(db *sql.DB, dbName string) ([]string, error) {
	results, err := queryStrings(db, `
		SELECT nspname AS schema_name
		FROM pg_namespace
		WHERE nspname NOT IN ('pg_catalog', 'information_schema')
		  AND nspname NOT LIKE 'pg\_%'
		ORDER BY (nspname = 'public') DESC, nspname`)
	if err != nil {
		return nil, fmt.Errorf("list schemas: %w", err)
	}
	names := make([]string, 0, len(results))
	for _, row := range results {
		names = append(names, row["schema_name"])
	}
	return names, nil
}

// tableListBase builds the table listing body shared by GetTables and
// GetTablePage (PostgreSQL flavor). When dbName carries a
// schema qualifier ("db.schema"), only that schema's objects are listed and
// table_name comes back schema-qualified; a bare dbName keeps the legacy
// flat listing that excludes system namespaces.
func (p *postgresProvider) tableListBase(dbName string) (string, []any, int) {
	_, schema := splitDbNode(dbName)
	var args []any
	filter, nameExpr := ` AND n.nspname NOT IN ('pg_catalog', 'information_schema')`, "c.relname AS table_name"
	if schema != "" {
		filter = " AND n.nspname = $1"
		args = append(args, schema)
		nameExpr = "n.nspname || '.' || c.relname AS table_name"
	}
	base := `
		SELECT ` + nameExpr + `,
		       CASE WHEN c.relkind = 'v' THEN 'VIEW' ELSE 'BASE TABLE' END AS table_type,
		       COALESCE(obj_description(c.oid, 'pg_class'), '') AS table_comment
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind IN ('r', 'p', 'v')` + filter
	return base, args, len(args) + 1
}

func (p *postgresProvider) GetTables(db *sql.DB, dbName string) ([]TableInfo, error) {
	base, args, _ := p.tableListBase(dbName)
	results, err := queryStrings(db, base+"\n\t\tORDER BY c.relname", args...)
	if err != nil {
		return nil, fmt.Errorf("get tables: %w", err)
	}
	infos := make([]TableInfo, 0, len(results))
	for _, row := range results {
		tp := "table"
		if row["table_type"] == "VIEW" {
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

func (p *postgresProvider) GetTablePage(db *sql.DB, dbName string, query string, offset, limit int) ([]TableInfo, int, error) {
	// Placeholder numbers continue after the schema arg ($1 when the node is
	// "db.schema"); the driver maps $N positionally, so numbering must stay
	// contiguous and unique per statement.
	base, baseArgs, next := p.tableListBase(dbName)

	countSQL := "SELECT COUNT(*) AS n FROM (" + base + ") c"
	countArgs := append([]any{}, baseArgs...)
	if query != "" {
		countSQL += fmt.Sprintf(" WHERE table_name ILIKE $%d", next)
		countArgs = append(countArgs, "%"+query+"%")
	}
	countRows, err := queryStrings(db, countSQL, countArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("count tables: %w", err)
	}
	total := 0
	if len(countRows) > 0 {
		total, _ = strconv.Atoi(countRows[0]["n"])
	}

	pageSQL := "SELECT table_name, table_type, table_comment FROM (" + base + ") c"
	pageArgs := append([]any{}, baseArgs...)
	if query != "" {
		pageSQL += fmt.Sprintf(" WHERE table_name ILIKE $%d", next)
		pageArgs = append(pageArgs, "%"+query+"%")
		pageSQL += fmt.Sprintf(" ORDER BY table_name LIMIT $%d OFFSET $%d", next+1, next+2)
		pageArgs = append(pageArgs, limit, offset)
	} else {
		pageSQL += fmt.Sprintf(" ORDER BY table_name LIMIT $%d OFFSET $%d", next, next+1)
		pageArgs = append(pageArgs, limit, offset)
	}
	results, err := queryStrings(db, pageSQL, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("get tables page: %w", err)
	}
	infos := make([]TableInfo, 0, len(results))
	for _, row := range results {
		tp := "table"
		if row["table_type"] == "VIEW" {
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

func (p *postgresProvider) GetTableSchema(db *sql.DB, dbName, tableName string) (*SchemaResult, error) {
	cacheKey := dbName + "." + tableName
	if cached, ok := postgresSchemaCache.Load(cacheKey); ok {
		return cached.(*SchemaResult), nil
	}

	// the tree hands out schema-qualified names ("sales.orders");
	// split and filter by namespace so same-named tables in different schemas
	// resolve correctly.
	schema, table := splitQualifiedName(tableName)
	var (
		colRows []map[string]string
		pkRows  []map[string]string
		idxRows []map[string]string
	)
	g, _ := errgroup.WithContext(context.Background())
	g.Go(func() error {
		colFilter, colArgs := "", []any{table}
		if schema != "" {
			colFilter = " AND c.table_schema = $2"
			colArgs = append(colArgs, schema)
		}
		r, err := queryStrings(db,
			`SELECT c.column_name, c.data_type, c.is_nullable, c.column_default,
			        COALESCE(pgd.description, '') AS column_comment
			 FROM information_schema.columns c
			 LEFT JOIN pg_catalog.pg_statio_all_tables st
			   ON c.table_schema = st.schemaname AND c.table_name = st.relname
			 LEFT JOIN pg_catalog.pg_description pgd
			   ON pgd.objoid = st.relid AND pgd.objsubid = c.ordinal_position
			 WHERE c.table_name = $1`+colFilter+`
			 ORDER BY c.ordinal_position`,
			colArgs...,
		)
		if err != nil {
			return fmt.Errorf("get columns: %w", err)
		}
		colRows = r
		return nil
	})
	g.Go(func() error {
		pkQuery, pkArgs := "SELECT a.attname FROM pg_index i JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey) WHERE i.indrelid = $1::regclass AND i.indisprimary", []any{table}
		if schema != "" {
			pkQuery = "SELECT a.attname FROM pg_index i JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey) JOIN pg_class t ON t.oid = i.indrelid JOIN pg_namespace n ON n.oid = t.relnamespace WHERE t.relname = $1 AND n.nspname = $2 AND i.indisprimary"
			pkArgs = []any{table, schema}
		}
		r, err := queryStrings(db, pkQuery, pkArgs...)
		if err != nil {
			return nil // PKs are non-fatal like before
		}
		pkRows = r
		return nil
	})
	g.Go(func() error {
		idxQuery := "SELECT i.relname AS index_name, ix.indisunique AS is_unique, ix.indisprimary AS is_primary, a.attname AS column_name FROM pg_class t JOIN pg_index ix ON t.oid = ix.indrelid JOIN pg_class i ON i.oid = ix.indexrelid JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey) JOIN pg_namespace n ON n.oid = t.relnamespace WHERE t.relname = $1"
		idxArgs := []any{table}
		if schema != "" {
			idxQuery += " AND n.nspname = $2"
			idxArgs = append(idxArgs, schema)
		}
		r, err := queryStrings(db, idxQuery+" ORDER BY i.relname, a.attnum", idxArgs...)
		if err != nil {
			return fmt.Errorf("get indexes: %w", err)
		}
		idxRows = r
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	columns := make([]ColumnInfo, 0, len(colRows))
	for _, row := range colRows {
		nullable := row["is_nullable"] == "YES"
		defVal := row["column_default"]
		defaultType := "none"
		if defVal == "" && nullable {
			defaultType = "null"
			defVal = "NULL"
		} else if defVal != "" {
			defaultType = "value"
		}
		columns = append(columns, ColumnInfo{
			Name:        row["column_name"],
			Type:        row["data_type"],
			Nullable:    nullable,
			DefaultVal:  defVal,
			DefaultType: defaultType,
			IsPrimary:   false,
			Comment:     row["column_comment"],
		})
	}

	for _, row := range pkRows {
		for i := range columns {
			if columns[i].Name == row["attname"] {
				columns[i].IsPrimary = true
			}
		}
	}

	idxMap := make(map[string]*IndexInfo)
	var idxOrder []string
	for _, row := range idxRows {
		name := row["index_name"]
		if _, ok := idxMap[name]; !ok {
			idxMap[name] = &IndexInfo{
				Name:      name,
				Columns:   []string{},
				Unique:    row["is_unique"] == "t",
				IsPrimary: row["is_primary"] == "t",
			}
			idxOrder = append(idxOrder, name)
		}
		idxMap[name].Columns = append(idxMap[name].Columns, row["column_name"])
	}

	indexes := make([]IndexInfo, 0, len(idxOrder))
	for _, name := range idxOrder {
		indexes = append(indexes, *idxMap[name])
	}

	result := &SchemaResult{Columns: columns, Indexes: indexes}
	postgresSchemaCache.Store(cacheKey, result)
	return result, nil
}

// ── DDL: Database ──

// CreateDatabase accepts a composite "db.schema" name: the
// schema part is optional and created inside the connected database; empty
// means "no extra schema" (tables land in the session search_path default,
// usually public). A pre-existing database is tolerated when a schema is
// requested; both already present is reported as a duplicate.
func (p *postgresProvider) CreateDatabase(db *sql.DB, dbName string) error {
	useDb, schema := splitDbNode(dbName)
	q := p.Quote
	var dbCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pg_database WHERE datname = $1`, useDb).Scan(&dbCount); err != nil {
		return err
	}
	if dbCount == 0 {
		if _, err := db.Exec(fmt.Sprintf("CREATE DATABASE %s", q(useDb))); err != nil {
			return err
		}
	} else if schema == "" {
		return fmt.Errorf("database %s already exists", useDb)
	}
	if schema == "" {
		return nil
	}
	if dbCount > 0 {
		var schemaCount int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pg_namespace WHERE nspname = $1`, schema).Scan(&schemaCount); err != nil {
			return err
		}
		if schemaCount > 0 {
			return fmt.Errorf("schema %s already exists", schema)
		}
	}
	_, err := db.Exec(fmt.Sprintf("CREATE SCHEMA %s", q(schema)))
	return err
}

func (p *postgresProvider) DropDatabase(db *sql.DB, dbName string) error {
	// The tree hands out composite "db.schema" node names; a database can
	// only be dropped by its bare name.
	useDb, _ := splitDbNode(dbName)
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("DROP DATABASE %s", q(useDb)))
	return err
}

// DropSchema drops the schema part of a composite "db.schema" node name.
// PostgreSQL refuses to drop a schema that still owns objects; that driver
// error is surfaced as-is so the user knows to empty the schema first.
func (p *postgresProvider) DropSchema(db *sql.DB, dbName string) error {
	_, schema := splitDbNode(dbName)
	if schema == "" {
		return fmt.Errorf("node %q carries no schema qualifier", dbName)
	}
	_, err := db.Exec(fmt.Sprintf("DROP SCHEMA %s", p.Quote(schema)))
	return err
}

// ── DDL: Table ──

func (p *postgresProvider) CreateTable(db *sql.DB, dbName, tableName string) error {
	// A new table created inside a "db.schema" node must land in that schema
	//, not the session search_path default.
	name := p.qualifyWithNodeSchema(dbName, tableName)
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY)", p.QualifyTable(name)))
	return err
}

func (p *postgresProvider) DropTable(db *sql.DB, dbName, tableName string) error {
	_, err := db.Exec(fmt.Sprintf("DROP TABLE %s", p.QualifyTable(tableName)))
	return err
}

func (p *postgresProvider) DropView(db *sql.DB, dbName, viewName string) error {
	_, err := db.Exec(fmt.Sprintf("DROP VIEW %s", p.QualifyTable(viewName)))
	return err
}

func (p *postgresProvider) TruncateTable(db *sql.DB, dbName, tableName string) error {
	_, err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s", p.QualifyTable(tableName)))
	return err
}

// CopyTable clones structure and data in one statement. Both sides are
// schema-qualified: the source arrives qualified from the tree, and the
// clone lands in the same schema node.
func (p *postgresProvider) CopyTable(db *sql.DB, dbName, tableName, newTableName string) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	target := p.qualifyWithNodeSchema(dbName, newTableName)
	_, err = conn.ExecContext(ctx, fmt.Sprintf("CREATE TABLE %s AS SELECT * FROM %s",
		p.QualifyTable(target), p.QualifyTable(tableName)))
	return err
}

// ── DDL: Column ──

func (p *postgresProvider) AddColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
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
			def, err := SafeDefaultLiteral(col.DefaultVal)
			if err != nil {
				return err
			}
			parts = append(parts, "DEFAULT "+def)
		} else {
			parts = append(parts, "DEFAULT ''")
		}
	}

	sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", p.QualifyTable(tableName), strings.Join(parts, " "))
	_, err := db.Exec(sql)
	return err
}

func (p *postgresProvider) ModifyColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
	q := p.Quote
	var stmts []string

	stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s",
		p.QualifyTable(tableName), q(col.Name), col.Type))

	if col.Nullable {
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL",
			p.QualifyTable(tableName), q(col.Name)))
	} else {
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL",
			p.QualifyTable(tableName), q(col.Name)))
	}

	switch col.DefaultType {
	case "null":
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT NULL",
			p.QualifyTable(tableName), q(col.Name)))
	case "value":
		if col.DefaultVal != "" {
			def, err := SafeDefaultLiteral(col.DefaultVal)
			if err != nil {
				return err
			}
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s",
				p.QualifyTable(tableName), q(col.Name), def))
		} else {
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT ''",
				p.QualifyTable(tableName), q(col.Name)))
		}
	default:
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT",
			p.QualifyTable(tableName), q(col.Name)))
	}

	if col.Comment != "" {
		stmts = append(stmts, fmt.Sprintf("COMMENT ON COLUMN %s.%s IS '%s'",
			p.QualifyTable(tableName), q(col.Name), strings.ReplaceAll(col.Comment, "'", "''")))
	}

	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func (p *postgresProvider) DropColumn(db *sql.DB, dbName, tableName, colName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", p.QualifyTable(tableName), q(colName)))
	return err
}

// ── DDL: Index ──

func (p *postgresProvider) AddIndex(db *sql.DB, dbName, tableName string, idx IndexDef) error {
	q := p.Quote
	var sql string
	if idx.IsPrimary {
		cols := make([]string, len(idx.Columns))
		for i, c := range idx.Columns {
			cols[i] = q(c)
		}
		sql = fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY (%s)", p.QualifyTable(tableName), strings.Join(cols, ", "))
	} else {
		uniqueStr := ""
		if idx.Unique {
			uniqueStr = "UNIQUE "
		}
		cols := make([]string, len(idx.Columns))
		for i, c := range idx.Columns {
			cols[i] = q(c)
		}
		sql = fmt.Sprintf("CREATE %sINDEX %s ON %s (%s)", uniqueStr, q(idx.Name), p.QualifyTable(tableName), strings.Join(cols, ", "))
	}
	_, err := db.Exec(sql)
	return err
}

func (p *postgresProvider) DropIndex(db *sql.DB, dbName, tableName, idxName string, isPrimary bool, autoIncCols []string) error {
	q := p.Quote
	if isPrimary {
		_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", p.QualifyTable(tableName), q(idxName)))
		return err
	}
	_, err := db.Exec(fmt.Sprintf("DROP INDEX %s", q(idxName)))
	return err
}

var postgresTypes = []string{
	"SMALLINT", "INTEGER", "BIGINT",
	"SERIAL", "BIGSERIAL", "SMALLSERIAL",
	"REAL", "DOUBLE PRECISION",
	"DECIMAL", "DECIMAL(10,2)",
	"NUMERIC", "NUMERIC(10,2)",
	"MONEY",
	"CHAR", "CHAR(1)",
	"VARCHAR", "VARCHAR(255)",
	"TEXT",
	"BYTEA",
	"DATE", "TIMESTAMP", "TIMESTAMPTZ", "TIME", "TIMETZ", "INTERVAL",
	"BOOLEAN", "JSON", "JSONB",
	"UUID", "INET", "CIDR", "MACADDR",
	"XML",
}

var postgresIntTypes = []string{
	"SMALLINT", "INTEGER", "BIGINT", "SERIAL", "BIGSERIAL", "SMALLSERIAL",
}

func (p *postgresProvider) DumpTable(db *sql.DB, dbName, tableName string, opts DumpOptions) (string, error) {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	// A schema-qualified table name ("sales.orders") carries its
	// own schema; bare names resolve via the session search_path.
	dumpSchema, dumpTable := splitQualifiedName(tableName)
	schema := dumpSchema
	if schema == "" {
		schema = "public"
		if curSchema, err := pgCurrentSchema(conn); err == nil && curSchema != "" {
			schema = curSchema
		}
	}

	var b strings.Builder
	quotedTable := p.QualifyTable(tableName)

	if opts.Structure {
		createSQL, derr := pgCreateTableSQL(conn, schema, dumpTable, quotedTable)
		if derr != nil {
			return "", derr
		}
		b.WriteString("--\n-- Structure for ")
		b.WriteString(tableName)
		b.WriteString("\n--\nDROP TABLE IF EXISTS ")
		b.WriteString(quotedTable)
		b.WriteString(" CASCADE;\n")
		b.WriteString(createSQL)
		if !strings.HasSuffix(createSQL, ";") {
			b.WriteByte(';')
		}
		b.WriteString("\n\n")
	}

	if opts.Data {
		cols, colTypes, derr := pgTableColumns(conn, schema, dumpTable)
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
					b.WriteString(dumpPgValue(v, colTypes[i]))
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

// pgCurrentSchema returns the first schema on the session search_path.
func pgCurrentSchema(conn *sql.Conn) (string, error) {
	var s string
	err := conn.QueryRowContext(context.Background(), "SELECT current_schema()").Scan(&s)
	return s, err
}

// pgCreateTableSQL builds a CREATE TABLE statement from pg_catalog, including
// column types (via format_type), defaults, NOT NULL, and table constraints
// (PK / UNIQUE / CHECK / FK) via pg_get_constraintdef.
func pgCreateTableSQL(conn *sql.Conn, schema, tableName, quotedTable string) (string, error) {
	rows, err := conn.QueryContext(context.Background(), `
		SELECT a.attname,
		       format_type(a.atttypid, a.atttypmod),
		       a.attnotnull,
		       pg_get_expr(ad.adbin, ad.adrelid) AS default
		FROM pg_attribute a
		JOIN pg_class t ON a.attrelid = t.oid
		JOIN pg_namespace n ON t.relnamespace = n.oid
		LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
		WHERE t.relname = $1 AND n.nspname = $2
		  AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum`, tableName, schema)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var b strings.Builder
	b.WriteString("CREATE TABLE ")
	b.WriteString(quotedTable)
	b.WriteString(" (\n")
	colLines := []string{}
	for rows.Next() {
		var name, ty string
		var notnull bool
		var dflt sql.NullString
		if err := rows.Scan(&name, &ty, &notnull, &dflt); err != nil {
			return "", err
		}
		quotedName, _ := SafePgIdent(name)
		line := "  " + quotedName + " " + ty
		if dflt.Valid && dflt.String != "" {
			line += " DEFAULT " + dflt.String
		}
		if notnull {
			line += " NOT NULL"
		}
		colLines = append(colLines, line)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	constraints, err := pgConstraintDefs(conn, schema, tableName)
	if err != nil {
		return "", err
	}
	colLines = append(colLines, constraints...)

	b.WriteString(strings.Join(colLines, ",\n"))
	b.WriteString("\n)")
	return b.String(), nil
}

// pgConstraintDefs returns table constraint clauses (PK/UNIQUE/CHECK/FK)
// via pg_get_constraintdef.
func pgConstraintDefs(conn *sql.Conn, schema, tableName string) ([]string, error) {
	rows, err := conn.QueryContext(context.Background(), `
		SELECT pg_get_constraintdef(c.oid)
		FROM pg_constraint c
		JOIN pg_class t ON c.conrelid = t.oid
		JOIN pg_namespace n ON t.relnamespace = n.oid
		WHERE t.relname = $1 AND n.nspname = $2
		ORDER BY c.contype`, tableName, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var def sql.NullString
		if err := rows.Scan(&def); err != nil {
			return nil, err
		}
		if def.Valid && def.String != "" {
			out = append(out, "  "+def.String)
		}
	}
	return out, rows.Err()
}

// pgTableColumns returns column names and types for a table.
func pgTableColumns(conn *sql.Conn, schema, tableName string) ([]string, []string, error) {
	rows, err := conn.QueryContext(context.Background(), `
		SELECT a.attname, format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class t ON a.attrelid = t.oid
		JOIN pg_namespace n ON t.relnamespace = n.oid
		WHERE t.relname = $1 AND n.nspname = $2
		  AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum`, tableName, schema)
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
		types = append(types, ty)
	}
	return names, types, rows.Err()
}

// dumpPgValue renders a scanned cell as a PostgreSQL literal. Booleans become
// TRUE/FALSE; bytea becomes '\x..'; timestamps drop the T/Z separators.
func dumpPgValue(v any, typeToken string) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case []byte:
		return "'\\x" + hexLower(val) + "'"
	case string:
		return quotedString(val)
	case bool:
		if val {
			return "TRUE"
		}
		return "FALSE"
	case time.Time:
		return "'" + val.Format("2006-01-02 15:04:05.999999") + "'"
	default:
		return scanToString(v)
	}
}
