package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

type mysqlProvider struct{}

func init() {
	Register("mysql", &mysqlProvider{})
}

func (p *mysqlProvider) DSN(host string, port int, user, password, dbName string) string {
	addr := host
	if port > 0 {
		addr = fmt.Sprintf("%s:%d", host, port)
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=true&loc=Local&timeout=10s&readTimeout=30s", user, password, addr, dbName)
	return dsn
}

func (p *mysqlProvider) DriverName() string {
	return "mysql"
}

func (p *mysqlProvider) Quote(name string) string {
	return "`" + name + "`"
}

func (p *mysqlProvider) PrepareExec(db execer, dbName string) error {
	if dbName == "" {
		return nil
	}
	_, err := db.ExecContext(context.Background(), fmt.Sprintf("USE `%s`", dbName))
	return err
}

func (p *mysqlProvider) GetCapabilities() DBCapabilities {
	return DBCapabilities{
		"columnTypes": mysqlTypes,
		"intTypes":    mysqlIntTypes,
	}
}

// ── Schema discovery ──

func (p *mysqlProvider) GetDatabases(db *sql.DB) ([]string, error) {
	results, err := queryStrings(db, "SHOW DATABASES")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(results))
	for _, row := range results {
		names = append(names, row["Database"])
	}
	return names, nil
}

func (p *mysqlProvider) GetTables(db *sql.DB, dbName string) ([]TableInfo, error) {
	results, err := queryStrings(db, fmt.Sprintf("SHOW FULL TABLES FROM `%s`", dbName))
	if err != nil {
		return nil, fmt.Errorf("get tables: %w", err)
	}
	infos := make([]TableInfo, 0, len(results))
	for _, row := range results {
		var name, tp string
		for key, val := range row {
			if strings.HasPrefix(key, "Tables_in_") {
				name = val
			} else {
				tp = val
			}
		}
		if tp == "BASE TABLE" {
			tp = "table"
		} else if tp == "VIEW" {
			tp = "view"
		}
		infos = append(infos, TableInfo{Name: name, Type: tp})
	}
	return infos, nil
}

func (p *mysqlProvider) GetTableSchema(db *sql.DB, dbName, tableName string) (*SchemaResult, error) {
	colRows, err := queryStrings(db, fmt.Sprintf("SHOW FULL COLUMNS FROM `%s`.`%s`", dbName, tableName))
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	columns := make([]ColumnInfo, 0, len(colRows))
	for _, row := range colRows {
		nullable := strings.EqualFold(row["Null"], "YES")
		isPrimary := row["Key"] == "PRI"
		extra := strings.ToLower(row["Extra"])
		onUpdate := strings.Contains(extra, "on update")

		defVal := row["Default"]
		defaultType := "none"
		if strings.Contains(extra, "auto_increment") {
			defaultType = "auto"
		} else if defVal == "NULL" || (defVal == "" && nullable) {
			defaultType = "null"
			defVal = "NULL"
		} else if defVal != "" {
			defaultType = "value"
		}

		columns = append(columns, ColumnInfo{
			Name:        row["Field"],
			Type:        row["Type"],
			Nullable:    nullable,
			DefaultVal:  defVal,
			DefaultType: defaultType,
			IsPrimary:   isPrimary,
			Comment:     row["Comment"],
			Collation:   row["Collation"],
			OnUpdate:    onUpdate,
		})
	}

	idxRows, err := queryStrings(db, fmt.Sprintf("SHOW INDEX FROM `%s`.`%s`", dbName, tableName))
	if err != nil {
		return nil, fmt.Errorf("get indexes: %w", err)
	}

	idxMap := make(map[string]*IndexInfo)
	var idxOrder []string
	for _, row := range idxRows {
		name := row["Key_name"]
		if _, ok := idxMap[name]; !ok {
			idxMap[name] = &IndexInfo{
				Name:      name,
				Columns:   []string{},
				Unique:    row["Non_unique"] == "0",
				IsPrimary: name == "PRIMARY",
			}
			idxOrder = append(idxOrder, name)
		}
		idxMap[name].Columns = append(idxMap[name].Columns, row["Column_name"])
	}

	indexes := make([]IndexInfo, 0, len(idxOrder))
	for _, name := range idxOrder {
		indexes = append(indexes, *idxMap[name])
	}

	return &SchemaResult{Columns: columns, Indexes: indexes}, nil
}

// ── DDL: Database ──

func (p *mysqlProvider) CreateDatabase(db *sql.DB, dbName string) error {
	_, err := db.Exec(fmt.Sprintf("CREATE DATABASE `%s`", dbName))
	return err
}

func (p *mysqlProvider) DropDatabase(db *sql.DB, dbName string) error {
	_, err := db.Exec(fmt.Sprintf("DROP DATABASE `%s`", dbName))
	return err
}

// ── DDL: Table ──

func (p *mysqlProvider) CreateTable(db *sql.DB, dbName, tableName string) error {
	if dbName != "" {
		if _, err := db.Exec(fmt.Sprintf("USE `%s`", dbName)); err != nil {
			return err
		}
	}
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE `%s` (id INT AUTO_INCREMENT PRIMARY KEY)", tableName))
	return err
}

func (p *mysqlProvider) DropTable(db *sql.DB, dbName, tableName string) error {
	if dbName != "" {
		if _, err := db.Exec(fmt.Sprintf("USE `%s`", dbName)); err != nil {
			return err
		}
	}
	_, err := db.Exec(fmt.Sprintf("DROP TABLE `%s`", tableName))
	return err
}

func (p *mysqlProvider) DropView(db *sql.DB, dbName, viewName string) error {
	if dbName != "" {
		if _, err := db.Exec(fmt.Sprintf("USE `%s`", dbName)); err != nil {
			return err
		}
	}
	_, err := db.Exec(fmt.Sprintf("DROP VIEW `%s`", viewName))
	return err
}

