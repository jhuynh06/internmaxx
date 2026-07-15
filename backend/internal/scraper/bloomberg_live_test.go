//go:build live

package scraper

import (
	"context"
	"testing"
	"time"

	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

func TestBloombergLive(t *testing.T) {
	c := registry.Company{Name: "Bloomberg", ATS: "bloomberg", Slug: "bloomberg"}
	jobs, err := Bloomberg{}.Fetch(context.Background(), NewClient(300*time.Millisecond), c)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("expected at least one job")
	}
	t.Logf("bloomberg: %d jobs", len(jobs))
	for i, j := range jobs {
		if i >= 3 {
			break
		}
		t.Logf("  %q id=%s link=%s region=%q date=%s", j.Position, j.ID, j.Link, j.Region, j.Date.Format(time.RFC3339))
	}
}
