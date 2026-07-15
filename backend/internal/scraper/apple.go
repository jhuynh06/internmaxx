package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// Apple scrapes jobs.apple.com. The old /api/v1/search endpoint (CSRF-token
// dance) is gone; today the search page embeds the full result state in the
// HTML as window.__staticRouterHydrationData, which this scraper parses.
// team=internships-STDNT-INTRN narrows to the Internships team at the source
// (free-text search=intern fuzzy-matches ~1900 unrelated postings).
type Apple struct{}

func (Apple) Source() string { return "apple" }

const (
	applePageSize = 20 // fixed by the site
	appleMaxPages = 30 // safety bound (=600 postings)
	appleTeam     = "internships-STDNT-INTRN"
)

// appleHydration mirrors the slice of __staticRouterHydrationData we need.
type appleHydration struct {
	LoaderData struct {
		Search struct {
			SearchResults []models.AppleJob `json:"searchResults"`
			TotalRecords  int               `json:"totalRecords"`
		} `json:"search"`
	} `json:"loaderData"`
}

func (Apple) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	var jobs []models.Job
	for page := 1; page <= appleMaxPages; page++ {
		u := fmt.Sprintf("https://jobs.apple.com/en-us/search?team=%s&page=%d", appleTeam, page)
		body, err := appleFetchPage(ctx, client, u)
		if err != nil {
			return nil, err
		}
		var hyd appleHydration
		if err := appleExtractHydration(body, &hyd); err != nil {
			return nil, fmt.Errorf("apple: %s: %w", u, err)
		}
		s := hyd.LoaderData.Search
		for _, j := range s.SearchResults {
			job := j.ToJob()
			job.Company = c.Name
			jobs = append(jobs, job)
		}
		if len(s.SearchResults) < applePageSize || len(jobs) >= s.TotalRecords {
			break
		}
	}
	return jobs, nil
}

func appleFetchPage(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, &HTTPError{URL: url, Status: resp.StatusCode, Body: string(snippet)}
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20))
}

// appleExtractHydration decodes the `window.__staticRouterHydrationData =
// JSON.parse("...")` script. The argument is a JSON string literal containing
// JSON, so it is unmarshalled twice. Any raw `"` inside the literal is escaped,
// which makes `");` a safe end marker.
func appleExtractHydration(body []byte, dst any) error {
	const marker = `window.__staticRouterHydrationData = JSON.parse(`
	start := bytes.Index(body, []byte(marker))
	if start < 0 {
		return fmt.Errorf("hydration data not found")
	}
	rest := body[start+len(marker):]
	end := bytes.Index(rest, []byte(`");`))
	if end < 0 {
		return fmt.Errorf("hydration end marker not found")
	}
	var inner string
	if err := json.Unmarshal(rest[:end+1], &inner); err != nil {
		return fmt.Errorf("decode hydration literal: %w", err)
	}
	if err := json.Unmarshal([]byte(inner), dst); err != nil {
		return fmt.Errorf("decode hydration json: %w", err)
	}
	return nil
}
