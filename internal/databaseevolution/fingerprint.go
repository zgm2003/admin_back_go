package databaseevolution

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-sql-driver/mysql"
)

const (
	serverMetadataQuery = "SELECT @@version, @@sql_mode"
	tablesQuery         = `
SELECT table_name, engine, table_collation, table_comment, COALESCE(auto_increment, 0)
FROM information_schema.tables
WHERE table_schema = ? AND table_type = 'BASE TABLE'
ORDER BY table_name`
	columnsQuery = `
SELECT c.table_name, c.ordinal_position, c.column_name, c.column_type, c.is_nullable,
       c.column_default, c.extra, c.generation_expression, c.column_comment,
       c.character_set_name, c.collation_name
FROM information_schema.columns AS c
JOIN information_schema.tables AS t
  ON t.table_schema = c.table_schema
 AND t.table_name = c.table_name
 AND t.table_type = 'BASE TABLE'
WHERE c.table_schema = ?
ORDER BY c.table_name, c.ordinal_position`
	indexesQuery = `
SELECT s.table_name, s.index_name, s.non_unique, s.index_type, s.seq_in_index,
       s.column_name, s.expression, s.sub_part, s.collation, s.is_visible
FROM information_schema.statistics AS s
JOIN information_schema.tables AS t
  ON t.table_schema = s.table_schema
 AND t.table_name = s.table_name
 AND t.table_type = 'BASE TABLE'
WHERE s.table_schema = ?
ORDER BY s.table_name, s.index_name, s.seq_in_index`
	foreignKeysQuery = `
SELECT kcu.table_name, kcu.constraint_name, kcu.ordinal_position, kcu.column_name,
       kcu.referenced_table_schema, kcu.referenced_table_name, kcu.referenced_column_name,
       rc.update_rule, rc.delete_rule
FROM information_schema.key_column_usage AS kcu
JOIN information_schema.referential_constraints AS rc
  ON rc.constraint_schema = kcu.constraint_schema
 AND rc.constraint_name = kcu.constraint_name
 AND rc.table_name = kcu.table_name
WHERE kcu.constraint_schema = ? AND kcu.referenced_table_name IS NOT NULL
ORDER BY kcu.table_name, kcu.constraint_name, kcu.ordinal_position`
	checksQuery = `
SELECT tc.table_name, tc.constraint_name, cc.check_clause, tc.enforced
FROM information_schema.table_constraints AS tc
JOIN information_schema.check_constraints AS cc
  ON cc.constraint_schema = tc.constraint_schema
 AND cc.constraint_name = tc.constraint_name
WHERE tc.constraint_schema = ? AND tc.constraint_type = 'CHECK'
ORDER BY tc.table_name, tc.constraint_name`
	triggersQuery = `
SELECT trigger_name, event_object_table, event_manipulation, action_timing,
       action_orientation, action_order, action_statement, sql_mode,
       character_set_client, collation_connection, database_collation
FROM information_schema.triggers
WHERE trigger_schema = ?
ORDER BY event_object_table, trigger_name`
	routinesQuery = `
SELECT routine_name, routine_type, dtd_identifier, routine_definition,
       sql_data_access, is_deterministic, security_type, sql_mode,
       routine_comment, character_set_client, collation_connection, database_collation
FROM information_schema.routines
WHERE routine_schema = ?
ORDER BY routine_type, routine_name`
	routineParametersQuery = `
SELECT specific_name, routine_type, ordinal_position, parameter_mode,
       parameter_name, dtd_identifier, character_set_name, collation_name
FROM information_schema.parameters
WHERE specific_schema = ?
ORDER BY routine_type, specific_name, ordinal_position`
	eventsQuery = `
SELECT event_name, event_definition, event_type,
       DATE_FORMAT(execute_at, '%Y-%m-%d %H:%i:%s.%f'),
       interval_value, interval_field,
       DATE_FORMAT(starts, '%Y-%m-%d %H:%i:%s.%f'),
       DATE_FORMAT(ends, '%Y-%m-%d %H:%i:%s.%f'),
       status, on_completion, sql_mode, event_comment, time_zone,
       character_set_client, collation_connection, database_collation
FROM information_schema.events
WHERE event_schema = ?
ORDER BY event_name`
)

