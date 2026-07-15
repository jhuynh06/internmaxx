package scraper

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// Amazon scrapes the public amazon.jobs search JSON. It filters to US interns at
// the source (base_query=intern + normalized_country_code=USA) and paginates.
type Amazon struct{}

func (Amazon) Source() string { return "amazon" }

const amazonPageSize = 100

type amazonResponse struct {
	Hits int                `json:"hits"`
	Jobs []models.AmazonJob `json:"jobs"`
}

func (Amazon) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	var jobs []models.Job
	for offset := 0; ; offset += amazonPageSize {
		url := fmt.Sprintf(
			"https://www.amazon.jobs/en/search.json?base_query=intern&normalized_country_code%%5B%%5D=USA&result_limit=%d&offset=%d&sort=recent",
			amazonPageSize, offset)
		var resp amazonResponse
		if err := getJSON(ctx, client, url, &resp); err != nil {
			return nil, err
		}
		for _, j := range resp.Jobs {
			job := j.ToJob()
			job.Company = c.Name
			jobs = append(jobs, job)
		}
		if len(resp.Jobs) < amazonPageSize || offset+amazonPageSize >= resp.Hits {
			break
		}
	}
	return jobs, nil
}
