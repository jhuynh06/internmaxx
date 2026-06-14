package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"github.com/jhuynh06/internmaxx/backend/internal/models"
)

var names = [5]string{"Intern",""}

type ashBy struct {
	Jobs []models.Job `json:"jobs"`
}

func containsAny(position string) (bool) {
	for _, word := range names {
		if strings.Contains(position, word) {
			return false
		}
	}

	return true
}

// Could convert jobs to a set
func filterInternships (jobs []models.Job) ([]models.Job) {
	filteredJobs := []models.Job{}
	for _, job := range jobs {
		if  containsAny(job.Position) {
			filteredJobs = append(filteredJobs, job)
		}
	}

	return filteredJobs
}

func scrapeAshby(client *http.Client, company string) ([]models.Job, error){
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s?includeCompensation=true", company),nil)
	if err != nil {
		return nil, err
	}


	query := &ashBy{}
	err = json.NewDecoder(req.Body).Decode(query)
	if err != nil {
		return nil, err
	}

	return filterInternships(query.Jobs), nil
}
