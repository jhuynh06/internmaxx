// Package store persists which postings have been seen, so a job alerts exactly
// once. It uses SQLite via the pure-Go modernc.org/sqlite driver (no CGO —
// cross-compiles cleanly to the Azure linux/amd64 VM).
//
// Seeding is the anti-spam guarantee: the first time a scope (source+company,
// or the aggregator as a whole) is scraped, every posting is recorded as
// already-notified and NOTHING is sent. Only postings that appear after seeding
// trigger alerts. Without this, adding the registry would fire ~15k messages.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
)

type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS seen_jobs (
    key         TEXT PRIMARY KEY,
    company     TEXT NOT NULL,
    title       TEXT NOT NULL,
    url         TEXT NOT NULL,
    first_seen  TEXT NOT NULL,
    notified_at TEXT
);
CREATE TABLE IF NOT EXISTS scope_state (
    scope     TEXT PRIMARY KEY,  -- e.g. "ashby/OpenAI" or "simplify"
    seeded_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS candidate_companies (
    name       TEXT PRIMARY KEY,  -- aggregator company not in the registry
    first_seen TEXT NOT NULL,
    last_seen  TEXT NOT NULL,
    sightings  INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS applications (
    job_key    TEXT PRIMARY KEY,
    company    TEXT NOT NULL,
    title      TEXT NOT NULL,
    url        TEXT,
    status     TEXT NOT NULL,     -- saved|applied|oa|phone|onsite|offer|rejected|ghosted
    notes      TEXT,
    applied_at TEXT,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_applications_status ON applications(status);
CREATE INDEX IF NOT EXISTS idx_seen_company ON seen_jobs(company);
-- Partial index so the per-cycle "pending" lookup touches only unsent rows,
-- not the full ~15k-row table.
CREATE INDEX IF NOT EXISTS idx_seen_pending ON seen_jobs(first_seen) WHERE notified_at IS NULL;
-- Serves the browse API's "newest across all companies" listing: an unfiltered
-- ORDER BY first_seen DESC can't use the partial idx_seen_pending above.
CREATE INDEX IF NOT EXISTS idx_seen_first_seen ON seen_jobs(first_seen DESC, key);
`

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	// SQLite handles one writer at a time; keep a single connection and enable
	// WAL for concurrent readers.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: pragma: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Diff records the given jobs for a scope and returns the ones that should be
// notified. On the scope's first-ever call (seeding) it records everything as
// already-notified and returns nil — no alerts. Thereafter it returns only
// jobs whose key has not been seen before.
func (s *Store) Diff(ctx context.Context, scope string, jobs []models.Job) ([]models.Job, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var seededAt sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT seeded_at FROM scope_state WHERE scope = ?`, scope).Scan(&seededAt)
	seeded := err == nil
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var toNotify []models.Job

	for _, j := range jobs {
		key := j.Key()

		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM seen_jobs WHERE key = ?`, key).Scan(new(int)); err == nil {
			exists = true
		} else if err != sql.ErrNoRows {
			return nil, err
		}
		if exists {
			continue
		}

		var notifiedAt any
		if seeded {
			notifiedAt = nil // pending notification
			toNotify = append(toNotify, j)
		} else {
			notifiedAt = now // seeding: mark as already handled
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO seen_jobs (key, company, title, url, first_seen, notified_at) VALUES (?,?,?,?,?,?)`,
			key, j.Company, j.Position, j.Link, now, notifiedAt,
		); err != nil {
			return nil, err
		}
	}

	if !seeded {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO scope_state (scope, seeded_at) VALUES (?, ?)`, scope, now,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if !seeded {
		return nil, nil
	}
	return toNotify, nil
}

// PendingRow is an inserted-but-not-yet-notified posting. It carries the key so
// it can be marked after a successful (re)send. Fields are the subset persisted
// in seen_jobs — enough for a basic alert if the original in-memory Job is gone
// (e.g. the notifier was down when it was first seen).
type PendingRow struct {
	Key     string
	Company string
	Title   string
	URL     string
}

// Pending returns postings recorded with notified_at IS NULL, so a transient
// notifier outage self-heals on the next cycle rather than dropping alerts.
func (s *Store) Pending(ctx context.Context, limit int) ([]PendingRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, company, title, url FROM seen_jobs WHERE notified_at IS NULL ORDER BY first_seen LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingRow
	for rows.Next() {
		var r PendingRow
		if err := rows.Scan(&r.Key, &r.Company, &r.Title, &r.URL); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RecordCandidates upserts aggregator company names that aren't in the registry,
// so you can review "companies I'm not tracking yet" and promote them. Returns
// the names that were seen for the first time (worth logging).
func (s *Store) RecordCandidates(ctx context.Context, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var fresh []string
	for _, name := range names {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO candidate_companies (name, first_seen, last_seen, sightings) VALUES (?,?,?,1)
			 ON CONFLICT(name) DO UPDATE SET last_seen=excluded.last_seen, sightings=sightings+1`,
			name, now, now)
		if err != nil {
			return nil, err
		}
		// A brand-new row affects 1 row via INSERT; the upsert path also reports
		// changes, so detect novelty by checking sightings after.
		var sightings int
		if err := tx.QueryRowContext(ctx, `SELECT sightings FROM candidate_companies WHERE name=?`, name).Scan(&sightings); err == nil && sightings == 1 {
			if n, _ := res.RowsAffected(); n > 0 {
				fresh = append(fresh, name)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return fresh, nil
}

// MarkNotified stamps notified_at on the given job keys after a successful send.
func (s *Store) MarkNotified(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `UPDATE seen_jobs SET notified_at = ? WHERE key = ? AND notified_at IS NULL`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, k := range keys {
		if _, err := stmt.ExecContext(ctx, now, k); err != nil {
			return err
		}
	}
	return tx.Commit()
}
