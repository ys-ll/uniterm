package database

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	mssql "github.com/microsoft/go-mssqldb"
	"github.com/microsoft/go-mssqldb/msdsn"
	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/go-sql-driver/mysql"
	pgx "github.com/jackc/pgx/v5"
	pgxstdlib "github.com/jackc/pgx/v5/stdlib"
	go_ora "github.com/sijms/go-ora/v2"
)

// schemaTimeout caps each schema / dictionary query (object tree, table
// schema, indexes, primary keys…). These run through queryStrings/queryAny.
// On a slow or heavily locked server a dictionary scan can hang forever; on
// such a hang database/sql returns the pooled connection after this deadline
// (it closes the driver conn to break the stalled read, even for drivers like
// go-ora that are not context-aware), so a stuck query can no longer starve the
// connection pool and freeze the whole app.
var schemaTimeout = 60 * time.Second

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
	return NewDBWithDial(dbType, dsn, nil)
}

// DialContextFunc is the proxy dial hook NewDBWithDial accepts. The method
// form lets the same value satisfy the dialer interfaces of every driver that
// supports custom dialing (pgx DialFunc, go-sql-driver, go-mssqldb Dialer,
// go-ora DialerContext).
type DialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// DialContext implements the drivers' dialer interfaces.
func (f DialContextFunc) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return f(ctx, network, addr)
}

// dialSeq hands out unique network names for go-sql-driver's global registry,
// so concurrent sessions with different proxies never overwrite each other.
var (
	dialSeqMu   sync.Mutex
	dialSeqNext int
)

// NewDBWithDial is NewDB with an optional outbound dial override. When set,
// the driver's TCP connections route through dialFunc (SOCKS5/HTTP proxy);
// drivers without an injection point (rqlite, sqlite — local or no hook)
// ignore it and dial as usual.
func NewDBWithDial(dbType, dsn string, dialFunc DialContextFunc) (*sql.DB, error) {
	if dialFunc == nil {
		return openDB(dbType, dsn)
	}
	var (
		db  *sql.DB
		err error
	)
	switch dbType {
	// The MySQL-family and PostgreSQL-family dbTypes all reuse their family's
	// dial injection point; only genuinely different drivers get their own
	// branch.
	case "postgres", "kingbase", "opengauss", "greenplum", "gbase8c", "cockroachdb", "yugabytedb", "highgo", "cloudberry", "vastbase":
		db, err = openPostgresWithDial(dsn, dialFunc)
	case "mysql", "mariadb", "tidb", "oceanbase", "doris", "starrocks", "gbase8a", "greatsql", "goldendb":
		db, err = openMySQLWithDial(dsn, dialFunc)
	case "sqlserver":
		db, err = openSQLServerWithDial(dsn, dialFunc)
	case "oracle":
		db, err = openOracleWithDial(dsn, dialFunc)
	case "clickhouse":
		db, err = openClickHouseWithDial(dsn, dialFunc)
	default:
		db, err = openDB(dbType, dsn)
	}
	if err != nil {
		return nil, err
	}
	configureDBPool(db, dbDriverName(dbType))
	return db, nil
}

func openDB(dbType, dsn string) (*sql.DB, error) {
	p, err := NewProvider(dbType)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(p.DriverName(), dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dbType, err)
	}
	configureDBPool(db, p.DriverName())
	return db, nil
}

// trackDBDriver records db's driver name at open time (see dbDrivers).
func trackDBDriver(db *sql.DB, name string) {
	dbDrivers.Store(db, name)
}

// configureDBPool applies the shared pool limits and records the driver name.
func configureDBPool(db *sql.DB, driver string) {
	trackDBDriver(db, driver)
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)
}

func openPostgresWithDial(dsn string, dialFunc DialContextFunc) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	cfg.DialFunc = dialFunc.DialContext
	return pgxstdlib.OpenDB(*cfg), nil
}

func openMySQLWithDial(dsn string, dialFunc DialContextFunc) (*sql.DB, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	// The driver resolves cfg.Net through a global registry; register a fresh
	// unique name per connection so two sessions through different proxies
	// never race on the same entry.
	dialSeqMu.Lock()
	dialSeqNext++
	netName := fmt.Sprintf("uniterm-proxy-%d", dialSeqNext)
	dialSeqMu.Unlock()
	mysql.RegisterDialContext(netName, func(ctx context.Context, addr string) (net.Conn, error) {
		return dialFunc(ctx, "tcp", addr)
	})
	cfg.Net = netName
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	return sql.OpenDB(connector), nil
}

func openSQLServerWithDial(dsn string, dialFunc DialContextFunc) (*sql.DB, error) {
	cfg, err := msdsn.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlserver: %w", err)
	}
	connector := mssql.NewConnectorConfig(cfg)
	connector.Dialer = dialFunc
	return sql.OpenDB(connector), nil
}

