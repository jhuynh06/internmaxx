package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// DEShaw scrapes deshaw.com/careers, a Next.js site with no jobs API: the
// full posting list ships inside the page's __NEXT_DATA__ JSON, so one GET of
// the (large, ~1.8MB) HTML page yields every regular role and internship.
//
// Fragility note: this depends on Next.js keeping the __NEXT_DATA__ script tag
// and on the pageProps field names (regularJobs/internships) — a site redesign
// breaks it loudly (extract error), not silently.
type DEShaw struct{}

func (DEShaw) Source() string { return "deshaw" }

const deshawURL = "https://www.deshaw.com/careers"

type deshawNextData struct {
	Props struct {
		PageProps struct {
			RegularJobs []models.DEShawJob `json:"regularJobs"`
			Internships []models.DEShawJob `json:"internships"`
		} `json:"pageProps"`
	} `json:"props"`
}

func (DEShaw) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	body, err := getHTML(ctx, client, deshawURL)
	if err != nil {
		return nil, err
	}
	blob, err := extractNextData(body)
	if err != nil {
		return nil, fmt.Errorf("deshaw: %w", err)
	}
	var next deshawNextData
	if err := json.Unmarshal([]byte(blob), &next); err != nil {
		return nil, fmt.Errorf("deshaw: decode __NEXT_DATA__: %w", err)
	}

	pp := next.Props.PageProps
	var jobs []models.Job
	seen := map[string]bool{}
	for _, list := range [][]models.DEShawJob{pp.Internships, pp.RegularJobs} {
		for _, j := range list {
			job := j.ToJob()
			if job.ID == "" || seen[job.ID] {
				continue
			}
			seen[job.ID] = true
			job.Company = c.Name
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// extractNextData returns the JSON inside the Next.js __NEXT_DATA__ script tag.
func extractNextData(body string) (string, error) {
	const marker = `<script id="__NEXT_DATA__" type="application/json">`
	start := strings.Index(body, marker)
	if start < 0 {
		return "", fmt.Errorf("__NEXT_DATA__ script tag not found")
	}
	rest := body[start+len(marker):]
	end := strings.Index(rest, "</script>")
	if end < 0 {
		return "", fmt.Errorf("__NEXT_DATA__ script tag not terminated")
	}
	return rest[:end], nil
}