// Fingerprint is the structural schema projection. Migration-sensitive row
// counts are captured as baseline evidence by the reconciliation workflow and
// are deliberately excluded from schema_sha256 so empty and imported schemas
// can converge on the same structural hash.
type Fingerprint struct {
	ServerVersion string       `json:"server_version"`
	SQLMode       string       `json:"sql_mode"`
	Schema        string       `json:"schema"`
	Tables        []Table      `json:"tables"`
	ForeignKeys   []ForeignKey `json:"foreign_keys"`
	Checks        []Check      `json:"checks"`
	Triggers      []Trigger    `json:"triggers"`
	Routines      []Routine    `json:"routines"`
	Events        []Event      `json:"events"`
}

type FingerprintDocument struct {
	GitCommit    string `json:"git_commit"`
	SchemaSHA256 string `json:"schema_sha256"`
	Fingerprint
}

type canonicalSchema struct {
	ServerVersion string       `json:"server_version"`
	SQLMode       string       `json:"sql_mode"`
	Tables        []Table      `json:"tables"`
	ForeignKeys   []ForeignKey `json:"foreign_keys"`
	Checks        []Check      `json:"checks"`
	Triggers      []Trigger    `json:"triggers"`
	Routines      []Routine    `json:"routines"`
	Events        []Event      `json:"events"`
}

type Table struct {
	Name          string   `json:"name"`
	Engine        string   `json:"engine"`
	Collation     string   `json:"collation"`
	Comment       string   `json:"comment"`
	AutoIncrement uint64   `json:"-"`
	Columns       []Column `json:"columns"`
	Indexes       []Index  `json:"indexes"`
}

type Column struct {
	Ordinal      int     `json:"ordinal"`
	Name         string  `json:"name"`
	ColumnType   string  `json:"column_type"`
	Nullable     bool    `json:"nullable"`
	Default      *string `json:"default"`
	Extra        string  `json:"extra"`
	Generation   string  `json:"generation"`
	Comment      string  `json:"comment"`
	CharacterSet *string `json:"character_set"`
	Collation    *string `json:"collation"`
}

type Index struct {
	Name    string        `json:"name"`
	Unique  bool          `json:"unique"`
	Type    string        `json:"type"`
	Visible bool          `json:"visible"`
	Columns []IndexColumn `json:"columns"`
}

type IndexColumn struct {
	Ordinal      int     `json:"ordinal"`
	Name         *string `json:"name"`
	Expression   *string `json:"expression"`
	PrefixLength *uint64 `json:"prefix_length"`
	Descending   bool    `json:"descending"`
}

type ForeignKey struct {
	Table            string `json:"table"`
	Name             string `json:"name"`
	Ordinal          int    `json:"ordinal"`
	Column           string `json:"column"`
	ReferencedSchema string `json:"referenced_schema"`
	ReferencedTable  string `json:"referenced_table"`
	ReferencedColumn string `json:"referenced_column"`
	UpdateRule       string `json:"update_rule"`
	DeleteRule       string `json:"delete_rule"`
}

type Check struct {
	Table    string `json:"table"`
	Name     string `json:"name"`
	Clause   string `json:"clause"`
	Enforced bool   `json:"enforced"`
}

type Trigger struct {
	Name                string  `json:"name"`
	Table               string  `json:"table"`
	Event               string  `json:"event"`
	Timing              string  `json:"timing"`
	Orientation         string  `json:"orientation"`
	ActionOrder         int     `json:"action_order"`
	Statement           string  `json:"statement"`
	SQLMode             string  `json:"sql_mode"`
	CharacterSet        *string `json:"character_set_client"`
	ConnectionCollation *string `json:"collation_connection"`
	DatabaseCollation   *string `json:"database_collation"`
}