func (p *mysqlProvider) TruncateTable(db *sql.DB, dbName, tableName string) error {
	if dbName != "" {
		if _, err := db.Exec(fmt.Sprintf("USE `%s`", dbName)); err != nil {
			return err
		}
	}
	_, err := db.Exec(fmt.Sprintf("TRUNCATE TABLE `%s`", tableName))
	return err
}

// ── DDL: Column ──

func (p *mysqlProvider) AddColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
	if dbName != "" {
		if _, err := db.Exec(fmt.Sprintf("USE `%s`", dbName)); err != nil {
			return err
		}
	}
	sql := p.buildColumnSQL("ADD COLUMN", tableName, col)
	_, err := db.Exec(sql)
	return err
}

func (p *mysqlProvider) ModifyColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
	if dbName != "" {
		if _, err := db.Exec(fmt.Sprintf("USE `%s`", dbName)); err != nil {
			return err
		}
	}
	sql := p.buildColumnSQL("MODIFY COLUMN", tableName, col)
	_, err := db.Exec(sql)
	return err
}

func (p *mysqlProvider) DropColumn(db *sql.DB, dbName, tableName, colName string) error {
	if dbName != "" {
		if _, err := db.Exec(fmt.Sprintf("USE `%s`", dbName)); err != nil {
			return err
		}
	}
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", p.Quote(tableName), p.Quote(colName)))
	return err
}

// ── DDL: Index ──

func (p *mysqlProvider) AddIndex(db *sql.DB, dbName, tableName string, idx IndexDef) error {
	if dbName != "" {
		if _, err := db.Exec(fmt.Sprintf("USE `%s`", dbName)); err != nil {
			return err
		}
	}

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

func (p *mysqlProvider) DropIndex(db *sql.DB, dbName, tableName, idxName string, isPrimary bool, autoIncCols []string) error {
	if dbName != "" {
		if _, err := db.Exec(fmt.Sprintf("USE `%s`", dbName)); err != nil {
			return err
		}
	}
	q := p.Quote
	if isPrimary {
		sql, err := p.buildDropPK(db, tableName, autoIncCols)
		if err != nil {
			return err
		}
		_, err = db.Exec(sql)
		return err
	}
	_, err := db.Exec(fmt.Sprintf("DROP INDEX %s ON %s", q(idxName), q(tableName)))
	return err
}

// ── SQL builders ──

func (p *mysqlProvider) buildColumnSQL(action, tableName string, col ColumnDef) string {
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
			parts = append(parts, fmt.Sprintf("DEFAULT '%s'", strings.ReplaceAll(col.DefaultVal, "'", "''")))
		} else {
			parts = append(parts, "DEFAULT ''")
		}
	case "auto":
		parts = append(parts, "AUTO_INCREMENT")
	}

	if col.OnUpdate {
		parts = append(parts, "ON UPDATE CURRENT_TIMESTAMP")
	}

	if col.Comment != "" {
		parts = append(parts, fmt.Sprintf("COMMENT '%s'", strings.ReplaceAll(col.Comment, "'", "''")))
	}

	if col.Collation != "" {
		parts = append(parts, "COLLATE "+col.Collation)
	}

	return fmt.Sprintf("ALTER TABLE %s %s %s", q(tableName), action, strings.Join(parts, " "))
}

// buildDropPK handles AUTO_INCREMENT columns that must be modified before dropping PK.
func (p *mysqlProvider) buildDropPK(db *sql.DB, tableName string, autoIncCols []string) (string, error) {
	q := p.Quote
	if len(autoIncCols) > 0 {
		rows, err := queryStrings(db, fmt.Sprintf("SHOW FULL COLUMNS FROM %s", q(tableName)))
		if err != nil {
			return "", fmt.Errorf("get columns for PK drop: %w", err)
		}
		colTypes := make(map[string]string)
		for _, row := range rows {
			colTypes[row["Field"]] = row["Type"]
		}

		var modParts []string
		for _, c := range autoIncCols {
			ct, ok := colTypes[c]
			if !ok {
				ct = "INT"
			}
			modParts = append(modParts, fmt.Sprintf("MODIFY COLUMN %s %s NOT NULL", q(c), ct))
		}
		return fmt.Sprintf("ALTER TABLE %s %s, DROP PRIMARY KEY", q(tableName), strings.Join(modParts, ", ")), nil
	}
	return fmt.Sprintf("ALTER TABLE %s DROP PRIMARY KEY", q(tableName)), nil
}

var mysqlTypes = []string{
	"TINYINT", "TINYINT(4)",
	"SMALLINT", "SMALLINT(6)",
	"MEDIUMINT", "MEDIUMINT(9)",
	"INT", "INT(11)",
	"INTEGER", "INTEGER(11)",
	"BIGINT", "BIGINT(20)",
	"FLOAT", "FLOAT(10,2)",
	"DOUBLE", "DOUBLE(10,2)",
	"DECIMAL", "DECIMAL(10,2)",
	"CHAR", "CHAR(1)",
	"VARCHAR", "VARCHAR(255)",
	"TINYTEXT", "TEXT", "MEDIUMTEXT", "LONGTEXT",
	"BLOB", "TINYBLOB", "MEDIUMBLOB", "LONGBLOB",
	"DATE", "DATETIME", "TIMESTAMP", "TIME", "YEAR",
	"ENUM", "SET", "JSON", "BOOLEAN", "BOOL",
}

var mysqlIntTypes = []string{
	"INT", "INTEGER", "BIGINT", "SMALLINT", "TINYINT", "MEDIUMINT",
}
