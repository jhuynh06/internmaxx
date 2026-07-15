//go:build live

package scraper

import (
	"context"
	"testing"
	"time"

	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

func TestTwoSigmaLive(t *testing.T) {
	c := registry.Company{Name: "Two Sigma", ATS: "twosigma", Slug: "twosigma"}
	jobs, err := TwoSigma{}.Fetch(context.Background(), NewClient(300*time.Millisecond), c)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("expected at least one job")
	}
	t.Logf("twosigma: %d jobs", len(jobs))
	for i, j := range jobs {
		if i >= 3 {
			break
		}
		t.Logf("  %q id=%s link=%s region=%q date=%s", j.Position, j.ID, j.Link, j.Region, j.Date.Format(time.RFC3339))
	}
}
