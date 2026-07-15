//go:build live

package scraper

import (
	"context"
	"testing"
	"time"

	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

func TestCitadelLive(t *testing.T) {
	companies := []registry.Company{
		{Name: "Citadel", ATS: "citadel", Slug: "citadel", Tier: 1, Group: "quant"},
		{Name: "Citadel Securities", ATS: "citadel", Slug: "citadelsecurities", Tier: 1, Group: "quant"},
	}
	client := NewClient(300 * time.Millisecond)
	for _, c := range companies {
		t.Run(c.Slug, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			jobs, err := Citadel{}.Fetch(ctx, client, c)
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if len(jobs) == 0 {
				t.Fatalf("expected at least one %s posting", c.Name)
			}
			t.Logf("fetched %d jobs", len(jobs))
			for i, j := range jobs {
				if i >= 3 {
					break
				}
				t.Logf("sample: %q id=%s link=%s region=%q", j.Position, j.ID, j.Link, j.Region)
			}
		})
	}
}
