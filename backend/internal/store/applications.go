package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
)

// UpsertApplication creates or updates a tracked application. It stamps
// updated_at, and sets applied_at the first time status becomes "applied".
func (s *Store) UpsertApplication(ctx context.Context, a models.Application) error {
	now := time.Now().UTC().Format(time.RFC3339)
	appliedAt := a.AppliedAt
	if appliedAt == "" && a.Status == "applied" {
		// Preserve an earlier applied_at if one exists; else set it now.
		var existing sql.NullString
		_ = s.db.QueryRowContext(ctx, `SELECT applied_at FROM applications WHERE job_key=?`, a.JobKey).Scan(&existing)
		if existing.Valid && existing.String != "" {
			appliedAt = existing.String
		} else {
			appliedAt = now
		}
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO applications (job_key, company, title, url, status, notes, applied_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?)
		 ON CONFLICT(job_key) DO UPDATE SET
		   company=excluded.company, title=excluded.title, url=excluded.url,
		   status=excluded.status, notes=excluded.notes,
		   applied_at=COALESCE(excluded.applied_at, applications.applied_at),
		   updated_at=excluded.updated_at`,
		a.JobKey, a.Company, a.Title, a.URL, a.Status, a.Notes, nullIfEmpty(appliedAt), now)
	return err
}

func (s *Store) GetApplication(ctx context.Context, key string) (models.Application, bool, error) {
	var a models.Application
	var url, notes, appliedAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT job_key, company, title, url, status, notes, applied_at, updated_at FROM applications WHERE job_key=?`, key).
		Scan(&a.JobKey, &a.Company, &a.Title, &url, &a.Status, &notes, &appliedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return models.Application{}, false, nil
	}
	if err != nil {
		return models.Application{}, false, err
	}
	a.URL, a.Notes, a.AppliedAt = url.String, notes.String, appliedAt.String
	return a, true, nil
}

// ListApplications returns all tracked applications, newest-updated first,
// optionally filtered by status ("" = all).
func (s *Store) ListApplications(ctx context.Context, status string) ([]models.Application, error) {
	q := `SELECT job_key, company, title, url, status, notes, applied_at, updated_at FROM applications`
	args := []any{}
	if status != "" {
		q += ` WHERE status=?`
		args = append(args, status)
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Application
	for rows.Next() {
		var a models.Application
		var url, notes, appliedAt sql.NullString
		if err := rows.Scan(&a.JobKey, &a.Company, &a.Title, &url, &a.Status, &notes, &appliedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.URL, a.Notes, a.AppliedAt = url.String, notes.String, appliedAt.String
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) DeleteApplication(ctx context.Context, key string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM applications WHERE job_key=?`, key)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
