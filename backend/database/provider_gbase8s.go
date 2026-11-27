package database

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	// Official pure-Go driver for GBase 8s (Informix-based SQL dialect),
	// registers as "gbase8s". GRSL-licensed vendor code, pinned to our
	// verbatim GitHub mirror via a replace directive.
	_ "gitee.com/GBase8s/go-gbase8s"
)

type gbase8sProvider struct{}

func init() {
	Register("gbase8s", &gbase8sProvider{})
}

// gbase8sHideCompatCatalogs filters out the Oracle-compatibility catalog
// views GBase 8s creates in every database (user_*/dba_*/all_*/v$*/dual and
// a few sys* helpers); the tree shows user objects only, mirroring how the
// other providers hide system schemas.
const gbase8sHideCompatCatalogs = `
	AND tabname NOT LIKE 'user\_%' ESCAPE '\'
	AND tabname NOT LIKE 'dba\_%' ESCAPE '\'
	AND tabname NOT LIKE 'all\_%' ESCAPE '\'
	AND tabname NOT LIKE 'v$%'
	AND tabname NOT LIKE 'v\_$%' ESCAPE '\'
	AND tabname <> 'dual'
	AND tabname NOT IN ('syscolcomments', 'syscolumnsext', 'syscomments', 'sysdomains', 'sysindexes')`

// Informix tabtype codes in systables.tabtype.
const (
	gbase8sTabTypeTable = "T"
	gbase8sTabTypeView  = "V"
)

