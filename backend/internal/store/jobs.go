package store

import (
	"context"
	"time"
)

// SeenJob is the read-only projection of a seen_jobs row exposed by the browse
// API. FirstSeen is the RFC3339 UTC time the scraper first saw the posting —
// the "new to me" timestamp; the ATS-native posted date is not persisted.
type SeenJob struct {
	Key       string `json:"key"`
	Company   string `json:"company"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	FirstSeen string `json:"first_seen"`
}

// ListJobsOpts filters and pages the seen_jobs listing. Zero values mean "no
// constraint": Company == "" lists all companies; a zero Since lists all time.
type ListJobsOpts struct {
	Company string    // case-insensitive exact match on seen_jobs.company
	Since   time.Time // include rows with first_seen >= Since (UTC)
	Limit   int
	Offset  int
}

// ListJobs returns seen postings newest-first (first_seen DESC, with key ASC as
// a deterministic tie-break because whole scrape batches share one first_seen)
// plus the total count matching the filter (ignoring limit/offset) for
// pagination metadata.
func (s *Store) ListJobs(ctx context.Context, o ListJobsOpts) ([]SeenJob, int, error) {
	where := ""
	var args []any
	if o.Company != "" {
		where += " WHERE company = ? COLLATE NOCASE"
		args = append(args, o.Company)
	}
	if !o.Since.IsZero() {
		if where == "" {
			where += " WHERE "
		} else {
			where += " AND "
		}
		// Bind as an RFC3339 string, never a raw time.Time: first_seen is TEXT
		// and every writer stores time.Now().UTC().Format(time.RFC3339), so the
		// lexicographic comparison is chronological only against that format.
		where += "first_seen >= ?"
		args = append(args, o.Since.UTC().Format(time.RFC3339))
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM seen_jobs`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := `SELECT key, company, title, url, first_seen FROM seen_jobs` + where +
		` ORDER BY first_seen DESC, key ASC LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, q, append(args, o.Limit, o.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []SeenJob
	for rows.Next() {
		var j SeenJob
		if err := rows.Scan(&j.Key, &j.Company, &j.Title, &j.URL, &j.FirstSeen); err != nil {
			return nil, 0, err
		}
		out = append(out, j)
	}
	return out, total, rows.Err()
}