func openOracleWithDial(dsn string, dialFunc DialContextFunc) (*sql.DB, error) {
	conn := go_ora.NewConnector(dsn)
	oc, ok := conn.(*go_ora.OracleConnector)
	if !ok {
		return nil, fmt.Errorf("open oracle: unexpected connector type %T", conn)
	}
	oc.Dialer(dialFunc)
	return sql.OpenDB(conn), nil
}

func openClickHouseWithDial(dsn string, dialFunc DialContextFunc) (*sql.DB, error) {
	cfg, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("open clickhouse: %w", err)
	}
	cfg.DialContext = func(ctx context.Context, addr string) (net.Conn, error) {
		return dialFunc(ctx, "tcp", addr)
	}
	return clickhouse.OpenDB(cfg), nil
}

// dbCtx returns the execution context for queries on db. The dm driver
// nil-derefs when a parameterized query runs with a deadline context
// (gitee.com/travelliu/dm v1.8.11192), so dm gets a bare background context;
// the driver is not context-aware anyway and ConnMaxLifetime still bounds
// stalled connections.
func dbCtx(db *sql.DB, timeout time.Duration) (context.Context, context.CancelFunc) {
	if driverNameOf(db) == "dm" {
		return context.Background(), func() {}
	}
	return context.WithTimeout(context.Background(), timeout)
}

// dbDrivers maps each open *sql.DB to its driver name; database/sql does not
// expose the driver name on the handle. Written once at open time.
var dbDrivers sync.Map

// driverNameOf returns the recorded driver name for db, or "" if unknown.
func driverNameOf(db *sql.DB) string {
	if v, ok := dbDrivers.Load(db); ok {
		return v.(string)
	}
	return ""
}

// dbDriverName maps a database type to the driver name sql.Open will use.
func dbDriverName(dbType string) string {
	switch dbType {
	case "postgres", "kingbase", "opengauss", "greenplum", "gbase8c", "cockroachdb", "yugabytedb", "highgo", "cloudberry", "vastbase":
		return "pgx"
	default:
		if p, err := NewProvider(dbType); err == nil {
			return p.DriverName()
		}
		return dbType
	}
}

// queryStrings executes a SQL query and returns all rows as []map[string]string.
func queryStrings(db *sql.DB, query string, args ...any) ([]map[string]string, error) {
	ctx, cancel := dbCtx(db, schemaTimeout)
	defer cancel()
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	guids := guidColumns(rows)
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
			// Keep the driver-reported key (providers like ClickHouse and
			// SQLite read it as-is) and add an upper-case alias: Oracle/DM
			// report upper-case column names natively while the GBase 8s
			// driver lower-cases them.
			row[col] = scanToStringCol(values[i], isGuidCol(guids, i))
			row[strings.ToUpper(col)] = row[col]
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// queryAny is like queryStrings but preserves nil values for proper NULL handling.
// Returns rows, column names in SQL order, and error.
func queryAny(db *sql.DB, query string, args ...any) ([]map[string]any, []string, error) {
	ctx, cancel := dbCtx(db, schemaTimeout)
	defer cancel()
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}

	guids := guidColumns(rows)
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
			row[col] = scanToAnyCol(values[i], isGuidCol(guids, i))
		}
		result = append(result, row)
	}
	return result, cols, rows.Err()
}

// QueryRowsStream streams rows to a callback without allocating a map per row.
// Use it for large results (>~1000 rows); for small results the existing
// queryStrings / queryAny maps are fine. The callback receives a reusable
// []string sized to len(cols); values are pre-converted via scanToString.
// Returning a non-nil error from the callback aborts the scan.
func QueryRowsStream(db *sql.DB, query string, callback func(row []string) error, args ...any) error {
	rows, err := db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return err
	}

	values := make([]any, len(cols))
	valuePtrs := make([]any, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}
	out := make([]string, len(cols))
	guids := guidColumns(rows)

	for rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			return err
		}
		for i, v := range values {
			out[i] = scanToStringCol(v, isGuidCol(guids, i))
		}
		if err := callback(out); err != nil {
			return err
		}
	}
	return rows.Err()
}

func scanToAny(v any) any {
	return scanToAnyCol(v, false)
}

// scanToAnyCol is scanToAny with the column's Guid flag (see guidColumns).
func scanToAnyCol(v any, guid bool) any {
	if v == nil {
		return nil
	}
	if s, ok := dmClobText(v); ok {
		return s
	}
	switch s := v.(type) {
	case []byte:
		return formatBytes(s, guid)
	default:
		return v
	}
}

func scanToString(v any) string {
	return scanToStringCol(v, false)
}

// scanToStringCol is scanToString with the column's Guid flag (see guidColumns).
func scanToStringCol(v any, guid bool) string {
	if v == nil {
		return ""
	}
	if s, ok := dmClobText(v); ok {
		return s
	}
	switch x := v.(type) {
	case []byte:
		return formatBytes(x, guid)
	case string:
		return x
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case time.Time:
		return x.Format(time.RFC3339Nano)
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
