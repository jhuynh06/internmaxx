package scraper

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// Netflix scrapes the public Eightfold-backed careers API.
type Netflix struct{}

func (Netflix) Source() string { return "netflix" }

// Eightfold silently caps a page at 10 positions no matter what `num` asks
// for (see eightfold.go), so paginate in steps of 10.
const netflixPageSize = 10

type netflixResponse struct {
	Count     int                 `json:"count"`
	Positions []models.NetflixJob `json:"positions"`
}

func (Netflix) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	var jobs []models.Job
	for start := 0; ; start += netflixPageSize {
		url := fmt.Sprintf(
			"https://explore.jobs.netflix.net/api/apply/v2/jobs?domain=netflix.com&query=intern&num=%d&start=%d",
			netflixPageSize, start)
		var resp netflixResponse
		if err := getJSON(ctx, client, url, &resp); err != nil {
			return nil, err
		}
		for _, j := range resp.Positions {
			job := j.ToJob()
			job.Company = c.Name
			jobs = append(jobs, job)
		}
		if len(resp.Positions) < netflixPageSize || start+netflixPageSize >= resp.Count {
			break
		}
	}
	return jobs, nil
}
