package scraper

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

type Greenhouse struct{}

func (Greenhouse) Source() string { return "greenhouse" }

type greenhouseResponse struct {
	Jobs []models.GreenhouseJob `json:"jobs"`
}

func (Greenhouse) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	url := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs?content=true", c.Slug)
	var raw greenhouseResponse
	if err := getJSON(ctx, client, url, &raw); err != nil {
		return nil, err
	}
	jobs := make([]models.Job, 0, len(raw.Jobs))
	for _, j := range raw.Jobs {
		job := j.ToJob()
		job.Company = c.Name
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// BoardName fetches the board's declared name — used by cmd/verify to catch
// slug collisions (greenhouse "figure" is Figure Lending, not the robotics co).
func (Greenhouse) BoardName(ctx context.Context, client *http.Client, slug string) (string, error) {
	url := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s", slug)
	var meta struct {
		Name string `json:"name"`
	}
	if err := getJSON(ctx, client, url, &meta); err != nil {
		return "", err
	}
	return meta.Name, nil
}
