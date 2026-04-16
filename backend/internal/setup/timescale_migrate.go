// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Resolve bundled Timescale schema SQL paths and execute migration scripts
// via PostgreSQL simple-query protocol (multi-statement), used by the first-run setup API.
package setup

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Ordered migration files under infrastructure/sql_scripts (or SQL_OPTIMA_SQL_SCRIPTS_DIR).
var migrationScriptFiles = []string{
	"00_timescale_schema.sql",
	"01_seed_data.sql",
	"02_rule_engine.sql",
}

func MigrationScriptName(step int) (string, error) {
	if step < 0 || step >= len(migrationScriptFiles) {
		return "", fmt.Errorf("invalid migration step %d (expected 0–%d)", step, len(migrationScriptFiles)-1)
	}
	return migrationScriptFiles[step], nil
}

// ResolveMigrationsDir returns the directory containing 00_timescale_schema.sql.
func ResolveMigrationsDir() (string, error) {
	if d := strings.TrimSpace(os.Getenv("SQL_OPTIMA_SQL_SCRIPTS_DIR")); d != "" {
		if ok, err := hasMigrationMarker(d); err == nil && ok {
			return filepath.Clean(d), nil
		}
		return "", fmt.Errorf("SQL_OPTIMA_SQL_SCRIPTS_DIR is set but migration files were not found in %q", d)
	}
	candidates := []string{
		filepath.Join("..", "infrastructure", "sql_scripts"),
		filepath.Join("infrastructure", "sql_scripts"),
	}
	wd, _ := os.Getwd()
	if wd != "" {
		candidates = append(candidates,
			filepath.Join(wd, "..", "infrastructure", "sql_scripts"),
			filepath.Join(wd, "infrastructure", "sql_scripts"),
		)
	}
	for _, c := range candidates {
		if ok, err := hasMigrationMarker(c); err == nil && ok {
			abs, errA := filepath.Abs(c)
			if errA == nil && abs != "" {
				return abs, nil
			}
			return filepath.Clean(c), nil
		}
	}
	return "", errors.New("could not locate infrastructure/sql_scripts (set SQL_OPTIMA_SQL_SCRIPTS_DIR to the directory containing 00_timescale_schema.sql)")
}

func hasMigrationMarker(dir string) (bool, error) {
	st, err := os.Stat(filepath.Join(dir, migrationScriptFiles[0]))
	if err != nil {
		return false, err
	}
	return !st.IsDir(), nil
}

// TSConnParams holds connection details for a one-off migration connection.
type TSConnParams struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
	SSLMode  string
}

func (p TSConnParams) parsePoolConfig() (*pgxpool.Config, error) {
	ssl := strings.TrimSpace(p.SSLMode)
	if ssl == "" {
		ssl = "require"
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(p.Username, p.Password),
		Host:   net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
		Path:   "/" + strings.TrimPrefix(strings.TrimSpace(p.Database), "/"),
	}
	q := url.Values{}
	q.Set("sslmode", ssl)
	u.RawQuery = q.Encode()
	return pgxpool.ParseConfig(u.String())
}

// ExecMigrationScript runs the full SQL file using simple-query (multi-statement) execution.
func ExecMigrationScript(ctx context.Context, p TSConnParams, sql []byte) (commandSummary string, err error) {
	cfg, err := p.parsePoolConfig()
	if err != nil {
		return "", err
	}
	cfg.MaxConns = 2
	cfg.MinConns = 0
	cfg.ConnConfig.RuntimeParams["application_name"] = "sql-optima-setup-migrate"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("create pool: %w", err)
	}
	defer pool.Close()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return "", fmt.Errorf("acquire: %w", err)
	}
	defer conn.Release()

	pgConn := conn.Conn().PgConn()
	// Many managed Postgres roles use an empty search_path; bundled DDL uses unqualified
	// names in public. Without this, CREATE can fail with SQLSTATE 3F000 ("no schema has been
	// selected to create in").
	const sessionPrefix = "SET search_path TO public, pg_catalog;\n\n"
	fullSQL := sessionPrefix + string(sql)
	mrr := pgConn.Exec(ctx, fullSQL)
	if mrr == nil {
		return "", errors.New("exec returned nil MultiResultReader")
	}

	var tags []string
	for mrr.NextResult() {
		rr := mrr.ResultReader()
		for rr.NextRow() {
			_ = rr.Values()
		}
		tag, cerr := rr.Close()
		if cerr != nil {
			_ = mrr.Close()
			return "", fmt.Errorf("statement failed: %w", cerr)
		}
		if ts := tag.String(); ts != "" {
			tags = append(tags, ts)
		}
	}
	if err = mrr.Close(); err != nil {
		return "", err
	}

	summary := fmt.Sprintf("%d statement(s) completed", len(tags))
	if len(tags) > 0 {
		keep := tags
		if len(keep) > 25 {
			keep = append([]string{fmt.Sprintf("…(%d statements total)", len(tags))}, keep[len(tags)-12:]...)
		}
		summary += ": " + strings.Join(keep, "; ")
	}
	return summary, nil
}

// ReadMigrationFile loads a migration script from disk.
func ReadMigrationFile(dir string, step int) ([]byte, string, error) {
	name, err := MigrationScriptName(step)
	if err != nil {
		return nil, "", err
	}
	path := filepath.Join(dir, name)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", path, err)
	}
	return b, name, nil
}

// RunMigrationStep reads SQL from dir and executes it against TimescaleDB.
func RunMigrationStep(ctx context.Context, dir string, step int, p TSConnParams) (fileName string, summary string, err error) {
	sql, name, err := ReadMigrationFile(dir, step)
	if err != nil {
		return "", "", err
	}
	summary, err = ExecMigrationScript(ctx, p, sql)
	return name, summary, err
}

// TruncateOutput limits log size returned to clients.
func TruncateOutput(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return "…(truncated)\n" + s[len(s)-max:]
}
