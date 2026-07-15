package scraper

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// Microsoft scrapes the Eightfold PCSX search API behind
// jobs.careers.microsoft.com. It queries both "intern" and "internship" and
// dedups by id: PCSX full-text match is token-exact, and live probing showed
// neither term's result set contains the other's (the filter package narrows
// precisely afterwards).
type Microsoft struct{}

func (Microsoft) Source() string { return "microsoft" }

const (
	microsoftPageSize = 10 // fixed by the API; its num param is ignored
	microsoftMaxPages = 60 // safety bound (=600 postings per query)
)

var microsoftQueries = []string{"intern", "internship"}

type microsoftResponse struct {
	Data struct {
		Positions []models.MicrosoftJob `json:"positions"`
		Count     int                   `json:"count"`
	} `json:"data"`
}

func (Microsoft) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	var jobs []models.Job
	seen := map[string]bool{}
	for _, q := range microsoftQueries {
		for page := 0; page < microsoftMaxPages; page++ {
			u := fmt.Sprintf(
				"https://apply.careers.microsoft.com/api/pcsx/search?domain=microsoft.com&query=%s&start=%d",
				url.QueryEscape(q), page*microsoftPageSize)
			var resp microsoftResponse
			if err := getJSON(ctx, client, u, &resp); err != nil {
				return nil, err
			}
			for _, j := range resp.Data.Positions {
				if seen[j.ID.String()] {
					continue
				}
				seen[j.ID.String()] = true
				job := j.ToJob()
				job.Company = c.Name
				jobs = append(jobs, job)
			}
			if len(resp.Data.Positions) < microsoftPageSize || (page+1)*microsoftPageSize >= resp.Data.Count {
				break
			}
		}
	}
	return jobs, nil
}
