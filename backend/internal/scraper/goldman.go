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

// Goldman scrapes Goldman Sachs' higher.gs.com via its public GraphQL gateway
// (api-higher.gs.com). It requests the CAMPUS + EARLY_CAREER experiences only:
// internships and analyst programs all live there, while PROFESSIONAL is
// thousands of experienced roles — same narrowing-at-source rationale as the
// Amazon/Workday keyword search. The filter package does the precise cut.
type Goldman struct{}

func (Goldman) Source() string { return "goldman" }

const (
	goldmanURL      = "https://api-higher.gs.com/gateway/api/v1/graphql"
	goldmanPageSize = 100
	goldmanMaxPages = 20

	// Query text mirrors higher.gs.com's own GetCampusRoles operation.
	goldmanQuery = `query GetCampusRoles($searchQueryInput: RoleSearchQueryInput!) {
  roleSearch(searchQueryInput: $searchQueryInput) {
    totalCount
    items {
      roleId
      jobTitle
      locations { primary city state country }
      status
      division
      jobType { code description }
      externalSource { sourceId }
      startDate
    }
  }
}`
)

type goldmanRequest struct {
	OperationName string           `json:"operationName"`
	Variables     goldmanVariables `json:"variables"`
	Query         string           `json:"query"`
}

type goldmanVariables struct {
	SearchQueryInput goldmanSearchInput `json:"searchQueryInput"`
}

type goldmanSearchInput struct {
	Page        goldmanPage `json:"page"`
	SearchTerm  string      `json:"searchTerm"`
	Experiences []string    `json:"experiences"`
}

type goldmanPage struct {
	PageSize   int `json:"pageSize"`
	PageNumber int `json:"pageNumber"` // 0-based
}

type goldmanResponse struct {
	Data struct {
		RoleSearch struct {
			TotalCount int                  `json:"totalCount"`
			Items      []models.GoldmanRole `json:"items"`
		} `json:"roleSearch"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (Goldman) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	var jobs []models.Job
	for page := 0; page < goldmanMaxPages; page++ {
		body, err := json.Marshal(goldmanRequest{
			OperationName: "GetCampusRoles",
			Variables: goldmanVariables{SearchQueryInput: goldmanSearchInput{
				Page:        goldmanPage{PageSize: goldmanPageSize, PageNumber: page},
				SearchTerm:  "",
				Experiences: []string{"CAMPUS", "EARLY_CAREER"},
			}},
			Query: goldmanQuery,
		})
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, goldmanURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "internmaxx/1.0")
		req.Header.Set("Origin", "https://higher.gs.com")

		var resp goldmanResponse
		if err := doJSON(client, req, &resp); err != nil {
			return nil, err
		}
		if len(resp.Errors) > 0 {
			return nil, fmt.Errorf("goldman: graphql: %s", resp.Errors[0].Message)
		}
		rs := resp.Data.RoleSearch
		for _, r := range rs.Items {
			job := r.ToJob()
			job.Company = c.Name
			jobs = append(jobs, job)
		}
		if len(rs.Items) < goldmanPageSize || len(jobs) >= rs.TotalCount {
			break
		}
	}
	return jobs, nil
}
