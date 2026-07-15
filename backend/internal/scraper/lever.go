package scraper

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

type Lever struct{}

func (Lever) Source() string { return "lever" }

func (Lever) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	// Lever returns a bare JSON array.
	url := fmt.Sprintf("https://api.lever.co/v0/postings/%s?mode=json", c.Slug)
	var raw []models.LeverJob
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