func (p *gbase8sProvider) DSN(host string, port int, user, password, dbName string, extraParams map[string]string) string {
	if port <= 0 {
		port = 9088
	}
	u := &url.URL{
		Scheme: "gbase8s",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
	}
	if dbName != "" {
		u.Path = "/" + dbName
	}
	// Omitting GBASEDBTSERVER lets the driver connect straight to host:port
	// (verified against a live server); injecting a default name breaks
	// authentication when it does not match the server's actual service
	// name, so only PROTOCOL and delimident get defaults. delimident=1 makes
	// double quotes delimit identifiers, which Quote relies on.
	q := u.Query()
	if _, ok := extraParams["PROTOCOL"]; !ok {
		q.Set("PROTOCOL", "onsoctcp")
	}
	q.Set("delimident", "1")
	for k, v := range extraParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (p *gbase8sProvider) DriverName() string {
	return "gbase8s"
}

func (p *gbase8sProvider) Quote(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// qualifyDb prefixes an object reference with its database. Informix-family
// tables and catalogs live inside a database and the session cannot switch
// databases mid-connection, so every statement aimed at a tree node's
// database (including its catalog tables) needs db:object qualification.
func (p *gbase8sProvider) qualifyDb(dbName, object string) string {
	if dbName == "" {
		return p.Quote(object)
	}
	return p.Quote(dbName) + ":" + p.Quote(object)
}

// PrepareExec is a no-op: the tree's database level maps to the DSN database,
// and Informix sessions cannot switch databases mid-connection.
func (p *gbase8sProvider) PrepareExec(db execer, dbName string) error {
	return nil
}

func (p *gbase8sProvider) DefaultTableQuery(dbName, tableName string, limit, offset int) string {
	return p.PagedTableQuery(dbName, tableName, limit, offset)
}

// PagedTableQuery uses Informix SKIP/FIRST (LIMIT/OFFSET is not universal
// across GBase 8s server builds).
func (p *gbase8sProvider) PagedTableQuery(dbName, tableName string, limit, offset int) string {
	q := p.qualifyDb(dbName, tableName)
	if offset > 0 {
		return fmt.Sprintf("SELECT SKIP %d FIRST %d * FROM %s", offset, limit, q)
	}
	return fmt.Sprintf("SELECT FIRST %d * FROM %s", limit, q)
}

// ── Row-level CRUD ──

func (p *gbase8sProvider) InsertRow(db *sql.DB, dbName, tableName string, values map[string]any) error {
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
		p.qualifyDb(dbName, tableName), strings.Join(quotedCols, ", "), strings.Join(placeholders, ", "))
	return execPrepared(p, db, dbName, stmt, args)
}

func (p *gbase8sProvider) UpdateRow(db *sql.DB, dbName, tableName string, set, where map[string]any) error {
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

	stmt := fmt.Sprintf("UPDATE %s SET %s", p.qualifyDb(dbName, tableName), strings.Join(setParts, ", "))
	if len(whereParts) > 0 {
		stmt += " WHERE " + strings.Join(whereParts, " AND ")
	}
	return execPrepared(p, db, dbName, stmt, args)
}

func (p *gbase8sProvider) DeleteRow(db *sql.DB, dbName, tableName string, where map[string]any) error {
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

	stmt := fmt.Sprintf("DELETE FROM %s", p.qualifyDb(dbName, tableName))
	if len(whereParts) > 0 {
		stmt += " WHERE " + strings.Join(whereParts, " AND ")
	}
	return execPrepared(p, db, dbName, stmt, args)
}

// ── Capabilities ──

func (p *gbase8sProvider) GetCapabilities() DBCapabilities {
	return DBCapabilities{
		"supportsAutoIncrement":      true,
		"supportsOnUpdate":           false,
		"supportsCollation":          false,
		"supportsCreateDatabase":     true,
		"autoIncrementForcesNotNull": true,
		"columnTypes":                gbase8sColumnTypes,
		"intTypes":                   gbase8sIntTypes,
	}
}

// ── Schema discovery ──

// gbase8sSystemDatabases lists the instance-maintained databases (monitoring
// catalog, task scheduler, auth catalog, backup scheduler) hidden from the
// tree, mirroring the other providers' system-schema filters.
var gbase8sSystemDatabases = map[string]bool{
	"sysmaster": true, "sysadmin": true, "sysuser": true,
	"sysutils": true, "sysaudit": true, "sys": true, "gbasedbt": true,
}

func (p *gbase8sProvider) GetDatabases(db *sql.DB) ([]string, error) {
	// sysmaster is the instance-wide catalog; sysdatabases lists every
	// database in the instance.
	results, err := queryStrings(db, `SELECT name FROM sysmaster:sysdatabases ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	dbs := make([]string, 0, len(results))
	for _, row := range results {
		n := row["NAME"]
		if n == "" || gbase8sSystemDatabases[strings.ToLower(n)] {
			continue
		}
		dbs = append(dbs, n)
	}
	return dbs, nil
}

// GetSchemas: Informix-style dialects have no inner-schema concept; the tree
// stays single-level (instance → databases → tables).
func (p *gbase8sProvider) GetSchemas(db *sql.DB, dbName string) ([]string, error) {
	return nil, nil
}

func (p *gbase8sProvider) GetTables(db *sql.DB, dbName string) ([]TableInfo, error) {
	results, err := queryStrings(db, `
		SELECT tabname, tabtype FROM `+p.qualifyDb(dbName, "systables")+`
		WHERE tabid >= 100 AND tabtype IN ('T', 'V')`+gbase8sHideCompatCatalogs+`
		ORDER BY tabname`)
	if err != nil {
		return nil, fmt.Errorf("get tables: %w", err)
	}
	return gbase8sTableInfos(results), nil
}

func (p *gbase8sProvider) GetTablePage(db *sql.DB, dbName string, query string, offset, limit int) ([]TableInfo, int, error) {
	where := `tabid >= 100 AND tabtype IN ('T', 'V')` + gbase8sHideCompatCatalogs
	args := make([]any, 0, 1)
	if query != "" {
		where += ` AND LOWER(tabname) LIKE ?`
		args = append(args, "%"+strings.ToLower(query)+"%")
	}

	countRows, err := queryStrings(db, `SELECT COUNT(*) AS n FROM `+p.qualifyDb(dbName, "systables")+` WHERE `+where, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("count tables: %w", err)
	}
	total := 0
	if len(countRows) > 0 {
		total, _ = strconv.Atoi(countRows[0]["N"])
	}

	results, err := queryStrings(db, fmt.Sprintf(`
		SELECT SKIP %d FIRST %d tabname, tabtype FROM `+p.qualifyDb(dbName, "systables")+`
		WHERE %s
		ORDER BY tabname`, offset, limit, where), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("get tables page: %w", err)
	}
	return gbase8sTableInfos(results), total, nil
}

func gbase8sTableInfos(results []map[string]string) []TableInfo {
	infos := make([]TableInfo, 0, len(results))
	for _, row := range results {
		tp := "table"
		if row["TABTYPE"] == gbase8sTabTypeView {
			tp = "view"
		}
		infos = append(infos, TableInfo{
			Name:    row["TABNAME"],
			Type:    tp,
			Comment: "",
		})
	}
	return infos
}

// gbase8sBaseType strips the NOT NULL flag bit (0x100) from syscolumns.coltype
// and returns the base type code.
func gbase8sBaseType(coltype int) int {
	return coltype & 0x00FF
}

// gbase8sColTypeName maps a syscolumns coltype code to a display type name,
// using collength for sized/precision types. Returns ok=false for exotic
// types (collections, ROW types) where the caller shows the raw code.
func gbase8sColTypeName(code, collength int) (name string, ok bool) {
	switch code {
	case 0: // CHAR
		return fmt.Sprintf("CHAR(%d)", collength), true
	case 1:
		return "SMALLINT", true
	case 2:
		return "INTEGER", true
	case 3:
		return "FLOAT", true
	case 4:
		return "SMALLFLOAT", true
	case 5: // DECIMAL: precision = collength>>8, scale = collength&0xFF
		if collength > 0 {
			return fmt.Sprintf("DECIMAL(%d,%d)", collength>>8, collength&0xFF), true
		}
		return "DECIMAL", true
	case 6:
		return "SERIAL", true
	case 7:
		return "DATE", true
	case 8: // MONEY
		if collength > 0 {
			return fmt.Sprintf("MONEY(%d,%d)", collength>>8, collength&0xFF), true
		}
		return "MONEY", true
	case 10: // DATETIME (qualifier encoded; show generic form)
		return "DATETIME", true
	case 11:
		return "BYTE", true
	case 12:
		return "TEXT", true
	case 13: // VARCHAR: collength is the max length directly on GBase 8s
		if collength > 0 {
			return fmt.Sprintf("VARCHAR(%d)", collength), true
		}
		return "VARCHAR", true
	case 14:
		return "INTERVAL", true
	case 15: // NCHAR
		return fmt.Sprintf("NCHAR(%d)", collength), true
	case 16: // NVARCHAR: collength is the max length directly on GBase 8s
		if collength > 0 {
			return fmt.Sprintf("NVARCHAR(%d)", collength), true
		}
		return "NVARCHAR", true
	case 17:
		return "INT8", true
	case 18:
		return "SERIAL8", true
	case 40:
		return "LVARCHAR", true
	case 41:
		return "BOOLEAN", true
	case 52:
		return "BIGINT", true
	case 53:
		return "BIGSERIAL", true
	}
	return "", false
}

func (p *gbase8sProvider) GetTableSchema(db *sql.DB, dbName, tableName string) (*SchemaResult, error) {
	colRows, err := queryStrings(db, `
		SELECT c.colname, c.coltype, c.collength
		FROM `+p.qualifyDb(dbName, "systables")+` t
		JOIN `+p.qualifyDb(dbName, "syscolumns")+` c ON c.tabid = t.tabid
		WHERE t.tabname = ? AND t.tabid >= 100
		ORDER BY c.colno`, tableName)
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	pkCols, err := gbase8sPrimaryColumns(db, dbName, tableName)
	if err != nil {
		return nil, fmt.Errorf("get primary keys: %w", err)
	}

	columns := make([]ColumnInfo, 0, len(colRows))
	for _, row := range colRows {
		coltype, _ := strconv.Atoi(row["COLTYPE"])
		collength, _ := strconv.Atoi(row["COLLENGTH"])
		code := gbase8sBaseType(coltype)
		nullable := coltype == code // 0x100 flag set means NOT NULL

		name, known := gbase8sColTypeName(code, collength)
		if !known {
			name = fmt.Sprintf("TYPE(%d)", code)
		}

		defVal := ""
		defaultType := "none"
		if nullable {
			defaultType = "null"
			defVal = "NULL"
		}
		columns = append(columns, ColumnInfo{
			Name:        row["COLNAME"],
			Type:        name,
			Nullable:    nullable,
			DefaultVal:  defVal,
			DefaultType: defaultType,
			IsPrimary:   pkCols[row["COLNAME"]],
			Comment:     "",
		})
	}

	indexes, err := gbase8sIndexes(db, dbName, tableName)
	if err != nil {
		return nil, fmt.Errorf("get indexes: %w", err)
	}

	return &SchemaResult{Columns: columns, Indexes: indexes}, nil
}

// gbase8sPrimaryColumns resolves PK column names via sysconstraints →
// sysindexes (idx1..idx16 hold colno positions, negative = descending).
func gbase8sPrimaryColumns(db *sql.DB, dbName, tableName string) (map[string]bool, error) {
	sysT, sysC, sysK, sysI := "systables", "syscolumns", "sysconstraints", "sysindexes"
	if dbName != "" {
		q := func(n string) string { return dbName + ":" + n }
		sysT, sysC, sysK, sysI = q("systables"), q("syscolumns"), q("sysconstraints"), q("sysindexes")
	}
	rows, err := queryStrings(db, `
		SELECT i.part1, i.part2, i.part3, i.part4, i.part5,
		       i.part6, i.part7, i.part8, i.part9, i.part10,
		       i.part11, i.part12, i.part13, i.part14, i.part15, i.part16
		FROM `+sysT+` t
		JOIN `+sysK+` k ON k.tabid = t.tabid AND k.constrtype = 'P'
		JOIN `+sysI+` i ON i.idxname = k.idxname AND i.tabid = t.tabid
		WHERE t.tabname = ? AND t.tabid >= 100`, tableName)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return map[string]bool{}, nil
	}

	// colno → colname for this table.
	colRows, err := queryStrings(db, `
		SELECT c.colno, c.colname FROM `+sysT+` t
		JOIN `+sysC+` c ON c.tabid = t.tabid
		WHERE t.tabname = ? AND t.tabid >= 100`, tableName)
	if err != nil {
		return nil, err
	}
	byColno := make(map[string]string, len(colRows))
	for _, r := range colRows {
		byColno[r["COLNO"]] = r["COLNAME"]
	}

	pk := make(map[string]bool)
	for _, r := range rows {
		for i := 1; i <= 16; i++ {
			key := fmt.Sprintf("PART%d", i)
			if r[key] == "" {
				continue
			}
			pos := strings.TrimPrefix(r[key], "-")
			if name, ok := byColno[pos]; ok {
				pk[name] = true
			}
		}
	}
	return pk, nil
}

// gbase8sIndexes lists indexes for a table. Informix auto-numbered indexes
// (names starting with a space) are hidden.
func gbase8sIndexes(db *sql.DB, dbName, tableName string) ([]IndexInfo, error) {
	sysT, sysC := "systables", "syscolumns"
	sysI, sysK := "sysindexes", "sysconstraints"
	if dbName != "" {
		q := func(n string) string { return dbName + ":" + n }
		sysT, sysC, sysI, sysK = q("systables"), q("syscolumns"), q("sysindexes"), q("sysconstraints")
	}
	rows, err := queryStrings(db, `
		SELECT i.idxname, i.idxtype, k.constrtype,
		       i.part1, i.part2, i.part3, i.part4, i.part5,
		       i.part6, i.part7, i.part8, i.part9, i.part10,
		       i.part11, i.part12, i.part13, i.part14, i.part15, i.part16
		FROM `+sysT+` t
		JOIN `+sysI+` i ON i.tabid = t.tabid
		LEFT JOIN `+sysK+` k ON k.idxname = i.idxname AND k.tabid = t.tabid
		WHERE t.tabname = ? AND t.tabid >= 100 AND i.idxname NOT LIKE ' %'
		ORDER BY i.idxname`, tableName)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	colRows, err := queryStrings(db, `
		SELECT c.colno, c.colname FROM `+sysT+` t
		JOIN `+sysC+` c ON c.tabid = t.tabid
		WHERE t.tabname = ? AND t.tabid >= 100`, tableName)
	if err != nil {
		return nil, err
	}
	byColno := make(map[string]string, len(colRows))
	for _, r := range colRows {
		byColno[r["COLNO"]] = r["COLNAME"]
	}

	indexes := make([]IndexInfo, 0, len(rows))
	for _, r := range rows {
		name := r["IDXNAME"]
		info := IndexInfo{
			Name:    name,
			Columns: []string{},
			Unique:  r["IDXTYPE"] == "U",
		}
		if r["CONSTRTYPE"] == "P" {
			info.IsPrimary = true
			info.Unique = true
		}
		for i := 1; i <= 16; i++ {
			key := fmt.Sprintf("PART%d", i)
			if r[key] == "" {
				continue
			}
			pos := strings.TrimPrefix(r[key], "-")
			if cname, ok := byColno[pos]; ok {
				info.Columns = append(info.Columns, cname)
			}
		}
		if len(info.Columns) > 0 || info.IsPrimary {
			indexes = append(indexes, info)
		}
	}
	return indexes, nil
}

// ── DDL: Database ──

func (p *gbase8sProvider) CreateDatabase(db *sql.DB, dbName string) error {
	_, err := db.Exec(fmt.Sprintf("CREATE DATABASE %s WITH LOG", p.Quote(dbName)))
	return err
}

func (p *gbase8sProvider) DropDatabase(db *sql.DB, dbName string) error {
	_, err := db.Exec(fmt.Sprintf("DROP DATABASE %s", p.Quote(dbName)))
	return err
}

// ── DDL: Table ──

func (p *gbase8sProvider) CreateTable(db *sql.DB, dbName, tableName string) error {
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY)", p.Quote(tableName)))
	return err
}

func (p *gbase8sProvider) DropTable(db *sql.DB, dbName, tableName string) error {
	_, err := db.Exec(fmt.Sprintf("DROP TABLE %s", p.Quote(tableName)))
	return err
}

func (p *gbase8sProvider) DropView(db *sql.DB, dbName, viewName string) error {
	_, err := db.Exec(fmt.Sprintf("DROP VIEW %s", p.Quote(viewName)))
	return err
}

func (p *gbase8sProvider) TruncateTable(db *sql.DB, dbName, tableName string) error {
	_, err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s", p.Quote(tableName)))
	return err
}

// CopyTable clones structure and data via CREATE TABLE AS SELECT (no
// constraints/indexes, like the other providers' quick copies).
func (p *gbase8sProvider) CopyTable(db *sql.DB, dbName, tableName, newTableName string) error {
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE %s AS SELECT * FROM %s", p.Quote(newTableName), p.Quote(tableName)))
	return err
}

// ── DDL: Column ──

func (p *gbase8sProvider) AddColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD (%s)", p.Quote(tableName), p.columnDefSQL(col)))
	return err
}

func (p *gbase8sProvider) ModifyColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s MODIFY (%s)", p.Quote(tableName), p.columnDefSQL(col)))
	return err
}

func (p *gbase8sProvider) DropColumn(db *sql.DB, dbName, tableName, colName string) error {
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP (%s)", p.Quote(tableName), p.Quote(colName)))
	return err
}

// columnDefSQL renders one column definition for ALTER TABLE ADD/MODIFY.
// Informix takes defaults inside the parenthesized column list.
func (p *gbase8sProvider) columnDefSQL(col ColumnDef) string {
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
		parts = append(parts, "DEFAULT "+gbase8sDefaultValue(col.DefaultVal))
	}

	return strings.Join(parts, " ")
}

// ── DDL: Index ──

func (p *gbase8sProvider) AddIndex(db *sql.DB, dbName, tableName string, idx IndexDef) error {
	cols := make([]string, len(idx.Columns))
	for i, c := range idx.Columns {
		cols[i] = p.Quote(c)
	}
	if idx.IsPrimary {
		_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT PRIMARY KEY (%s)",
			p.Quote(tableName), strings.Join(cols, ", ")))
		return err
	}
	uniqueStr := ""
	if idx.Unique {
		uniqueStr = "UNIQUE "
	}
	_, err := db.Exec(fmt.Sprintf("CREATE %sINDEX %s ON %s (%s)",
		uniqueStr, p.Quote(idx.Name), p.Quote(tableName), strings.Join(cols, ", ")))
	return err
}

func (p *gbase8sProvider) DropIndex(db *sql.DB, dbName, tableName, idxName string, isPrimary bool, autoIncCols []string) error {
	if isPrimary {
		// Informix drops PKs by constraint name; the grid passes the
		// constraint/index name shown in the tree.
		_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", p.Quote(tableName), p.Quote(idxName)))
		return err
	}
	_, err := db.Exec(fmt.Sprintf("DROP INDEX %s", p.Quote(idxName)))
	return err
}

func (p *gbase8sProvider) QualifyTable(tableName string) string {
	return p.Quote(tableName)
}

// ── SQL builders ──

func gbase8sDefaultValue(defaultVal string) string {
	upper := strings.ToUpper(strings.TrimSpace(defaultVal))
	switch upper {
	case "CURRENT", "TODAY":
		return upper
	}
	return "'" + strings.ReplaceAll(defaultVal, "'", "''") + "'"
}

var gbase8sColumnTypes = []string{
	"SMALLINT", "INTEGER", "BIGINT", "INT8",
	"SERIAL", "SERIAL8", "BIGSERIAL",
	"DECIMAL(10,2)", "MONEY(16,2)", "FLOAT", "SMALLFLOAT",
	"BOOLEAN",
	"CHAR(10)", "VARCHAR(255)", "NCHAR(10)", "NVARCHAR(255)", "LVARCHAR",
	"TEXT", "BYTE",
	"DATE", "DATETIME YEAR TO SECOND", "INTERVAL DAY(3) TO SECOND",
}

var gbase8sIntTypes = []string{
	"SMALLINT", "INTEGER", "BIGINT", "INT8",
}

// ── Dump ──

func (p *gbase8sProvider) DumpTable(db *sql.DB, dbName, tableName string, opts DumpOptions) (string, error) {
	ctx := context.Background()

	var b strings.Builder
	quotedTable := p.Quote(tableName)

	if opts.Structure {
		createSQL, err := gbase8sCreateTableSQL(db, p, dbName, tableName)
		if err != nil {
			return "", err
		}
		b.WriteString("--\n-- Structure for ")
		b.WriteString(tableName)
		b.WriteString("\n--\nDROP TABLE IF EXISTS " + quotedTable + ";\n")
		b.WriteString(createSQL)
		b.WriteString(";\n\n")
	}

	if opts.Data {
		colRows, err := queryStrings(db, `
			SELECT c.colname FROM `+p.qualifyDb(dbName, "systables")+` t
			JOIN `+p.qualifyDb(dbName, "syscolumns")+` c ON c.tabid = t.tabid
			WHERE t.tabname = ? AND t.tabid >= 100
			ORDER BY c.colno`, tableName)
		if err != nil {
			return "", err
		}
		if len(colRows) > 0 {
			b.WriteString("--\n-- Data for ")
			b.WriteString(tableName)
			b.WriteString("\n--\n")
			colList := make([]string, len(colRows))
			for i, r := range colRows {
				colList[i] = p.Quote(r["COLNAME"])
			}
			prefix := "INSERT INTO " + quotedTable + " (" + strings.Join(colList, ", ") + ") VALUES "

			rows, err := db.QueryContext(ctx, "SELECT "+strings.Join(colList, ", ")+" FROM "+p.qualifyDb(dbName, tableName))
			if err != nil {
				return "", err
			}
			defer rows.Close()

			for rows.Next() {
				vals := make([]any, len(colRows))
				ptrs := make([]any, len(colRows))
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
					b.WriteString(dumpGbase8sValue(v))
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

// gbase8sCreateTableSQL rebuilds a CREATE TABLE statement from the catalog
// (Informix dialects have no GET_DDL equivalent).
func gbase8sCreateTableSQL(db *sql.DB, p *gbase8sProvider, dbName, tableName string) (string, error) {
	colRows, err := queryStrings(db, `
		SELECT c.colname, c.coltype, c.collength
		FROM `+p.qualifyDb(dbName, "systables")+` t
		JOIN `+p.qualifyDb(dbName, "syscolumns")+` c ON c.tabid = t.tabid
		WHERE t.tabname = ? AND t.tabid >= 100
		ORDER BY c.colno`, tableName)
	if err != nil {
		return "", err
	}
	if len(colRows) == 0 {
		return "", fmt.Errorf("table %s not found", tableName)
	}

	// Serial columns and explicit defaults come from sysdefaults; PK/serial
	// NOT NULL flags mirror GetTableSchema.
	pk, err := gbase8sPrimaryColumns(db, dbName, tableName)
	if err != nil {
		return "", err
	}

	defRows, err := queryStrings(db, `
		SELECT d.colno, d.type, d."default", d.class
		FROM `+p.qualifyDb(dbName, "systables")+` t
		JOIN `+p.qualifyDb(dbName, "sysdefaults")+` d ON d.tabid = t.tabid
		WHERE t.tabname = ? AND t.tabid >= 100`, tableName)
	if err != nil {
		return "", err
	}
	defaults := make(map[int]string, len(defRows))
	for _, r := range defRows {
		colno, _ := strconv.Atoi(r["COLNO"])
		dclass := strings.ToUpper(strings.TrimSpace(r["CLASS"]))
		switch dclass {
		case "C":
			defaults[colno] = "CURRENT"
		case "T":
			defaults[colno] = "TODAY"
		default:
			if lit := strings.TrimSpace(r["DEFAULT"]); lit != "" {
				defaults[colno] = "'" + strings.ReplaceAll(lit, "'", "''") + "'"
			}
		}
	}

	var pkCols []string
	lines := make([]string, 0, len(colRows)+1)
	for i, row := range colRows {
		coltype, _ := strconv.Atoi(row["COLTYPE"])
		collength, _ := strconv.Atoi(row["COLLENGTH"])
		code := gbase8sBaseType(coltype)
		nullable := coltype == code

		tname, known := gbase8sColTypeName(code, collength)
		if !known {
			tname = fmt.Sprintf("LVARCHAR")
		}
		line := p.Quote(row["COLNAME"]) + " " + tname
		if !nullable {
			line += " NOT NULL"
		}
		if expr, ok := defaults[i+1]; ok { // colno is 1-based, rows ordered by colno
			line += " DEFAULT " + expr
		}
		if pk[row["COLNAME"]] {
			pkCols = append(pkCols, row["COLNAME"])
		}
		lines = append(lines, line)
	}
	if len(pkCols) > 0 {
		quoted := make([]string, len(pkCols))
		for i, c := range pkCols {
			quoted[i] = p.Quote(c)
		}
		lines = append(lines, "PRIMARY KEY ("+strings.Join(quoted, ", ")+")")
	}

	return fmt.Sprintf("CREATE TABLE %s (\n  %s\n)", p.Quote(tableName), strings.Join(lines, ",\n  ")), nil
}

// dumpGbase8sValue renders one scanned value as a SQL literal.
func dumpGbase8sValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		return "'" + strings.ReplaceAll(string(x), "'", "''") + "'"
	case string:
		return "'" + strings.ReplaceAll(x, "'", "''") + "'"
	case bool:
		if x {
			return "'t'"
		}
		return "'f'"
	default:
		return fmt.Sprintf("%v", x)
	}
}
