package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
)

type ashBy struct {
	Jobs []models.AshbyJob `json:"jobs"`
}

func filterInternships ([]models.AshbyJob) ([]models.AshbyJob, error) {

}

func scrapeAshby(client *http.Client, company string) ([]models.AshbyJob, error){
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s?includeCompensation=true", company),nil)
	if err != nil {
		return nil, err
	}


	query := &ashBy{}
	err = json.NewDecoder(req.Body).Decode(query)
	if err != nil {
		return nil, err
	}

	return query.Jobs, nil
}
