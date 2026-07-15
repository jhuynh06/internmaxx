package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// seedJob inserts a seen_jobs row directly (bypassing Diff's seeding logic).
func seedJob(t *testing.T, s *Store, key, company, title string, firstSeen time.Time) {
	t.Helper()
	_, err := s.db.Exec(
		`INSERT INTO seen_jobs (key, company, title, url, first_seen, notified_at) VALUES (?,?,?,?,?,?)`,
		key, company, title, "https://example.com/"+key, firstSeen.UTC().Format(time.RFC3339), nil)
	if err != nil {
		t.Fatalf("seed %s: %v", key, err)
	}
}

func TestListJobs(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now().UTC()
	// Two rows share a first_seen to exercise the (first_seen, key) tie-break.
	seedJob(t, s, "b-key", "OpenAI", "SWE Intern", now)
	seedJob(t, s, "a-key", "OpenAI", "ML Intern", now)
	seedJob(t, s, "old", "Jane Street", "Quant Intern", now.AddDate(0, 0, -30))
	seedJob(t, s, "mid", "Anthropic", "Research Intern", now.AddDate(0, 0, -3))

	ctx := context.Background()

	t.Run("newest first with deterministic tie-break", func(t *testing.T) {
		got, total, err := s.ListJobs(ctx, ListJobsOpts{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if total != 4 {
			t.Fatalf("total = %d, want 4", total)
		}
		// Same-timestamp rows come first (newest), ordered by key ASC.
		want := []string{"a-key", "b-key", "mid", "old"}
		for i, w := range want {
			if got[i].Key != w {
				t.Errorf("row %d = %s, want %s", i, got[i].Key, w)
			}
		}
	})

	t.Run("days cutoff via string comparison", func(t *testing.T) {
		got, total, err := s.ListJobs(ctx, ListJobsOpts{Since: now.AddDate(0, 0, -7), Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if total != 3 { // excludes the 30-day-old row
			t.Fatalf("total = %d, want 3", total)
		}
		for _, j := range got {
			if j.Key == "old" {
				t.Error("30-day-old row leaked past the 7-day cutoff")
			}
		}
	})

	t.Run("company match is case-insensitive", func(t *testing.T) {
		got, total, err := s.ListJobs(ctx, ListJobsOpts{Company: "openai", Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if total != 2 || len(got) != 2 {
			t.Fatalf("total/len = %d/%d, want 2/2", total, len(got))
		}
	})

	t.Run("pagination offset and total", func(t *testing.T) {
		page1, total, err := s.ListJobs(ctx, ListJobsOpts{Limit: 2, Offset: 0})
		if err != nil {
			t.Fatal(err)
		}
		page2, _, err := s.ListJobs(ctx, ListJobsOpts{Limit: 2, Offset: 2})
		if err != nil {
			t.Fatal(err)
		}
		if total != 4 {
			t.Fatalf("total = %d, want 4", total)
		}
		if len(page1) != 2 || len(page2) != 2 {
			t.Fatalf("page sizes = %d/%d, want 2/2", len(page1), len(page2))
		}
		if page1[1].Key == page2[0].Key {
			t.Error("page boundary overlaps")
		}
	})

	t.Run("empty result returns zero total", func(t *testing.T) {
		got, total, err := s.ListJobs(ctx, ListJobsOpts{Company: "NoSuchCo", Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if total != 0 || len(got) != 0 {
			t.Fatalf("total/len = %d/%d, want 0/0", total, len(got))
		}
	})
}
