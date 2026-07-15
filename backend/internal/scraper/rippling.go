package scraper

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// Rippling scrapes a company's public Rippling ATS board (the registry slug is
// the board segment, so any Rippling-hosted company works, including Rippling
// itself). The endpoint returns the whole board in one unpaginated array.
type Rippling struct{}

func (Rippling) Source() string { return "rippling" }

func (Rippling) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	url := fmt.Sprintf("https://api.rippling.com/platform/api/ats/v1/board/%s/jobs", c.Slug)
	var raw []models.RipplingJob
	if err := getJSON(ctx, client, url, &raw); err != nil {
		return nil, err
	}
	jobs := make([]models.Job, 0, len(raw))
	for _, j := range raw {
		job := j.ToJob()
		job.Company = c.Name
		jobs = append(jobs, job)
	}
	return jobs, nil
}
