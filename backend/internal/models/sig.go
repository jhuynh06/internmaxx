package models

import "time"

// ── SIG / Susquehanna (custom site, Jibe/iCIMS) ─────────────────────
// GET https://careers.sig.com/api/jobs?keywords=intern&page=1&limit=100
// Each element wraps the posting under a "data" key (unwrapped by the scraper).

type SIGJob struct {
	Slug         string        `json:"slug"`   // "10942" — path segment of the public page
	ReqID        string        `json:"req_id"` // same value as slug; stable requisition id
	Title        string        `json:"title"`
	Description  string        `json:"description"`
	City         string        `json:"city"`
	State        string        `json:"state"`
	CountryCode  string        `json:"country_code"`  // "US"
	FullLocation string        `json:"full_location"` // "Richmond, Virginia"
	PostedDate   string        `json:"posted_date"`   // "2026-06-11T15:14:00+0000"
	Categories   []sigCategory `json:"categories"`    // [{name: "Interns + Co-ops"}]
}

type sigCategory struct {
	Name string `json:"name"`
}

func (s SIGJob) ToJob() Job {
	date, _ := time.Parse("2006-01-02T15:04:05-0700", s.PostedDate)
	region := s.FullLocation
	if region == "" {
		region = s.City
		if s.State != "" {
			if region != "" {
				region += ", "
			}
			region += s.State
		}
	}
	employment := ""
	if len(s.Categories) > 0 {
		employment = s.Categories[0].Name
	}
	id := s.ReqID
	if id == "" {
		id = s.Slug
	}
	link := ""
	if s.Slug != "" {
		link = "https://careers.sig.com/jobs/" + s.Slug
	}
	return Job{
		ID:             id,
		Source:         "sig",
		Position:       s.Title,
		Date:           date,
		Link:           link,
		Region:         region,
		Country:        s.CountryCode,
		EmploymentType: employment,
		Description:    s.Description,
	}
}
