package database

import (
	"context"
	"database/sql"
	"fmt"
)

// execer is the common interface for sql.DB, sql.Conn, and sql.Tx.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Provider encapsulates all database-type-specific behavior.
type Provider interface {
	// DSN generates a driver-specific connection string from connection fields.
	DSN(host string, port int, user, password, dbName string) string

	// DriverName returns the Go SQL driver name to use for sql.Open.
	DriverName() string

	// Quote returns an identifier quoted for this database.
	Quote(name string) string

	// Schema discovery
	GetDatabases(db *sql.DB) ([]string, error)
	GetTables(db *sql.DB, dbName string) ([]TableInfo, error)
	GetTableSchema(db *sql.DB, dbName, tableName string) (*SchemaResult, error)

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
