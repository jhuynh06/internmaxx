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

// Google scrapes the server-rendered careers search at
// google.com/about/careers/applications. There is no public JSON API; the
// results page embeds the job list as an AF_initDataCallback payload
// (key 'ds:1') whose data is plain JSON: [jobs, null, totalCount, pageSize].
// employment_type=INTERN narrows at the source (ATS-native employment type,
// same count as q=intern when probed).
type Google struct{}

func (Google) Source() string { return "google" }

const (
	googlePageSize = 20 // fixed by the site
	googleMaxPages = 30 // safety bound (=600 postings)
)

func (Google) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	var jobs []models.Job
	for page := 1; page <= googleMaxPages; page++ {
		u := fmt.Sprintf(
			"https://www.google.com/about/careers/applications/jobs/results?employment_type=INTERN&page=%d",
			page)
		body, err := googleFetchPage(ctx, client, u)
		if err != nil {
			return nil, err
		}
		raw, err := googleExtractDS1(body)
		if err != nil {
			return nil, fmt.Errorf("google: %s: %w", u, err)
		}
		var payload []json.RawMessage // [jobs, null, totalCount, pageSize]
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, fmt.Errorf("google: decode ds:1 payload: %w", err)
		}
		if len(payload) < 3 {
			return nil, fmt.Errorf("google: ds:1 payload has %d fields, want >=3", len(payload))
		}
		var pageJobs []models.GoogleJob
		if err := json.Unmarshal(payload[0], &pageJobs); err != nil {
			return nil, fmt.Errorf("google: decode jobs: %w", err)
		}
		var total int
		_ = json.Unmarshal(payload[2], &total)

		for _, j := range pageJobs {
			job := j.ToJob()
			job.Company = c.Name
			jobs = append(jobs, job)
		}
		if len(pageJobs) < googlePageSize || len(jobs) >= total {
			break
		}
	}
	return jobs, nil
}

// googleFetchPage GETs an HTML page with browser-like headers (the careers
// frontend serves any UA today, but a real UA keeps us on the common path).
func googleFetchPage(ctx context.Context, client *http.Client, url string) ([]byte, error) {
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

// googleExtractDS1 pulls the JSON data out of the ds:1 AF_initDataCallback
// block. The framing (`data:` ... `, sideChannel:`) is stable across Google's
// server-rendered apps but is not a public contract — if this errors, re-derive
// the markers from a fresh page.
func googleExtractDS1(body []byte) ([]byte, error) {
	start := bytes.Index(body, []byte("AF_initDataCallback({key: 'ds:1'"))
	if start < 0 {
		return nil, fmt.Errorf("ds:1 block not found")
	}
	rest := body[start:]
	dataIdx := bytes.Index(rest, []byte("data:"))
	if dataIdx < 0 {
		return nil, fmt.Errorf("ds:1 data marker not found")
	}
	rest = rest[dataIdx+len("data:"):]
	end := bytes.Index(rest, []byte(", sideChannel:"))
	if end < 0 {
		return nil, fmt.Errorf("ds:1 end marker not found")
	}
	return rest[:end], nil
}
