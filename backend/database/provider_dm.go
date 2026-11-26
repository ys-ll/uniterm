package database

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	// Pure-Go database/sql driver for DM8 (registers as "dm"). Third-party
	// reimplementation of the DM wire protocol, pinned to our GitHub mirror.
	// Imported non-blank for the *dm.DmClob scan fallback in DumpTable.
	dm "github.com/ys-ll/dm"
)

type dmProvider struct{}

func init() {
	Register("dm", &dmProvider{})
}

// dmSystemSchemas lists the DM-maintained schemas that hold no user tables,
// kept out of the schema tree (mirrors the Oracle provider's systemSchemas).
var dmSystemSchemas = []string{"SYS", "SYSSSO", "SYSAUDITOR", "CTISYS", "OUTLN"}

func (p *dmProvider) DSN(host string, port int, user, password, dbName string, extraParams map[string]string) string {
	if port <= 0 {
		port = 5236
	}
	u := &url.URL{
		Scheme: "dm",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
	}
	if len(extraParams) > 0 {
		q := u.Query()
		for k, v := range extraParams {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func (p *dmProvider) DriverName() string {
	return "dm"
}

func (p *dmProvider) Quote(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// PrepareExec switches the session schema so unqualified table names in user
// SQL resolve against the selected tree node. DM stores one database per
// instance (Oracle-like); the tree's database level is the schema/user.
func (p *dmProvider) PrepareExec(db execer, dbName string) error {
	if dbName == "" {
		return nil
	}
	// Only switch when the name really exists as a schema; otherwise keep the
	// session on the login user's default schema instead of failing every
	// query on a non-existent schema (same guard as the Oracle provider).
	rows, err := db.QueryContext(context.Background(),
		"SELECT COUNT(*) FROM SYS.SYSOBJECTS WHERE TYPE$ = 'SCH' AND NAME = ?", strings.ToUpper(dbName))
	if err != nil {
		// The catalog probe itself should never block user SQL; on failure
		// just try the switch and let it fail there if the schema is bogus.
	} else {
		var n int
		if rows.Next() {
			_ = rows.Scan(&n)
		}
		rows.Close()
		if n == 0 {
			return nil
		}
	}
	_, err = db.ExecContext(context.Background(), fmt.Sprintf("SET SCHEMA %s", p.Quote(strings.ToUpper(dbName))))
	return err
}

func (p *dmProvider) DefaultTableQuery(dbName, tableName string, limit, offset int) string {
	if offset > 0 {
		return fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", p.Quote(tableName), limit, offset)
	}
	return fmt.Sprintf("SELECT * FROM %s LIMIT %d", p.Quote(tableName), limit)
}

func (p *dmProvider) PagedTableQuery(dbName, tableName string, limit, offset int) string {
	return fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", p.Quote(tableName), limit, offset)
}

// ── Row-level CRUD ──

func (p *dmProvider) InsertRow(db *sql.DB, dbName, tableName string, values map[string]any) error {
	cols := sortedKeys(values)
	quotedCols := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	args := make([]any, 0, len(cols))
	for i, c := range cols {
		quotedCols[i] = p.Quote(c)
		placeholders[i] = "?"
		args = append(args, values[c])
	}
	stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		p.Quote(tableName), strings.Join(quotedCols, ", "), strings.Join(placeholders, ", "))
	return execPrepared(p, db, dbName, stmt, args)
}

func (p *dmProvider) UpdateRow(db *sql.DB, dbName, tableName string, set, where map[string]any) error {
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

	stmt := fmt.Sprintf("UPDATE %s SET %s", p.Quote(tableName), strings.Join(setParts, ", "))
	if len(whereParts) > 0 {
		stmt += " WHERE " + strings.Join(whereParts, " AND ")
	}
	return execPrepared(p, db, dbName, stmt, args)
}

func (p *dmProvider) DeleteRow(db *sql.DB, dbName, tableName string, where map[string]any) error {
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

	stmt := fmt.Sprintf("DELETE FROM %s", p.Quote(tableName))
	if len(whereParts) > 0 {
		stmt += " WHERE " + strings.Join(whereParts, " AND ")
	}
	return execPrepared(p, db, dbName, stmt, args)
}

// ── Capabilities ──

func (p *dmProvider) GetCapabilities() DBCapabilities {
	return DBCapabilities{
		"supportsAutoIncrement":      true,
		"supportsOnUpdate":           false,
		"supportsCollation":          false,
		"supportsCreateDatabase":     false,
		"autoIncrementForcesNotNull": true,
		"columnTypes":                dmColumnTypes,
		"intTypes":                   dmIntTypes,
	}
}

// ── Schema discovery ──

func (p *dmProvider) GetDatabases(db *sql.DB) ([]string, error) {
	results, err := queryStrings(db, "SELECT USER AS SCHEMA_NAME FROM DUAL")
	if err != nil {
		return nil, err
	}
	current := ""
	if len(results) > 0 {
		current = results[0]["SCHEMA_NAME"]
	}

	excluded := "'" + strings.Join(dmSystemSchemas, `', '`) + "'"
	owners, err := queryStrings(db, fmt.Sprintf(`SELECT DISTINCT owner FROM all_objects
		WHERE object_type IN ('TABLE', 'VIEW')
		  AND owner NOT IN (%s)
		ORDER BY owner`, excluded))
	if err != nil {
		return nil, fmt.Errorf("list schemas: %w", err)
	}

	// Keep the current schema first: resolveSchema falls back to the first
	// entry and the tree panel pins it to the top.
	schemas := make([]string, 0, len(owners)+1)
	if current != "" {
		schemas = append(schemas, current)
	}
	for _, row := range owners {
		if o := row["OWNER"]; o != "" && !strings.EqualFold(o, current) {
			schemas = append(schemas, o)
		}
	}
	return schemas, nil
}

// GetSchemas: DM has no inner-schema concept (the tree node itself is the
// schema), so the tree stays single-level.
func (p *dmProvider) GetSchemas(db *sql.DB, dbName string) ([]string, error) {
	return nil, nil
}

func (p *dmProvider) GetTables(db *sql.DB, dbName string) ([]TableInfo, error) {
	owner, err := p.resolveSchema(db, dbName)
	if err != nil {
		return nil, err
	}
	results, err := queryStrings(db, `
		SELECT o.object_name, o.object_type, NVL(c.comments, '') AS comments
		FROM all_objects o
		LEFT JOIN all_tab_comments c
		  ON c.owner = o.owner AND c.table_name = o.object_name
		WHERE o.owner = ?
		  AND o.object_type IN ('TABLE', 'VIEW')
		  AND o.object_name NOT LIKE 'BIN$%'
		  AND o.object_name NOT LIKE '##%'
		ORDER BY o.object_name`, strings.ToUpper(owner))
	if err != nil {
		return nil, fmt.Errorf("get tables: %w", err)
	}
	return dmTableInfos(results), nil
}

// GetTablePage returns one sorted page of tables plus the total count. A
// non-empty query filters by name case-insensitively (LIKE %q%); paging uses
// DM's MySQL-style LIMIT/OFFSET.
func (p *dmProvider) GetTablePage(db *sql.DB, dbName string, query string, offset, limit int) ([]TableInfo, int, error) {
	owner, err := p.resolveSchema(db, dbName)
	if err != nil {
		return nil, 0, err
	}
	owner = strings.ToUpper(owner)

	where := `o.owner = ? AND o.object_type IN ('TABLE', 'VIEW') AND o.object_name NOT LIKE 'BIN$%' AND o.object_name NOT LIKE '##%'`
	countArgs := []any{owner}
	pageArgs := []any{owner}
	if query != "" {
		where += ` AND UPPER(o.object_name) LIKE ?`
		pattern := "%" + strings.ToUpper(query) + "%"
		countArgs = append(countArgs, pattern)
		pageArgs = append(pageArgs, pattern)
	}

	countRows, err := queryStrings(db, `
		SELECT COUNT(*) AS n
		FROM all_objects o
		WHERE `+where, countArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("count tables: %w", err)
	}
	total := 0
	if len(countRows) > 0 {
		total, _ = strconv.Atoi(countRows[0]["N"])
	}

	results, err := queryStrings(db, fmt.Sprintf(`
		SELECT o.object_name, o.object_type, NVL(c.comments, '') AS comments
		FROM all_objects o
		LEFT JOIN all_tab_comments c
		  ON c.owner = o.owner AND c.table_name = o.object_name
		WHERE %s
		ORDER BY o.object_name
		LIMIT %d OFFSET %d`, where, offset+limit, offset), pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("get tables page: %w", err)
	}
	return dmTableInfos(results), total, nil
}

func dmTableInfos(results []map[string]string) []TableInfo {
	infos := make([]TableInfo, 0, len(results))
	for _, row := range results {
		tp := "table"
		if row["OBJECT_TYPE"] == "VIEW" {
			tp = "view"
		}
		infos = append(infos, TableInfo{
			Name:    row["OBJECT_NAME"],
			Type:    tp,
			Comment: row["COMMENTS"],
		})
	}
	return infos
}

func (p *dmProvider) GetTableSchema(db *sql.DB, dbName, tableName string) (*SchemaResult, error) {
	owner, err := p.resolveSchema(db, dbName)
	if err != nil {
		return nil, err
	}
	owner = strings.ToUpper(owner)
	table := strings.ToUpper(tableName)

	colRows, err := queryStrings(db, `
		SELECT c.column_name, c.data_type, c.data_length, c.data_precision, c.data_scale, c.nullable, c.data_default,
		       NVL(cc.comments, '') AS comments
		FROM all_tab_columns c
		LEFT JOIN all_col_comments cc
		  ON cc.owner = c.owner AND cc.table_name = c.table_name AND cc.column_name = c.column_name
		WHERE c.owner = ? AND c.table_name = ?
		ORDER BY c.column_id`, owner, table)
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	pkCols, err := dmPrimaryColumns(db, owner, table)
	if err != nil {
		return nil, fmt.Errorf("get primary keys: %w", err)
	}

	columns := make([]ColumnInfo, 0, len(colRows))
	for _, row := range colRows {
		nullable := row["NULLABLE"] == "Y"
		defVal := strings.TrimSpace(row["DATA_DEFAULT"])
		defaultType := "none"
		if defVal == "" && nullable {
			defaultType = "null"
			defVal = "NULL"
		} else if strings.EqualFold(defVal, "NULL") {
			defaultType = "null"
		} else if defVal != "" {
			defaultType = "value"
		}
		columns = append(columns, ColumnInfo{
			Name:        row["COLUMN_NAME"],
			Type:        oracleColumnType(row),
			Nullable:    nullable,
			DefaultVal:  defVal,
			DefaultType: defaultType,
			IsPrimary:   pkCols[row["COLUMN_NAME"]],
			Comment:     row["COMMENTS"],
		})
	}

	indexes, err := dmIndexes(db, owner, table)
	if err != nil {
		return nil, fmt.Errorf("get indexes: %w", err)
	}

	return &SchemaResult{Columns: columns, Indexes: indexes}, nil
}

func dmPrimaryColumns(db *sql.DB, owner, tableName string) (map[string]bool, error) {
	rows, err := queryStrings(db, `
		SELECT cols.column_name
		FROM all_constraints cons
		JOIN all_cons_columns cols
		  ON cols.owner = cons.owner
		 AND cols.constraint_name = cons.constraint_name
		 AND cols.table_name = cons.table_name
		WHERE cons.owner = ?
		  AND cons.table_name = ?
		  AND cons.constraint_type = 'P'`, owner, tableName)
	if err != nil {
		return nil, err
	}
	cols := make(map[string]bool, len(rows))
	for _, row := range rows {
		cols[row["COLUMN_NAME"]] = true
	}
	return cols, nil
}

func dmIndexes(db *sql.DB, owner, tableName string) ([]IndexInfo, error) {
	rows, err := queryStrings(db, `
		SELECT NVL(cons.constraint_name, idx.index_name) AS index_name,
		       idx.uniqueness,
		       col.column_name,
		       CASE WHEN cons.constraint_type = 'P' THEN 'Y' ELSE 'N' END AS is_primary
		FROM all_indexes idx
		JOIN all_ind_columns col
		  ON col.index_owner = idx.owner
		 AND col.index_name = idx.index_name
		 AND col.table_owner = idx.table_owner
		 AND col.table_name = idx.table_name
		LEFT JOIN all_constraints cons
		  ON cons.owner = idx.table_owner
		 AND cons.table_name = idx.table_name
		 AND cons.index_owner = idx.owner
		 AND cons.index_name = idx.index_name
		 AND cons.constraint_type = 'P'
		WHERE idx.table_owner = ?
		  AND idx.table_name = ?
		ORDER BY index_name, col.column_position`, owner, tableName)
	if err != nil {
		return nil, err
	}

	idxMap := make(map[string]*IndexInfo)
	var idxOrder []string
	for _, row := range rows {
		name := row["INDEX_NAME"]
		if _, ok := idxMap[name]; !ok {
			idxMap[name] = &IndexInfo{
				Name:      name,
				Columns:   []string{},
				Unique:    row["UNIQUENESS"] == "UNIQUE",
				IsPrimary: row["IS_PRIMARY"] == "Y",
			}
			idxOrder = append(idxOrder, name)
		}
		idxMap[name].Columns = append(idxMap[name].Columns, row["COLUMN_NAME"])
	}

	indexes := make([]IndexInfo, 0, len(idxOrder))
	for _, name := range idxOrder {
		indexes = append(indexes, *idxMap[name])
	}
	return indexes, nil
}

func (p *dmProvider) resolveSchema(db *sql.DB, dbName string) (string, error) {
	if dbName != "" {
		return dbName, nil
	}
	dbs, err := p.GetDatabases(db)
	if err != nil {
		return "", err
	}
	if len(dbs) == 0 {
		return "", nil
	}
	return dbs[0], nil
}

// ── DDL: Database ──

func (p *dmProvider) CreateDatabase(db *sql.DB, dbName string) error {
	return fmt.Errorf("dm provider does not support CREATE DATABASE; create schemas outside uniTerm")
}

func (p *dmProvider) DropDatabase(db *sql.DB, dbName string) error {
	return fmt.Errorf("dm provider does not support DROP DATABASE; drop schemas outside uniTerm")
}

// ── DDL: Table ──

func (p *dmProvider) CreateTable(db *sql.DB, dbName, tableName string) error {
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE %s (id INT IDENTITY(1,1) PRIMARY KEY)", p.Quote(tableName)))
	return err
}

func (p *dmProvider) DropTable(db *sql.DB, dbName, tableName string) error {
	_, err := db.Exec(fmt.Sprintf("DROP TABLE %s", p.Quote(tableName)))
	return err
}

func (p *dmProvider) DropView(db *sql.DB, dbName, viewName string) error {
	_, err := db.Exec(fmt.Sprintf("DROP VIEW %s", p.Quote(viewName)))
	return err
}

func (p *dmProvider) TruncateTable(db *sql.DB, dbName, tableName string) error {
	_, err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s", p.Quote(tableName)))
	return err
}

// CopyTable clones structure and data via CREATE TABLE AS SELECT (no
// constraints/indexes, like the other providers' quick copies).
func (p *dmProvider) CopyTable(db *sql.DB, dbName, tableName, newTableName string) error {
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE %s AS SELECT * FROM %s", p.Quote(newTableName), p.Quote(tableName)))
	return err
}

// ── DDL: Column ──

func (p *dmProvider) AddColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
	if _, err := db.Exec(p.buildColumnSQL("ADD", tableName, col)); err != nil {
		return err
	}
	if col.Comment != "" {
		_, err := db.Exec(p.commentOnColumnSQL(dbName, tableName, col.Name, col.Comment))
		return err
	}
	return nil
}

func (p *dmProvider) ModifyColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
	stmts := []string{p.buildColumnSQL("MODIFY", tableName, col)}
	if col.Comment != "" {
		stmts = append(stmts, p.commentOnColumnSQL(dbName, tableName, col.Name, col.Comment))
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (p *dmProvider) DropColumn(db *sql.DB, dbName, tableName, colName string) error {
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", p.Quote(tableName), p.Quote(colName)))
	return err
}

// ── DDL: Index ──

func (p *dmProvider) AddIndex(db *sql.DB, dbName, tableName string, idx IndexDef) error {
	cols := make([]string, len(idx.Columns))
	for i, c := range idx.Columns {
		cols[i] = p.Quote(c)
	}
	if idx.IsPrimary {
		_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY (%s)", p.Quote(tableName), strings.Join(cols, ", ")))
		return err
	}
	uniqueStr := ""
	if idx.Unique {
		uniqueStr = "UNIQUE "
	}
	_, err := db.Exec(fmt.Sprintf("CREATE %sINDEX %s ON %s (%s)", uniqueStr, p.Quote(idx.Name), p.Quote(tableName), strings.Join(cols, ", ")))
	return err
}

func (p *dmProvider) DropIndex(db *sql.DB, dbName, tableName, idxName string, isPrimary bool, autoIncCols []string) error {
	if isPrimary {
		_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP PRIMARY KEY", p.Quote(tableName)))
		return err
	}
	_, err := db.Exec(fmt.Sprintf("DROP INDEX %s", p.Quote(idxName)))
	return err
}

// ── SQL builders ──

func (p *dmProvider) buildColumnSQL(action, tableName string, col ColumnDef) string {
	if !strings.Contains(tableName, `"`) {
		tableName = p.Quote(tableName)
	}
	var parts []string
	parts = append(parts, p.Quote(col.Name), col.Type)

	if col.Nullable {
		parts = append(parts, "NULL")
	} else {
		parts = append(parts, "NOT NULL")
	}

	switch col.DefaultType {
	case "null":
		parts = append(parts, "DEFAULT NULL")
	case "value":
		parts = append(parts, "DEFAULT "+oracleDefaultValue(col.DefaultVal))
	}

	return fmt.Sprintf("ALTER TABLE %s %s %s", tableName, action, strings.Join(parts, " "))
}

func (p *dmProvider) commentOnColumnSQL(dbName, tableName, colName, comment string) string {
	return fmt.Sprintf("COMMENT ON COLUMN %s.%s IS '%s'",
		p.Quote(tableName), p.Quote(colName), strings.ReplaceAll(comment, "'", "''"))
}

func (p *dmProvider) QualifyTable(tableName string) string {
	return p.Quote(tableName)
}

// dmClobText reads a driver-returned *dm.DmClob into a plain string; ok is
// false when v is not a CLOB (or cannot be read).
func dmClobText(v any) (string, bool) {
	clob, ok := v.(*dm.DmClob)
	if !ok {
		return "", false
	}
	s, err := clob.ReadString(1, 64<<20)
	if err != nil {
		return "", false
	}
	return s, true
}

var dmColumnTypes = []string{
	"INT", "INTEGER", "BIGINT", "TINYINT", "SMALLINT",
	"DECIMAL(10,2)", "NUMERIC", "FLOAT", "DOUBLE", "REAL",
	"BIT", "CHAR(10)", "VARCHAR(255)", "VARCHAR2(255)", "TEXT", "CLOB", "BLOB",
	"BINARY(16)", "VARBINARY(255)",
	"DATE", "TIME", "TIMESTAMP", "DATETIME",
}

var dmIntTypes = []string{
	"INT", "INTEGER", "BIGINT", "TINYINT", "SMALLINT",
}

// ── Dump ──

func (p *dmProvider) DumpTable(db *sql.DB, dbName, tableName string, opts DumpOptions) (string, error) {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if err := p.PrepareExec(conn, dbName); err != nil {
		return "", err
	}

	owner, err := p.resolveSchema(db, dbName)
	if err != nil {
		return "", err
	}
	owner = strings.ToUpper(owner)
	table := strings.ToUpper(tableName)

	var b strings.Builder
	quotedTable := p.Quote(tableName)

	if opts.Structure {
		kind := oracleTableKind(conn, owner, tableName)
		// GET_DDL returns a CLOB the driver surfaces as *dm.DmClob, so scan
		// into any and read the text out of whichever type comes back.
		var ddlVal any
		err := conn.QueryRowContext(ctx,
			"SELECT DBMS_METADATA.GET_DDL(?, ?, ?) FROM DUAL",
			strings.ToUpper(kind), table, owner).Scan(&ddlVal)
		if err != nil {
			return "", err
		}
		var createSQL string
		switch v := ddlVal.(type) {
		case string:
			createSQL = v
		case *dm.DmClob:
			createSQL, err = v.ReadString(1, 64<<20)
			if err != nil {
				return "", fmt.Errorf("read DDL clob: %w", err)
			}
		default:
			return "", fmt.Errorf("unexpected GET_DDL value type %T", ddlVal)
		}
		b.WriteString("--\n-- Structure for ")
		b.WriteString(tableName)
		b.WriteString("\n--\nDROP " + kind + " IF EXISTS " + quotedTable + ";\n")
		b.WriteString(createSQL)
		b.WriteString(";\n\n")
	}

	if opts.Data {
		cols, colTypes, derr := oracleTableColumns(conn, owner, tableName)
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
					b.WriteString(dumpOracleValue(v, colTypes[i]))
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
