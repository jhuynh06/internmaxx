//go:build live

package scraper

import (
	"context"
	"testing"
	"time"

	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

func TestGoogleLive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	c := registry.Company{Name: "Google", ATS: "google", Slug: "google", Tier: 1, Group: "software"}
	jobs, err := Google{}.Fetch(ctx, NewClient(300*time.Millisecond), c)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("expected at least one Google posting")
	}
	t.Logf("fetched %d jobs", len(jobs))
	for i, j := range jobs {
		if i >= 3 {
			break
		}
		t.Logf("sample: %q id=%s link=%s region=%q date=%s", j.Position, j.ID, j.Link, j.Region, j.Date.Format(time.RFC3339))
	}
}
