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

// Workday scrapes a company's Workday CXS board. Unlike the other sources it
// needs three coordinates per company (tenant/instance/site, in the registry's
// `workday:` block) and POSTs a paginated search.
//
// It searches "internship" rather than "intern": Workday's match is fuzzy, and
// "intern" also returns internal/international roles — "internship" cuts the
// result set 10-20x (e.g. Micron 2886 -> 241) while still surfacing intern
// titles, which the filter package then narrows precisely.
type Workday struct{}

func (Workday) Source() string { return "workday" }

const (
	workdayPageSize = 20 // Workday caps page size at 20; larger returns empty
	workdayMaxPages = 60 // safety bound (=1200 postings) for huge fuzzy matches
	workdaySearch   = "internship"
)

type workdayRequest struct {
	AppliedFacets map[string]any `json:"appliedFacets"`
	Limit         int            `json:"limit"`
	Offset        int            `json:"offset"`
	SearchText    string         `json:"searchText"`
}

type workdayResponse struct {
	Total       int                 `json:"total"`
	JobPostings []models.WorkdayJob `json:"jobPostings"`
}

func (Workday) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	if c.Workday == nil || c.Workday.Tenant == "" {
		return nil, fmt.Errorf("workday: %s missing workday {tenant,instance,site} config", c.Name)
	}
	w := c.Workday
	host := fmt.Sprintf("%s.%s.myworkdayjobs.com", w.Tenant, w.Instance)
	url := fmt.Sprintf("https://%s/wday/cxs/%s/%s/jobs", host, w.Tenant, w.Site)
	search := workdaySearch
	if w.Search != "" {
		search = w.Search
	}

	var jobs []models.Job
	total := 0 // some tenants (e.g. Snap) report total only on the first page
	for page := 0; page < workdayMaxPages; page++ {
		body, err := json.Marshal(workdayRequest{
			AppliedFacets: map[string]any{},
			Limit:         workdayPageSize,
			Offset:        page * workdayPageSize,
			SearchText:    search,
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
		req.Header.Set("User-Agent", "internmaxx/1.0")

		var resp workdayResponse
		if err := doJSON(client, req, &resp); err != nil {
			return nil, err
		}
		if page == 0 {
			total = resp.Total
		}
		for _, j := range resp.JobPostings {
			job := j.ToJob(host, w.Site)
			job.Company = c.Name
			jobs = append(jobs, job)
		}
		if len(resp.JobPostings) < workdayPageSize || len(jobs) >= total {
			break
		}
	}
	return jobs, nil
}
