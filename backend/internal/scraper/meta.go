package scraper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// Meta scrapes www.metacareers.com via its Relay GraphQL endpoint. Two
// requests: GET /jobs to obtain the LSD anti-CSRF token embedded in the HTML
// (no cookies needed), then POST /graphql replaying the site's own persisted
// query. One response carries every match (no pagination; ~300 for "intern").
//
// Fragile by nature: metaDocID pins a Relay persisted query that Meta can
// rotate on redeploy, and the GET requires Sec-Fetch-* headers (missing them
// is an outright 400). If Fetch starts failing, re-derive the doc_id from the
// CareersJobSearchResultsDataQuery_candidate_portalRelayOperation module in
// the site's JS bundles.
type Meta struct{}

func (Meta) Source() string { return "meta" }

const (
	metaDocID        = "27506805582236862" // CareersJobSearchResultsDataQuery
	metaFriendlyName = "CareersJobSearchResultsDataQuery"
	metaSearch       = "intern"
	metaUA           = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

var metaLSDRe = regexp.MustCompile(`"LSD",\[\],\{"token":"([^"]+)"`)

type metaResponse struct {
	Data struct {
		JobSearch struct {
			AllJobs []models.MetaJob `json:"all_jobs"`
		} `json:"job_search_with_featured_jobs"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (Meta) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	lsd, err := metaFetchLSD(ctx, client)
	if err != nil {
		return nil, err
	}

	variables := fmt.Sprintf(
		`{"isLoggedIn":false,"search_input":{"q":%q,"page":1,"results_per_page":null},"viewasUserID":null}`,
		metaSearch)
	form := url.Values{}
	form.Set("lsd", lsd)
	form.Set("fb_api_caller_class", "RelayModern")
	form.Set("fb_api_req_friendly_name", metaFriendlyName)
	form.Set("variables", variables)
	form.Set("server_timestamps", "true")
	form.Set("doc_id", metaDocID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://www.metacareers.com/graphql", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", metaUA)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("X-FB-LSD", lsd)
	req.Header.Set("X-FB-Friendly-Name", metaFriendlyName)
	req.Header.Set("Origin", "https://www.metacareers.com")
	req.Header.Set("Referer", "https://www.metacareers.com/jobs")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	var resp metaResponse
	if err := doJSON(client, req, &resp); err != nil {
		return nil, err
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("meta: graphql error: %s (doc_id may have rotated)", resp.Errors[0].Message)
	}

	var jobs []models.Job
	for _, j := range resp.Data.JobSearch.AllJobs {
		job := j.ToJob()
		job.Company = c.Name
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// metaFetchLSD loads the public jobs page and scrapes the LSD token out of the
// embedded server config.
func metaFetchLSD(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.metacareers.com/jobs", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", metaUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return "", &HTTPError{URL: req.URL.String(), Status: resp.StatusCode, Body: string(snippet)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	m := metaLSDRe.FindSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("meta: LSD token not found in /jobs HTML")
	}
	return string(m[1]), nil
}
