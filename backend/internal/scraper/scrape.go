package scraper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"regexp"
	"github.com/jhuynh06/internmaxx/backend/internal/models"
)

var names = []string{
	"intern",
	"coop",
	"co-op",
	"co op",
	"apprentice",
	"trainee",
	"student",
}

type ashbyResponse struct {
	Jobs []models.AshbyJob `json:"jobs"`
}

type greenhouseResponse struct {
	Jobs []models.GreenhouseJob `json:"jobs"`
}

var internRe = func() *regexp.Regexp {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = regexp.QuoteMeta(n)
	}
	return regexp.MustCompile(`(?i)\b(` + strings.Join(quoted, "|") + `)s?\b`)
}()

func containsAny(position string) bool {
	return internRe.MatchString(position)
}

func filterInternships(jobs []models.Job) []models.Job {
	filtered := []models.Job{}
	for _, job := range jobs {
		if containsAny(job.Position) {
			filtered = append(filtered, job)
		}
	}
	return filtered
}

func ScrapeAshby(client *http.Client, company string) ([]models.Job, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s?includeCompensation=true", company), nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ashby: unexpected status %d for company %s", resp.StatusCode, company)
	}

	var raw ashbyResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	jobs := make([]models.Job, len(raw.Jobs))
	for i, j := range raw.Jobs {
		jobs[i] = j.ToJob()
	}

	return filterInternships(jobs), nil
}

func ScrapeGreenhouse(client *http.Client, company string) ([]models.Job, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs?content=true", company), nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("greenhouse: unexpected status %d for company %s", resp.StatusCode, company)
	}

	var raw greenhouseResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	jobs := make([]models.Job, len(raw.Jobs))
	for i, j := range raw.Jobs {
		jobs[i] = j.ToJob()
	}

	return filterInternships(jobs), nil
}
