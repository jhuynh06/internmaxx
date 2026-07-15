//go:build live

package scraper

import (
	"context"
	"testing"
	"time"

	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

func TestShopifyLive(t *testing.T) {
	c := registry.Company{Name: "Shopify", ATS: "shopify", Slug: "shopify"}
	jobs, err := Shopify{}.Fetch(context.Background(), NewClient(300*time.Millisecond), c)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("expected at least one job")
	}
	t.Logf("shopify: %d jobs", len(jobs))
	for i, j := range jobs {
		if i >= 3 {
			break
		}
		t.Logf("  %q id=%s link=%s region=%q date=%s modality=%s type=%s", j.Position, j.ID, j.Link, j.Region, j.Date.Format(time.RFC3339), j.Modality, j.EmploymentType)
	}
}