type Routine struct {
	Name                string             `json:"name"`
	Type                string             `json:"type"`
	ReturnType          *string            `json:"return_type"`
	Definition          *string            `json:"definition"`
	SQLDataAccess       string             `json:"sql_data_access"`
	Deterministic       bool               `json:"deterministic"`
	SecurityType        string             `json:"security_type"`
	SQLMode             string             `json:"sql_mode"`
	Comment             string             `json:"comment"`
	CharacterSet        *string            `json:"character_set_client"`
	ConnectionCollation *string            `json:"collation_connection"`
	DatabaseCollation   *string            `json:"database_collation"`
	Parameters          []RoutineParameter `json:"parameters"`
}

type RoutineParameter struct {
	Ordinal       int     `json:"ordinal"`
	Mode          *string `json:"mode"`
	Name          *string `json:"name"`
	DTDIdentifier string  `json:"dtd_identifier"`
	CharacterSet  *string `json:"character_set"`
	Collation     *string `json:"collation"`
}

type Event struct {
	Name                string  `json:"name"`
	Definition          string  `json:"definition"`
	Type                string  `json:"type"`
	ExecuteAt           *string `json:"execute_at"`
	IntervalValue       *string `json:"interval_value"`
	IntervalField       *string `json:"interval_field"`
	Starts              *string `json:"starts"`
	Ends                *string `json:"ends"`
	Status              string  `json:"status"`
	OnCompletion        string  `json:"on_completion"`
	SQLMode             string  `json:"sql_mode"`
	Comment             string  `json:"comment"`
	TimeZone            string  `json:"time_zone"`
	CharacterSet        *string `json:"character_set_client"`
	ConnectionCollation *string `json:"collation_connection"`
	DatabaseCollation   *string `json:"database_collation"`
}

// CanonicalJSON excludes the physical schema name and normalizes self-referencing
// foreign keys so disposable schemas with identical structure share one hash.
func CanonicalJSON(fingerprint Fingerprint) ([]byte, error) {
	normalized := normalizeFingerprint(fingerprint)
	for index := range normalized.ForeignKeys {
		if normalized.ForeignKeys[index].ReferencedSchema == normalized.Schema {
			normalized.ForeignKeys[index].ReferencedSchema = ""
		}
	}
	return json.Marshal(canonicalSchema{
		ServerVersion: normalized.ServerVersion,
		SQLMode:       normalized.SQLMode,
		Tables:        normalized.Tables,
		ForeignKeys:   normalized.ForeignKeys,
		Checks:        normalized.Checks,
		Triggers:      normalized.Triggers,
		Routines:      normalized.Routines,
		Events:        normalized.Events,
	})
}

func NewFingerprintDocument(gitCommit string, fingerprint Fingerprint) (FingerprintDocument, error) {
	gitCommit = strings.TrimSpace(gitCommit)
	if gitCommit == "" {
		return FingerprintDocument{}, fmt.Errorf("git commit is required")
	}
	canonical, err := CanonicalJSON(fingerprint)
	if err != nil {
		return FingerprintDocument{}, fmt.Errorf("encode canonical fingerprint: %w", err)
	}
	return FingerprintDocument{
		GitCommit:    gitCommit,
		SchemaSHA256: fmt.Sprintf("%x", sha256.Sum256(canonical)),
		Fingerprint:  normalizeFingerprint(fingerprint),
	}, nil
}

func WriteFingerprintDocument(outputPath string, document FingerprintDocument) error {
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return fmt.Errorf("output path is required")
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode fingerprint document: %w", err)
	}
	data = append(data, '\n')

	directory := filepath.Dir(outputPath)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(outputPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary fingerprint document: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure temporary fingerprint document: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write temporary fingerprint document: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary fingerprint document: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary fingerprint document: %w", err)
	}
	if err := os.Rename(temporaryPath, outputPath); err != nil {
		return fmt.Errorf("replace fingerprint document: %w", err)
	}
	committed = true
	return nil
}

