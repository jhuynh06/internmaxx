package models

import "strings"

// ── Meta (custom site, GraphQL) ─────────────────────────────────────
// POST https://www.metacareers.com/graphql (doc_id persisted query
// CareersJobSearchResultsDataQuery, lsd token scraped from /jobs).
// The API exposes no posting date; dedup keys on the job id.

type MetaJob struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Locations []string `json:"locations"` // "Menlo Park, CA", "London, UK"
	Teams     []string `json:"teams"`     // "AI Research", "Internship - PhD", ...
	SubTeams  []string `json:"sub_teams"`
}

func (m MetaJob) ToJob() Job {
	link := ""
	if m.ID != "" {
		link = "https://www.metacareers.com/jobs/" + m.ID
	}
	return Job{
		ID:       m.ID,
		Source:   "meta",
		Position: m.Title,
		Link:     link,
		Region:   strings.Join(m.Locations, " / "),
	}
}
