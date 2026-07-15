package models

import (
	"encoding/json"
	"strings"
	"time"
)

// ── D. E. Shaw (custom Next.js site) ────────────────────────────────
// GET https://www.deshaw.com/careers — no separate jobs API; every posting is
// embedded in the page's __NEXT_DATA__ blob under props.pageProps.regularJobs
// and props.pageProps.internships (internalJobs is internal-only and skipped).

type DEShawJob struct {
	ID          json.Number    `json:"id"`
	DisplayName string         `json:"displayName"`
	Offices     []DEShawOffice `json:"office"`
	Categories  []string       `json:"category"`
	Data        DEShawData     `json:"data"`
}

type DEShawOffice struct {
	Name string `json:"name"` // "New York"
}

type DEShawData struct {
	JobURL         string            `json:"jobUrl"`        // "Fundamental-Research-Analyst-Intern-New-York-Summer-2027-5709"
	ValidFromDate  string            `json:"validFromDate"` // "2025-12-17"
	JobDescription DEShawDescription `json:"jobDescription"`
}

type DEShawDescription struct {
	WebsiteDescription string `json:"websiteDescription"`
}

func (d DEShawJob) ToJob() Job {
	var offices []string
	for _, o := range d.Offices {
		if o.Name != "" {
			offices = append(offices, o.Name)
		}
	}
	date, _ := time.Parse("2006-01-02", d.Data.ValidFromDate)
	link := ""
	if d.Data.JobURL != "" {
		// The site links postings as the lowercased jobUrl slug (id suffix included).
		link = "https://www.deshaw.com/careers/" + strings.ToLower(d.Data.JobURL)
	}
	return Job{
		ID:          d.ID.String(),
		Source:      "deshaw",
		Position:    d.DisplayName,
		Date:        date,
		Link:        link,
		Region:      strings.Join(offices, " / "),
		Description: d.Data.JobDescription.WebsiteDescription,
	}
}
