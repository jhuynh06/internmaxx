//go:build live

package scraper

import (
	"context"
	"testing"
	"time"

	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

func TestGoldmanLive(t *testing.T) {
	c := registry.Company{Name: "Goldman Sachs", ATS: "goldman", Slug: "goldman"}
	jobs, err := Goldman{}.Fetch(context.Background(), NewClient(300*time.Millisecond), c)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("expected at least one job")
	}
	t.Logf("goldman: %d jobs", len(jobs))
	for i, j := range jobs {
		if i >= 3 {
			break
		}
		t.Logf("  %q id=%s link=%s region=%q date=%s", j.Position, j.ID, j.Link, j.Region, j.Date.Format(time.RFC3339))
	}
}