func Capture(ctx context.Context, db *sql.DB, schema string) (Fingerprint, error) {
	if db == nil {
		return Fingerprint{}, fmt.Errorf("database connection is required")
	}
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return Fingerprint{}, fmt.Errorf("schema is required")
	}

	result := Fingerprint{Schema: schema}
	if err := db.QueryRowContext(ctx, serverMetadataQuery).Scan(&result.ServerVersion, &result.SQLMode); err != nil {
		return Fingerprint{}, fmt.Errorf("capture server metadata: %w", err)
	}

	tableByName, err := captureTables(ctx, db, schema, &result)
	if err != nil {
		return Fingerprint{}, err
	}
	if err := captureColumns(ctx, db, schema, tableByName, &result); err != nil {
		return Fingerprint{}, err
	}
	if err := captureIndexes(ctx, db, schema, tableByName, &result); err != nil {
		return Fingerprint{}, err
	}
	if result.ForeignKeys, err = captureForeignKeys(ctx, db, schema); err != nil {
		return Fingerprint{}, err
	}
	if result.Checks, err = captureChecks(ctx, db, schema); err != nil {
		return Fingerprint{}, err
	}
	if result.Triggers, err = captureTriggers(ctx, db, schema); err != nil {
		return Fingerprint{}, err
	}
	if result.Routines, err = captureRoutines(ctx, db, schema); err != nil {
		return Fingerprint{}, err
	}
	if err := captureRoutineParameters(ctx, db, schema, result.Routines); err != nil {
		return Fingerprint{}, err
	}
	if result.Events, err = captureEvents(ctx, db, schema); err != nil {
		return Fingerprint{}, err
	}
	return normalizeFingerprint(result), nil
}

func captureTables(ctx context.Context, db *sql.DB, schema string, result *Fingerprint) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, tablesQuery, schema)
	if err != nil {
		return nil, fmt.Errorf("capture tables: %w", err)
	}
	defer rows.Close()

	tableByName := make(map[string]int)
	for rows.Next() {
		var table Table
		if err := rows.Scan(&table.Name, &table.Engine, &table.Collation, &table.Comment, &table.AutoIncrement); err != nil {
			return nil, fmt.Errorf("scan table: %w", err)
		}
		tableByName[table.Name] = len(result.Tables)
		result.Tables = append(result.Tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tables: %w", err)
	}
	return tableByName, nil
}

