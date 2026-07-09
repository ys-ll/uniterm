package database

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

// sortedKeys returns the sorted keys of a map; deterministic ordering makes
// generated SQL stable and easier to test.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// execer is the common interface for sql.DB, sql.Conn, and sql.Tx.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Provider encapsulates all database-type-specific behavior.
type Provider interface {
	// DSN generates a driver-specific connection string from connection fields.
	DSN(host string, port int, user, password, dbName string, extraParams map[string]string) string

	// DriverName returns the Go SQL driver name to use for sql.Open.
	DriverName() string

	// Quote returns an identifier quoted for this database.
	Quote(name string) string

	// Schema discovery
	GetDatabases(db *sql.DB) ([]string, error)
	GetTables(db *sql.DB, dbName string) ([]TableInfo, error)
	GetTableSchema(db *sql.DB, dbName, tableName string) (*SchemaResult, error)

	// DefaultTableQuery returns the default SELECT statement used when opening a table.
	DefaultTableQuery(dbName, tableName string, limit int) string

	// Row-level CRUD helpers for the result-grid inline actions.
	InsertRow(db *sql.DB, dbName, tableName string, values map[string]any) error
	UpdateRow(db *sql.DB, dbName, tableName string, set, where map[string]any) error
	DeleteRow(db *sql.DB, dbName, tableName string, where map[string]any) error

	// DDL: Database
	CreateDatabase(db *sql.DB, dbName string) error
	DropDatabase(db *sql.DB, dbName string) error

	// DDL: Table
	CreateTable(db *sql.DB, dbName, tableName string) error
	DropTable(db *sql.DB, dbName, tableName string) error
	DropView(db *sql.DB, dbName, viewName string) error
	TruncateTable(db *sql.DB, dbName, tableName string) error

	// DDL: Column
	AddColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error
	ModifyColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error
	DropColumn(db *sql.DB, dbName, tableName string, colName string) error

	// DDL: Index
	AddIndex(db *sql.DB, dbName, tableName string, idx IndexDef) error
	DropIndex(db *sql.DB, dbName, tableName string, idxName string, isPrimary bool, autoIncCols []string) error

	// Capabilities
	GetCapabilities() DBCapabilities

	// PrepareExec executes any per-database setup before running user SQL.
	PrepareExec(db execer, dbName string) error
}

// execPrepared executes a parameterized statement on a dedicated connection
// after running the provider's per-database setup (PrepareExec).
func execPrepared(p Provider, db *sql.DB, dbName, sql string, args []any) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := p.PrepareExec(conn, dbName); err != nil {
		return err
	}

	_, err = conn.ExecContext(ctx, sql, args...)
	return err
}

var providers = map[string]Provider{}

// Register registers a Provider for a database type. Call from init().
func Register(dbType string, p Provider) {
	providers[dbType] = p
}

// NewProvider returns the Provider for the given database type, or an error
// if the type is not supported.
func NewProvider(dbType string) (Provider, error) {
	p, ok := providers[dbType]
	if !ok {
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}
	return p, nil
}
