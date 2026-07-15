//go:build live

package scraper

import (
	"context"
	"testing"
	"time"

	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

func TestRipplingLive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	jobs, err := Rippling{}.Fetch(ctx, NewClient(300*time.Millisecond), registry.Company{Name: "Rippling", Slug: "rippling"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("expected at least one job")
	}
	t.Logf("fetched %d jobs", len(jobs))
	for i, j := range jobs {
		if i >= 3 {
			break
		}
		t.Logf("position=%q id=%s link=%s region=%q date=%s", j.Position, j.ID, j.Link, j.Region, j.Date.Format(time.RFC3339))
	}
}
