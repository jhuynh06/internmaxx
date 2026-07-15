// Package scraper fetches raw postings from job sources and converts them to
// the canonical models.Job. Each source is one file; adding a source is one
// implementation plus one entry in the registry map returned by All().
//
// Scrapers do NOT filter — they return every posting the source exposes. The
// intern/role/location filtering happens later in the filter package so the
// two concerns stay independently testable.
package scraper

import (
	"context"
	"net/http"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

type Scraper interface {
	// Source is the key used as companies.yaml `ats:` and models.Job.Source.
	Source() string
	// Fetch returns all postings for one company, unfiltered.
	Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error)
}

// All returns the source-key -> Scraper map. Keep in sync with companies.yaml
// `ats:` values.
func All() map[string]Scraper {
	scrapers := []Scraper{
		Ashby{},
		Greenhouse{},
		Lever{},
		SmartRecruiters{},
		Workday{},
		Amazon{},
		Netflix{},
		Oracle{},
		Eightfold{},
		SIG{},
		Rippling{},
		Uber{},
		DEShaw{},
		Goldman{},
		TwoSigma{},
		Bloomberg{},
		Shopify{},
		Microsoft{},
		Google{},
		Apple{},
		Meta{},
		Citadel{},
	}
	m := make(map[string]Scraper, len(scrapers))
	for _, s := range scrapers {
		m[s.Source()] = s
	}
	return m
}
