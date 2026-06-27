package database

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
)

type postgresProvider struct{}

func init() {
	Register("postgres", &postgresProvider{})
}

func (p *postgresProvider) DSN(host string, port int, user, password, dbName string) string {
	if port <= 0 {
		port = 5432
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbName)
}

func (p *postgresProvider) DriverName() string {
	return "postgres"
}

func (p *postgresProvider) Quote(name string) string {
	return `"` + name + `"`
}

func (p *postgresProvider) PrepareExec(db execer, dbName string) error {
	return nil
}

func (p *postgresProvider) GetCapabilities() DBCapabilities {
	return DBCapabilities{
		"supportsAutoIncrement":      false,
		"supportsOnUpdate":           false,
		"supportsCollation":          false,
		"autoIncrementForcesNotNull": false,
		"columnTypes":                postgresTypes,
		"intTypes":                   postgresIntTypes,
	}
}

// ── Schema discovery ──

func (p *postgresProvider) GetDatabases(db *sql.DB) ([]string, error) {
	results, err := queryStrings(db, "SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(results))
	for _, row := range results {
		names = append(names, row["datname"])
	}
	return names, nil
}

func (p *postgresProvider) GetTables(db *sql.DB, dbName string) ([]TableInfo, error) {
	results, err := queryStrings(db,
		"SELECT table_name, table_type FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog', 'information_schema') ORDER BY table_name")
	if err != nil {
		return nil, fmt.Errorf("get tables: %w", err)
	}
	infos := make([]TableInfo, 0, len(results))
	for _, row := range results {
		tp := "table"
		if row["table_type"] == "VIEW" {
			tp = "view"
		}
		infos = append(infos, TableInfo{Name: row["table_name"], Type: tp})
	}
	return infos, nil
}

func (p *postgresProvider) GetTableSchema(db *sql.DB, dbName, tableName string) (*SchemaResult, error) {
	colRows, err := queryStrings(db,
		fmt.Sprintf("SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_name = $1 ORDER BY ordinal_position"),
		tableName,
	)
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
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
		})
	}

	// Primary keys
	pkRows, err := queryStrings(db,
		fmt.Sprintf("SELECT a.attname FROM pg_index i JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey) WHERE i.indrelid = $1::regclass AND i.indisprimary"),
		tableName,
	)
	if err == nil {
		for _, row := range pkRows {
			for i := range columns {
				if columns[i].Name == row["attname"] {
					columns[i].IsPrimary = true
				}
			}
		}
	}

	// Indexes
	idxRows, err := queryStrings(db,
		fmt.Sprintf("SELECT i.relname AS index_name, ix.indisunique AS is_unique, ix.indisprimary AS is_primary, a.attname AS column_name FROM pg_class t JOIN pg_index ix ON t.oid = ix.indrelid JOIN pg_class i ON i.oid = ix.indexrelid JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey) WHERE t.relname = $1 ORDER BY i.relname, a.attnum"),
		tableName,
	)
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

	return &SchemaResult{Columns: columns, Indexes: indexes}, nil
}

// ── DDL: Database ──

func (p *postgresProvider) CreateDatabase(db *sql.DB, dbName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("CREATE DATABASE %s", q(dbName)))
	return err
}

func (p *postgresProvider) DropDatabase(db *sql.DB, dbName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("DROP DATABASE %s", q(dbName)))
	return err
}

// ── DDL: Table ──

func (p *postgresProvider) CreateTable(db *sql.DB, dbName, tableName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY)", q(tableName)))
	return err
}

func (p *postgresProvider) DropTable(db *sql.DB, dbName, tableName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("DROP TABLE %s", q(tableName)))
	return err
}

func (p *postgresProvider) DropView(db *sql.DB, dbName, viewName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("DROP VIEW %s", q(viewName)))
	return err
}

func (p *postgresProvider) TruncateTable(db *sql.DB, dbName, tableName string) error {
	q := p.Quote
	_, err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s", q(tableName)))
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
			parts = append(parts, "DEFAULT "+col.DefaultVal)
		} else {
			parts = append(parts, "DEFAULT ''")
		}
	}

	sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", q(tableName), strings.Join(parts, " "))
	_, err := db.Exec(sql)
	return err
}

func (p *postgresProvider) ModifyColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
	q := p.Quote
	var stmts []string

	stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s",
		q(tableName), q(col.Name), col.Type))

	if col.Nullable {
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL",
			q(tableName), q(col.Name)))
	} else {
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL",
			q(tableName), q(col.Name)))
	}

	switch col.DefaultType {
	case "null":
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT NULL",
			q(tableName), q(col.Name)))
	case "value":
		if col.DefaultVal != "" {
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s",
				q(tableName), q(col.Name), col.DefaultVal))
		} else {
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT ''",
				q(tableName), q(col.Name)))
		}
	default:
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT",
			q(tableName), q(col.Name)))
	}

	if col.Comment != "" {
		stmts = append(stmts, fmt.Sprintf("COMMENT ON COLUMN %s.%s IS '%s'",
			q(tableName), q(col.Name), strings.ReplaceAll(col.Comment, "'", "''")))
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
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", q(tableName), q(colName)))
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
		sql = fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY (%s)", q(tableName), strings.Join(cols, ", "))
	} else {
		uniqueStr := ""
		if idx.Unique {
			uniqueStr = "UNIQUE "
		}
		cols := make([]string, len(idx.Columns))
		for i, c := range idx.Columns {
			cols[i] = q(c)
		}
		sql = fmt.Sprintf("CREATE %sINDEX %s ON %s (%s)", uniqueStr, q(idx.Name), q(tableName), strings.Join(cols, ", "))
	}
	_, err := db.Exec(sql)
	return err
}

func (p *postgresProvider) DropIndex(db *sql.DB, dbName, tableName, idxName string, isPrimary bool, autoIncCols []string) error {
	q := p.Quote
	if isPrimary {
		_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", q(tableName), q(idxName)))
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
