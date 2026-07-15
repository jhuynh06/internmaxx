package models

import (
	"encoding/json"
	"time"
)

// ── Uber (custom site) ──────────────────────────────────────────────
// POST https://www.uber.com/api/loadSearchJobsResults?localeCode=en
// body {"params":{"text":"intern"},"limit":100,"page":0}   (page is 0-based)

type UberJob struct {
	ID           json.Number  `json:"id"` // numeric, e.g. 158513
	Title        string       `json:"title"`
	Description  string       `json:"description"`
	Location     uberLocation `json:"location"`
	CreationDate time.Time    `json:"creationDate"` // "2026-06-19T15:12:02.000Z"
	TimeType     string       `json:"timeType"`     // usually empty
}

type uberLocation struct {
	City        string `json:"city"`
	Region      string `json:"region"`
	Country     string `json:"country"`     // ISO-3, e.g. "MEX"
	CountryName string `json:"countryName"` // "Mexico"
}

func (u UberJob) ToJob() Job {
	region := u.Location.City
	if u.Location.Region != "" {
		if region != "" {
			region += ", "
		}
		region += u.Location.Region
	}
	link := ""
	if u.ID.String() != "" {
		// Redirects to jobs.uber.com/en/jobs/{id}/ but this is the public form.
		link = "https://www.uber.com/global/en/careers/list/" + u.ID.String() + "/"
	}
	return Job{
		ID:             u.ID.String(),
		Source:         "uber",
		Position:       u.Title,
		Date:           u.CreationDate,
		Link:           link,
		Region:         region,
		Country:        u.Location.CountryName,
		EmploymentType: u.TimeType,
		Description:    u.Description,
	}
}
