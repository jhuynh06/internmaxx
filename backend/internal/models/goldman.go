package models

import (
	"strings"
	"time"
)

// ── Goldman Sachs (higher.gs.com, custom GraphQL) ───────────────────
// POST https://api-higher.gs.com/gateway/api/v1/graphql
// query GetCampusRoles { roleSearch(searchQueryInput: {...}) { totalCount items {...} } }
// Public (no auth); the query text is lifted from higher.gs.com's JS bundles.

type GoldmanRole struct {
	RoleID         string            `json:"roleId"`   // "170775_GS_CAMPUS" — stable
	JobTitle       string            `json:"jobTitle"` // "2027 | Americas | ... | Summer Analyst"
	Locations      []GoldmanLocation `json:"locations"`
	Status         string            `json:"status"` // "POSTED"
	Division       string            `json:"division"`
	JobType        *GoldmanJobType   `json:"jobType"`
	ExternalSource GoldmanSource     `json:"externalSource"`
	StartDate      time.Time         `json:"startDate"` // posting go-live timestamp
}

type GoldmanLocation struct {
	Primary bool   `json:"primary"`
	City    string `json:"city"`
	State   string `json:"state"`
	Country string `json:"country"`
}

type GoldmanJobType struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type GoldmanSource struct {
	SourceID string `json:"sourceId"` // numeric id used in the public role URL
}

func (g GoldmanRole) ToJob() Job {
	loc := GoldmanLocation{}
	if len(g.Locations) > 0 {
		loc = g.Locations[0]
		for _, l := range g.Locations {
			if l.Primary {
				loc = l
				break
			}
		}
	}
	region := loc.City
	if loc.State != "" && loc.State != loc.City {
		if region != "" {
			region += ", "
		}
		region += loc.State
	}
	link := ""
	if g.ExternalSource.SourceID != "" {
		link = "https://higher.gs.com/roles/" + g.ExternalSource.SourceID
	}
	employmentType := ""
	if g.JobType != nil {
		employmentType = g.JobType.Description
	}
	return Job{
		ID:             g.RoleID,
		Source:         "goldman",
		Position:       strings.TrimSpace(g.JobTitle),
		Date:           g.StartDate,
		Link:           link,
		Region:         region,
		Country:        loc.Country,
		EmploymentType: employmentType,
		Description:    g.Division,
	}
}
