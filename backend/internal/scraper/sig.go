package scraper

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// SIG scrapes Susquehanna's Jibe/iCIMS-backed careers API. It narrows to
// intern postings at the source (keywords=intern) and paginates, though the
// full intern set currently fits in one 100-item page.
type SIG struct{}

func (SIG) Source() string { return "sig" }

const sigPageSize = 100

type sigResponse struct {
	TotalCount int `json:"totalCount"`
	Jobs       []struct {
		Data models.SIGJob `json:"data"`
	} `json:"jobs"`
}

func (SIG) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	var jobs []models.Job
	for page := 1; ; page++ { // page is 1-based
		url := fmt.Sprintf(
			"https://careers.sig.com/api/jobs?keywords=intern&page=%d&limit=%d",
			page, sigPageSize)
		var resp sigResponse
		if err := getJSON(ctx, client, url, &resp); err != nil {
			return nil, err
		}
		for _, j := range resp.Jobs {
			job := j.Data.ToJob()
			job.Company = c.Name
			jobs = append(jobs, job)
		}
		if len(resp.Jobs) < sigPageSize || len(jobs) >= resp.TotalCount {
			break
		}
	}
	return jobs, nil
}