func captureColumns(ctx context.Context, db *sql.DB, schema string, tableByName map[string]int, result *Fingerprint) error {
	rows, err := db.QueryContext(ctx, columnsQuery, schema)
	if err != nil {
		return fmt.Errorf("capture columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tableName string
		var column Column
		var nullable string
		var defaultValue sql.NullString
		var characterSet sql.NullString
		var collation sql.NullString
		if err := rows.Scan(&tableName, &column.Ordinal, &column.Name, &column.ColumnType, &nullable, &defaultValue, &column.Extra, &column.Generation, &column.Comment, &characterSet, &collation); err != nil {
			return fmt.Errorf("scan column: %w", err)
		}
		tableIndex, ok := tableByName[tableName]
		if !ok {
			return fmt.Errorf("column references unknown table")
		}
		column.Nullable = nullable == "YES"
		column.Default = nullableStringPointer(defaultValue)
		column.CharacterSet = nullableStringPointer(characterSet)
		column.Collation = nullableStringPointer(collation)
		result.Tables[tableIndex].Columns = append(result.Tables[tableIndex].Columns, column)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate columns: %w", err)
	}
	return nil
}

func captureIndexes(ctx context.Context, db *sql.DB, schema string, tableByName map[string]int, result *Fingerprint) error {
	rows, err := db.QueryContext(ctx, indexesQuery, schema)
	if err != nil {
		return fmt.Errorf("capture indexes: %w", err)
	}
	defer rows.Close()

	indexPositions := make(map[string]int)
	for rows.Next() {
		var tableName string
		var indexName string
		var nonUnique int
		var indexType string
		var column IndexColumn
		var columnName sql.NullString
		var expression sql.NullString
		var prefixLength sql.NullInt64
		var collation sql.NullString
		var visible string
		if err := rows.Scan(&tableName, &indexName, &nonUnique, &indexType, &column.Ordinal, &columnName, &expression, &prefixLength, &collation, &visible); err != nil {
			return fmt.Errorf("scan index: %w", err)
		}
		tableIndex, ok := tableByName[tableName]
		if !ok {
			return fmt.Errorf("index references unknown table")
		}
		key := tableName + "\x00" + indexName
		indexPosition, exists := indexPositions[key]
		if !exists {
			indexPosition = len(result.Tables[tableIndex].Indexes)
			indexPositions[key] = indexPosition
			result.Tables[tableIndex].Indexes = append(result.Tables[tableIndex].Indexes, Index{
				Name:    indexName,
				Unique:  nonUnique == 0,
				Type:    indexType,
				Visible: visible == "YES",
			})
		}
		column.Name = nullableStringPointer(columnName)
		column.Expression = nullableStringPointer(expression)
		column.PrefixLength = nullableUint64Pointer(prefixLength)
		column.Descending = collation.Valid && collation.String == "D"
		result.Tables[tableIndex].Indexes[indexPosition].Columns = append(result.Tables[tableIndex].Indexes[indexPosition].Columns, column)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate indexes: %w", err)
	}
	return nil
}

func captureForeignKeys(ctx context.Context, db *sql.DB, schema string) ([]ForeignKey, error) {
	rows, err := db.QueryContext(ctx, foreignKeysQuery, schema)
	if err != nil {
		return nil, fmt.Errorf("capture foreign keys: %w", err)
	}
	defer rows.Close()

	result := make([]ForeignKey, 0)
	for rows.Next() {
		var row ForeignKey
		if err := rows.Scan(&row.Table, &row.Name, &row.Ordinal, &row.Column, &row.ReferencedSchema, &row.ReferencedTable, &row.ReferencedColumn, &row.UpdateRule, &row.DeleteRule); err != nil {
			return nil, fmt.Errorf("scan foreign key: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate foreign keys: %w", err)
	}
	return result, nil
}

func captureChecks(ctx context.Context, db *sql.DB, schema string) ([]Check, error) {
	rows, err := db.QueryContext(ctx, checksQuery, schema)
	if err != nil {
		return nil, fmt.Errorf("capture checks: %w", err)
	}
	defer rows.Close()

	result := make([]Check, 0)
	for rows.Next() {
		var row Check
		var enforced string
		if err := rows.Scan(&row.Table, &row.Name, &row.Clause, &enforced); err != nil {
			return nil, fmt.Errorf("scan check: %w", err)
		}
		row.Enforced = enforced == "YES"
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checks: %w", err)
	}
	return result, nil
}

func captureTriggers(ctx context.Context, db *sql.DB, schema string) ([]Trigger, error) {
	rows, err := db.QueryContext(ctx, triggersQuery, schema)
	if err != nil {
		return nil, fmt.Errorf("capture triggers: %w", err)
	}
	defer rows.Close()

	result := make([]Trigger, 0)
	for rows.Next() {
		var row Trigger
		var characterSet sql.NullString
		var connectionCollation sql.NullString
		var databaseCollation sql.NullString
		if err := rows.Scan(&row.Name, &row.Table, &row.Event, &row.Timing, &row.Orientation, &row.ActionOrder, &row.Statement, &row.SQLMode, &characterSet, &connectionCollation, &databaseCollation); err != nil {
			return nil, fmt.Errorf("scan trigger: %w", err)
		}
		row.CharacterSet = nullableStringPointer(characterSet)
		row.ConnectionCollation = nullableStringPointer(connectionCollation)
		row.DatabaseCollation = nullableStringPointer(databaseCollation)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate triggers: %w", err)
	}
	return result, nil
}

func captureRoutines(ctx context.Context, db *sql.DB, schema string) ([]Routine, error) {
	rows, err := db.QueryContext(ctx, routinesQuery, schema)
	if err != nil {
		return nil, fmt.Errorf("capture routines: %w", err)
	}
	defer rows.Close()

	result := make([]Routine, 0)
	for rows.Next() {
		var row Routine
		var returnType sql.NullString
		var definition sql.NullString
		var deterministic string
		var characterSet sql.NullString
		var connectionCollation sql.NullString
		var databaseCollation sql.NullString
		if err := rows.Scan(&row.Name, &row.Type, &returnType, &definition, &row.SQLDataAccess, &deterministic, &row.SecurityType, &row.SQLMode, &row.Comment, &characterSet, &connectionCollation, &databaseCollation); err != nil {
			return nil, fmt.Errorf("scan routine: %w", err)
		}
		row.ReturnType = nullableStringPointer(returnType)
		row.Definition = nullableStringPointer(definition)
		row.Deterministic = deterministic == "YES"
		row.CharacterSet = nullableStringPointer(characterSet)
		row.ConnectionCollation = nullableStringPointer(connectionCollation)
		row.DatabaseCollation = nullableStringPointer(databaseCollation)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate routines: %w", err)
	}
	return result, nil
}

func captureRoutineParameters(ctx context.Context, db *sql.DB, schema string, routines []Routine) error {
	routinePositions := make(map[string]int, len(routines))
	for index := range routines {
		routinePositions[routines[index].Type+"\x00"+routines[index].Name] = index
	}

	rows, err := db.QueryContext(ctx, routineParametersQuery, schema)
	if err != nil {
		return fmt.Errorf("capture routine parameters: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var routineName string
		var routineType string
		var parameter RoutineParameter
		var mode sql.NullString
		var name sql.NullString
		var characterSet sql.NullString
		var collation sql.NullString
		if err := rows.Scan(&routineName, &routineType, &parameter.Ordinal, &mode, &name, &parameter.DTDIdentifier, &characterSet, &collation); err != nil {
			return fmt.Errorf("scan routine parameter: %w", err)
		}
		routineIndex, ok := routinePositions[routineType+"\x00"+routineName]
		if !ok {
			return fmt.Errorf("routine parameter references unknown routine")
		}
		parameter.Mode = nullableStringPointer(mode)
		parameter.Name = nullableStringPointer(name)
		parameter.CharacterSet = nullableStringPointer(characterSet)
		parameter.Collation = nullableStringPointer(collation)
		routines[routineIndex].Parameters = append(routines[routineIndex].Parameters, parameter)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate routine parameters: %w", err)
	}
	return nil
}

func captureEvents(ctx context.Context, db *sql.DB, schema string) ([]Event, error) {
	rows, err := db.QueryContext(ctx, eventsQuery, schema)
	if err != nil {
		return nil, fmt.Errorf("capture events: %w", err)
	}
	defer rows.Close()

	result := make([]Event, 0)
	for rows.Next() {
		var row Event
		var executeAt sql.NullString
		var intervalValue sql.NullString
		var intervalField sql.NullString
		var starts sql.NullString
		var ends sql.NullString
		var characterSet sql.NullString
		var connectionCollation sql.NullString
		var databaseCollation sql.NullString
		if err := rows.Scan(&row.Name, &row.Definition, &row.Type, &executeAt, &intervalValue, &intervalField, &starts, &ends, &row.Status, &row.OnCompletion, &row.SQLMode, &row.Comment, &row.TimeZone, &characterSet, &connectionCollation, &databaseCollation); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		row.ExecuteAt = nullableStringPointer(executeAt)
		row.IntervalValue = nullableStringPointer(intervalValue)
		row.IntervalField = nullableStringPointer(intervalField)
		row.Starts = nullableStringPointer(starts)
		row.Ends = nullableStringPointer(ends)
		row.CharacterSet = nullableStringPointer(characterSet)
		row.ConnectionCollation = nullableStringPointer(connectionCollation)
		row.DatabaseCollation = nullableStringPointer(databaseCollation)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return result, nil
}

func nullableStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func nullableUint64Pointer(value sql.NullInt64) *uint64 {
	if !value.Valid || value.Int64 < 0 {
		return nil
	}
	result := uint64(value.Int64)
	return &result
}

func ValidateSchemaDSN(rawDSN string, schema string) error {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return fmt.Errorf("schema is required")
	}
	if strings.TrimSpace(rawDSN) == "" {
		return fmt.Errorf("MYSQL_DSN is required")
	}
	config, err := mysql.ParseDSN(rawDSN)
	if err != nil {
		return fmt.Errorf("MYSQL_DSN is malformed")
	}
	if config.DBName != schema {
		return fmt.Errorf("MYSQL_DSN database does not match requested schema")
	}
	return nil
}

func normalizeFingerprint(fingerprint Fingerprint) Fingerprint {
	result := fingerprint
	result.Tables = append([]Table(nil), fingerprint.Tables...)
	for index := range result.Tables {
		result.Tables[index].Columns = append([]Column(nil), result.Tables[index].Columns...)
		sort.Slice(result.Tables[index].Columns, func(left, right int) bool {
			if result.Tables[index].Columns[left].Ordinal != result.Tables[index].Columns[right].Ordinal {
				return result.Tables[index].Columns[left].Ordinal < result.Tables[index].Columns[right].Ordinal
			}
			return result.Tables[index].Columns[left].Name < result.Tables[index].Columns[right].Name
		})

		result.Tables[index].Indexes = append([]Index(nil), result.Tables[index].Indexes...)
		for indexPosition := range result.Tables[index].Indexes {
			columns := result.Tables[index].Indexes[indexPosition].Columns
			result.Tables[index].Indexes[indexPosition].Columns = append([]IndexColumn(nil), columns...)
			sort.Slice(result.Tables[index].Indexes[indexPosition].Columns, func(left, right int) bool {
				return result.Tables[index].Indexes[indexPosition].Columns[left].Ordinal < result.Tables[index].Indexes[indexPosition].Columns[right].Ordinal
			})
		}
		sort.Slice(result.Tables[index].Indexes, func(left, right int) bool {
			return result.Tables[index].Indexes[left].Name < result.Tables[index].Indexes[right].Name
		})
	}
	sort.Slice(result.Tables, func(left, right int) bool {
		return result.Tables[left].Name < result.Tables[right].Name
	})

	result.ForeignKeys = append([]ForeignKey(nil), fingerprint.ForeignKeys...)
	sort.Slice(result.ForeignKeys, func(left, right int) bool {
		leftKey := result.ForeignKeys[left].Table + "\x00" + result.ForeignKeys[left].Name
		rightKey := result.ForeignKeys[right].Table + "\x00" + result.ForeignKeys[right].Name
		if leftKey != rightKey {
			return leftKey < rightKey
		}
		return result.ForeignKeys[left].Ordinal < result.ForeignKeys[right].Ordinal
	})

	result.Checks = append([]Check(nil), fingerprint.Checks...)
	sort.Slice(result.Checks, func(left, right int) bool {
		return result.Checks[left].Table+"\x00"+result.Checks[left].Name < result.Checks[right].Table+"\x00"+result.Checks[right].Name
	})

	result.Triggers = append([]Trigger(nil), fingerprint.Triggers...)
	sort.Slice(result.Triggers, func(left, right int) bool {
		return result.Triggers[left].Table+"\x00"+result.Triggers[left].Name < result.Triggers[right].Table+"\x00"+result.Triggers[right].Name
	})

	result.Routines = append([]Routine(nil), fingerprint.Routines...)
	for index := range result.Routines {
		result.Routines[index].Parameters = append([]RoutineParameter(nil), fingerprint.Routines[index].Parameters...)
		sort.Slice(result.Routines[index].Parameters, func(left, right int) bool {
			if result.Routines[index].Parameters[left].Ordinal != result.Routines[index].Parameters[right].Ordinal {
				return result.Routines[index].Parameters[left].Ordinal < result.Routines[index].Parameters[right].Ordinal
			}
			return nullableStringValue(result.Routines[index].Parameters[left].Name) < nullableStringValue(result.Routines[index].Parameters[right].Name)
		})
	}
	sort.Slice(result.Routines, func(left, right int) bool {
		return result.Routines[left].Type+"\x00"+result.Routines[left].Name < result.Routines[right].Type+"\x00"+result.Routines[right].Name
	})

	result.Events = append([]Event(nil), fingerprint.Events...)
	sort.Slice(result.Events, func(left, right int) bool {
		return result.Events[left].Name < result.Events[right].Name
	})
	return result
}

func nullableStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
