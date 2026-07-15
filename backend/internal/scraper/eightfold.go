package scraper

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// Eightfold scrapes any Eightfold-backed careers site via the registry's
// `eightfold: {host, domain, query}` block. Query is optional: Eightfold's
// fuzzy search can miss intern titles on small boards (Millennium matches 1 of
// 224 for "intern"), so such boards omit it and fetch everything — the filter
// package narrows afterward. Netflix has its own scraper for dedup-key
// stability; new Eightfold companies should use this one.
type Eightfold struct{}

func (Eightfold) Source() string { return "eightfold" }

// Eightfold silently caps a page at 10 positions no matter what `num` asks
// for, so paginate in steps of what actually comes back.
const eightfoldPageSize = 10

type eightfoldResponse struct {
	Count     int                   `json:"count"`
	Positions []models.EightfoldJob `json:"positions"`
}

func (Eightfold) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	if c.Eightfold == nil || c.Eightfold.Host == "" || c.Eightfold.Domain == "" {
		return nil, fmt.Errorf("eightfold: %s missing eightfold {host,domain} config", c.Name)
	}
	var jobs []models.Job
	for start := 0; ; start += eightfoldPageSize {
		u := fmt.Sprintf(
			"https://%s/api/apply/v2/jobs?domain=%s&query=%s&num=%d&start=%d",
			c.Eightfold.Host, url.QueryEscape(c.Eightfold.Domain),
			url.QueryEscape(c.Eightfold.Query), eightfoldPageSize, start)
		var resp eightfoldResponse
		if err := getJSON(ctx, client, u, &resp); err != nil {
			return nil, err
		}
		for _, j := range resp.Positions {
			job := j.ToJob()
			job.Company = c.Name
			jobs = append(jobs, job)
		}
		if len(resp.Positions) < eightfoldPageSize || start+eightfoldPageSize >= resp.Count {
			break
		}
	}
	return jobs, nil
}
