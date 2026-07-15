package models

import "time"

// ── Shopify (custom site, Ashby-backed) ─────────────────────────────
// GET https://www.shopify.com/careers.data — the React Router "single fetch"
// payload for shopify.com/careers, a turbo-stream flat array whose loader data
// carries jobPostingsWithJobs (every listed posting). Shopify runs Ashby
// internally (apply links are /careers?ashby_jid={id}) but its public Ashby
// board is disabled (api.ashbyhq.com/posting-api/job-board/shopify → 404),
// so this payload is the only plain-HTTP source.

type ShopifyJob struct {
	ID             string `json:"id"` // Ashby job-posting uuid — stable
	Title          string `json:"title"`
	TeamName       string `json:"teamName"`
	LocationName   string `json:"locationName"` // "Americas", "Canada", ...
	WorkplaceType  string `json:"workplaceType"`
	EmploymentType string `json:"employmentType"` // "FullTime", "Intern", ...
	PublishedDate  string `json:"publishedDate"`  // "2026-05-22"
	ExternalLink   string `json:"externalLink"`   // https://www.shopify.com/careers?ashby_jid={id}
}

func (s ShopifyJob) ToJob() Job {
	date, _ := time.Parse("2006-01-02", s.PublishedDate)
	return Job{
		ID:             s.ID,
		Source:         "shopify",
		Position:       s.Title,
		Date:           date,
		Link:           s.ExternalLink,
		Region:         s.LocationName,
		Modality:       s.WorkplaceType,
		EmploymentType: s.EmploymentType,
		Description:    s.TeamName,
	}
}
