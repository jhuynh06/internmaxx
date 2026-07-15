package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// Uber scrapes the careers search API behind www.uber.com/careers. It narrows
// to intern postings at the source (params.text=intern) and POSTs a 0-based
// paginated search. Uber's edge blocks unknown User-Agents (406) and requires
// an x-csrf-token header (any value), so the request is built by hand rather
// than through getJSON.
type Uber struct{}

func (Uber) Source() string { return "uber" }

const (
	uberPageSize = 100
	uberMaxPages = 30 // safety bound (=3000 postings) in case totalResults misbehaves
	uberSearch   = "intern"
	uberUA       = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36"
)

type uberRequest struct {
	Params uberParams `json:"params"`
	Limit  int        `json:"limit"`
	Page   int        `json:"page"`
}

type uberParams struct {
	Text string `json:"text"`
}

type uberResponse struct {
	Status string `json:"status"`
	Data   struct {
		Results      []models.UberJob `json:"results"`
		TotalResults struct {
			Low int `json:"low"` // total match count (Long encoded as {low,high})
		} `json:"totalResults"`
	} `json:"data"`
}

func (Uber) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	const url = "https://www.uber.com/api/loadSearchJobsResults?localeCode=en"

	var jobs []models.Job
	for page := 0; page < uberMaxPages; page++ { // page is 0-based
		body, err := json.Marshal(uberRequest{
			Params: uberParams{Text: uberSearch},
			Limit:  uberPageSize,
			Page:   page,
		})
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("x-csrf-token", "x")
		req.Header.Set("User-Agent", uberUA)

		var resp uberResponse
		if err := doJSON(client, req, &resp); err != nil {
			return nil, err
		}
		if resp.Status != "success" {
			return nil, fmt.Errorf("uber: search returned status %q", resp.Status)
		}
		for _, j := range resp.Data.Results {
			job := j.ToJob()
			job.Company = c.Name
			jobs = append(jobs, job)
		}
		if len(resp.Data.Results) < uberPageSize || len(jobs) >= resp.Data.TotalResults.Low {
			break
		}
	}
	return jobs, nil
}
