package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/hazz-dev/svcmon/internal/checker"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS checks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    service     TEXT    NOT NULL,
    status      TEXT    NOT NULL CHECK(status IN ('up', 'down')),
    response_ms INTEGER NOT NULL,
    error       TEXT    NOT NULL DEFAULT '',
    checked_at  TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_checks_service ON checks(service);
CREATE INDEX IF NOT EXISTS idx_checks_checked_at ON checks(checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_checks_service_checked ON checks(service, checked_at DESC);
`

// Check is a stored check result.
type Check struct {
	ID         int64
	Service    string
	Status     string
	ResponseMs int64
	Error      string
	CheckedAt  time.Time
}

// DB wraps a SQLite database.
type DB struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and applies the schema.
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite at %q: %w", path, err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("applying pragma %q: %w", p, err)
		}
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}

	return &DB{db: db}, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// InsertCheck persists a check result.
func (d *DB) InsertCheck(ctx context.Context, r checker.CheckResult) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO checks (service, status, response_ms, error, checked_at) VALUES (?, ?, ?, ?, ?)`,
		r.ServiceName,
		string(r.Status),
		r.ResponseTime.Milliseconds(),
		r.Error,
		r.CheckedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("inserting check for %q: %w", r.ServiceName, err)
	}
	return nil
}

// LatestCheck returns the most recent check for the given service, or nil if none.
func (d *DB) LatestCheck(ctx context.Context, service string) (*Check, error) {
	row := d.db.QueryRowContext(ctx,
		`SELECT id, service, status, response_ms, error, checked_at FROM checks WHERE service = ? ORDER BY checked_at DESC LIMIT 1`,
		service,
	)
	c, err := scanCheck(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying latest check for %q: %w", service, err)
	}
	return c, nil
}

// ServiceHistory returns paginated check history for a service plus the total count.
func (d *DB) ServiceHistory(ctx context.Context, service string, limit, offset int) ([]Check, int, error) {
	var total int
	err := d.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM checks WHERE service = ?`, service,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting checks for %q: %w", service, err)
	}

	rows, err := d.db.QueryContext(ctx,
		`SELECT id, service, status, response_ms, error, checked_at FROM checks WHERE service = ? ORDER BY checked_at DESC LIMIT ? OFFSET ?`,
		service, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("querying history for %q: %w", service, err)
	}
	defer rows.Close()

	checks, err := scanChecks(rows)
	if err != nil {
		return nil, 0, err
	}
	return checks, total, nil
}

// AllLatest returns the most recent check for each service.
func (d *DB) AllLatest(ctx context.Context) ([]Check, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT id, service, status, response_ms, error, checked_at
		FROM checks
		WHERE id IN (
			SELECT MAX(id) FROM checks GROUP BY service
		)
		ORDER BY service
	`)
	if err != nil {
		return nil, fmt.Errorf("querying all latest: %w", err)
	}
	defer rows.Close()
	return scanChecks(rows)
}

// UptimePercent returns the percentage of "up" checks in the last N checks for a service.
func (d *DB) UptimePercent(ctx context.Context, service string, last int) (float64, error) {
	var total int
	var upCount sql.NullInt64
	err := d.db.QueryRowContext(ctx, `
		SELECT COUNT(*), SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END)
		FROM (
			SELECT status FROM checks WHERE service = ? ORDER BY checked_at DESC LIMIT ?
		)
	`, service, last).Scan(&total, &upCount)
	if err != nil {
		return 0, fmt.Errorf("calculating uptime for %q: %w", service, err)
	}
	if total == 0 {
		return 0, nil
	}
	return float64(upCount.Int64) / float64(total) * 100, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanCheck(row scanner) (*Check, error) {
	var c Check
	var checkedAt string
	err := row.Scan(&c.ID, &c.Service, &c.Status, &c.ResponseMs, &c.Error, &checkedAt)
	if err != nil {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339Nano, checkedAt)
	if err != nil {
		// Fallback to RFC3339 without sub-second precision.
		t, err = time.Parse(time.RFC3339, checkedAt)
		if err != nil {
			return nil, fmt.Errorf("parsing checked_at %q: %w", checkedAt, err)
		}
	}
	c.CheckedAt = t
	return &c, nil
}

func scanChecks(rows *sql.Rows) ([]Check, error) {
	var checks []Check
	for rows.Next() {
		c, err := scanCheck(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning check row: %w", err)
		}
		checks = append(checks, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating check rows: %w", err)
	}
	return checks, nil
}
