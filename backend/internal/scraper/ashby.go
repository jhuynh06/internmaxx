package scraper

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

type Ashby struct{}

func (Ashby) Source() string { return "ashby" }

type ashbyResponse struct {
	Jobs []models.AshbyJob `json:"jobs"`
}

func (Ashby) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	url := fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s?includeCompensation=true", c.Slug)
	var raw ashbyResponse
	if err := getJSON(ctx, client, url, &raw); err != nil {
		return nil, err
	}
	jobs := make([]models.Job, 0, len(raw.Jobs))
	for _, j := range raw.Jobs {
		job := j.ToJob()
		job.Company = c.Name
		jobs = append(jobs, job)
	}
	return jobs, nil
}
