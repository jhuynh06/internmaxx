package scraper

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

type SmartRecruiters struct{}

func (SmartRecruiters) Source() string { return "smartrecruiters" }

type srResponse struct {
	TotalFound int                         `json:"totalFound"`
	Content    []models.SmartRecruitersJob `json:"content"`
}

func (SmartRecruiters) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	// NOTE: SmartRecruiters returns HTTP 200 with an empty result set for ANY
	// slug — validity is totalFound > 0, never the status code. The endpoint is
	// paginated (max 100/page).
	const pageSize = 100
	var jobs []models.Job
	for offset := 0; ; offset += pageSize {
		url := fmt.Sprintf(
			"https://api.smartrecruiters.com/v1/companies/%s/postings?limit=%d&offset=%d",
			c.Slug, pageSize, offset,
		)
		var page srResponse
		if err := getJSON(ctx, client, url, &page); err != nil {
			return nil, err
		}
		for _, j := range page.Content {
			job := j.ToJob()
			job.Company = c.Name
			jobs = append(jobs, job)
		}
		if len(page.Content) < pageSize {
			break
		}
	}
	return jobs, nil
}
