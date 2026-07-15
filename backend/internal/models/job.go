package models

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Job is the canonical, source-independent representation of a posting.
// Every ATS/aggregator raw type converts into this via a ToJob() method.
type Job struct {
	ID             string    // ATS-native posting id; stable component of the dedup key
	Company        string    // display name (set by the scraper from the registry entry)
	Source         string    // "ashby", "greenhouse", "lever", "smartrecruiters", "simplify"
	Position       string    // job title
	Date           time.Time // best available posted/published time
	Link           string
	Region         string
	Country        string
	Modality       string // on-site / remote / hybrid, when known
	EmploymentType string // e.g. "Intern", "FullTime" — ATS-native when available
	Description    string
	Category       string // set by the filter package (e.g. "Software", "ML/AI")
}

// Application tracks a user's progress on a specific posting. JobKey matches a
// seen_jobs key (or any stable id the user supplies).
type Application struct {
	JobKey    string `json:"job_key"`
	Company   string `json:"company"`
	Title     string `json:"title"`
	URL       string `json:"url,omitempty"`
	Status    string `json:"status"`
	Notes     string `json:"notes,omitempty"`
	AppliedAt string `json:"applied_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// ApplicationStatuses is the allowed status pipeline, in order.
var ApplicationStatuses = []string{"saved", "applied", "oa", "phone", "onsite", "offer", "rejected", "ghosted"}

// ValidStatus reports whether s is an allowed application status.
func ValidStatus(s string) bool {
	for _, v := range ApplicationStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// Key uniquely identifies a posting for dedup. Prefers the ATS id and falls
// back to the URL (title alone is never unique — same title, many reqs).
func (j Job) Key() string {
	id := j.ID
	if id == "" {
		id = j.Link
	}
	return j.Source + "/" + j.Company + "/" + id
}

// ── Ashby ───────────────────────────────────────────────────────────
// GET https://api.ashbyhq.com/posting-api/job-board/{slug}?includeCompensation=true

type AshbyJob struct {
	ID             string       `json:"id"`
	Position       string       `json:"title"`
	Date           time.Time    `json:"publishedAt"`
	Link           string       `json:"jobUrl"`
	Location       string       `json:"location"` // city, top-level string
	Address        ashbyAddress `json:"address"`
	Modality       string       `json:"workplaceType"` // On-site / Remote / Hybrid
	EmploymentType string       `json:"employmentType"`
	Description    string       `json:"descriptionHtml"`
}

type ashbyAddress struct {
	Postal ashbyPostal `json:"postalAddress"`
}

type ashbyPostal struct {
	Region   string `json:"addressRegion"`
	Country  string `json:"addressCountry"`
	Locality string `json:"addressLocality"`
}

func (a AshbyJob) ToJob() Job {
	region := a.Location
	if region == "" {
		region = a.Address.Postal.Region
	}
	return Job{
		ID:             a.ID,
		Source:         "ashby",
		Position:       a.Position,
		Date:           a.Date,
		Link:           a.Link,
		Region:         region,
		Country:        a.Address.Postal.Country,
		Modality:       a.Modality,
		EmploymentType: a.EmploymentType,
		Description:    a.Description,
	}
}

// ── Greenhouse ──────────────────────────────────────────────────────
// GET https://boards-api.greenhouse.io/v1/boards/{slug}/jobs?content=true

type GreenhouseJob struct {
	ID             int64      `json:"id"`
	Position       string     `json:"title"`
	UpdatedAt      time.Time  `json:"updated_at"`
	FirstPublished time.Time  `json:"first_published"`
	Link           string     `json:"absolute_url"`
	Location       ghLocation `json:"location"`
	Description    string     `json:"content"`
}

type ghLocation struct {
	Name string `json:"name"`
}

func (g GreenhouseJob) ToJob() Job {
	// first_published is the real posted date; updated_at moves on any edit.
	date := g.FirstPublished
	if date.IsZero() {
		date = g.UpdatedAt
	}
	return Job{
		ID:          strconv.FormatInt(g.ID, 10),
		Source:      "greenhouse",
		Position:    g.Position,
		Date:        date,
		Link:        g.Link,
		Region:      g.Location.Name,
		Description: g.Description,
	}
}

// ── Lever ───────────────────────────────────────────────────────────
// GET https://api.lever.co/v0/postings/{slug}?mode=json

type LeverJob struct {
	ID          string        `json:"id"`
	Position    string        `json:"text"`
	CreatedAt   int64         `json:"createdAt"` // epoch milliseconds
	Link        string        `json:"hostedUrl"`
	Country     string        `json:"country"`
	Workplace   string        `json:"workplaceType"` // on-site / remote / hybrid
	Categories  leverCategory `json:"categories"`
	Description string        `json:"descriptionPlain"`
}

type leverCategory struct {
	Location   string `json:"location"`
	Commitment string `json:"commitment"` // "Intern", "Full-time", ...
	Team       string `json:"team"`
}

func (l LeverJob) ToJob() Job {
	var date time.Time
	if l.CreatedAt > 0 {
		date = time.UnixMilli(l.CreatedAt).UTC()
	}
	return Job{
		ID:             l.ID,
		Source:         "lever",
		Position:       l.Position,
		Date:           date,
		Link:           l.Link,
		Region:         l.Categories.Location,
		Country:        l.Country,
		Modality:       l.Workplace,
		EmploymentType: l.Categories.Commitment,
		Description:    l.Description,
	}
}

// ── SmartRecruiters ─────────────────────────────────────────────────
// GET https://api.smartrecruiters.com/v1/companies/{slug}/postings?limit=100&offset=0

type SmartRecruitersJob struct {
	ID           string       `json:"id"`
	Position     string       `json:"name"`
	ReleasedDate time.Time    `json:"releasedDate"`
	Location     srLocation   `json:"location"`
	Employment   srEmployment `json:"typeOfEmployment"`
	Ref          string       `json:"ref"` // API url; public apply url is derived below
	Company      srCompany    `json:"company"`
}

type srLocation struct {
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
	Remote  bool   `json:"remote"`
	Hybrid  bool   `json:"hybrid"`
}

type srEmployment struct {
	Label string `json:"label"`
}

type srCompany struct {
	Identifier string `json:"identifier"`
	Name       string `json:"name"`
}

func (s SmartRecruitersJob) ToJob() Job {
	region := s.Location.City
	if s.Location.Region != "" {
		if region != "" {
			region += ", "
		}
		region += s.Location.Region
	}
	modality := ""
	switch {
	case s.Location.Remote:
		modality = "Remote"
	case s.Location.Hybrid:
		modality = "Hybrid"
	}
	link := ""
	if s.Company.Identifier != "" {
		link = "https://jobs.smartrecruiters.com/" + s.Company.Identifier + "/" + s.ID
	}
	return Job{
		ID:             s.ID,
		Source:         "smartrecruiters",
		Position:       s.Position,
		Date:           s.ReleasedDate,
		Link:           link,
		Region:         region,
		Country:        s.Location.Country,
		Modality:       modality,
		EmploymentType: s.Employment.Label,
	}
}

// ── Workday ─────────────────────────────────────────────────────────
// POST https://{tenant}.{instance}.myworkdayjobs.com/wday/cxs/{tenant}/{site}/jobs
// body {"appliedFacets":{},"limit":20,"offset":0,"searchText":"internship"}

type WorkdayJob struct {
	Title         string   `json:"title"`
	ExternalPath  string   `json:"externalPath"`  // e.g. /job/City/Title_JR123
	LocationsText string   `json:"locationsText"` // "Santa Clara, CA" or "5 Locations"
	PostedOn      string   `json:"postedOn"`      // relative: "Posted Today", "Posted 30+ Days Ago"
	BulletFields  []string `json:"bulletFields"`  // usually [reqId]
}

// ToJob converts a Workday posting. host is "{tenant}.{instance}.myworkdayjobs.com"
// and site is the career-site path segment, both needed to build the public URL
// and a stable id (Workday responses carry neither absolute).
func (w WorkdayJob) ToJob(host, site string) Job {
	id := w.ExternalPath
	if len(w.BulletFields) > 0 && w.BulletFields[0] != "" {
		id = w.BulletFields[0] // req id (e.g. JR2016444) — stable across re-list
	}
	link := ""
	if w.ExternalPath != "" {
		link = "https://" + host + "/en-US/" + site + w.ExternalPath
	}
	return Job{
		ID:       id,
		Source:   "workday",
		Position: w.Title,
		Date:     parseWorkdayPostedOn(w.PostedOn),
		Link:     link,
		Region:   w.LocationsText,
	}
}

// parseWorkdayPostedOn best-efforts the relative "Posted ..." string. Anything
// older/ambiguous returns zero (dedup keys on the req id, so date is cosmetic).
func parseWorkdayPostedOn(s string) time.Time {
	l := strings.ToLower(s)
	now := time.Now().UTC()
	switch {
	case strings.Contains(l, "today"):
		return now
	case strings.Contains(l, "yesterday"):
		return now.AddDate(0, 0, -1)
	}
	if m := workdayDaysAgoRe.FindStringSubmatch(l); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return now.AddDate(0, 0, -n)
		}
	}
	return time.Time{}
}

var workdayDaysAgoRe = regexp.MustCompile(`posted\s+(\d+)\+?\s+days?\s+ago`)

// ── Amazon (custom site) ────────────────────────────────────────────
// GET https://www.amazon.jobs/en/search.json?base_query=intern&normalized_country_code[]=USA

type AmazonJob struct {
	Title       string `json:"title"`
	City        string `json:"city"`
	State       string `json:"state"`
	CountryCode string `json:"country_code"`
	PostedDate  string `json:"posted_date"` // "May 13, 2026"
	JobPath     string `json:"job_path"`
	IDIcims     string `json:"id_icims"`
}

func (a AmazonJob) ToJob() Job {
	region := a.City
	if a.State != "" {
		if region != "" {
			region += ", "
		}
		region += a.State
	}
	date, _ := time.Parse("Jan 2, 2006", a.PostedDate)
	link := ""
	if a.JobPath != "" {
		link = "https://www.amazon.jobs" + a.JobPath
	}
	return Job{
		ID:       a.IDIcims,
		Source:   "amazon",
		Position: a.Title,
		Date:     date,
		Link:     link,
		Region:   region,
		Country:  a.CountryCode,
	}
}

// ── Netflix (custom site, Eightfold) ────────────────────────────────
// GET https://explore.jobs.netflix.net/api/apply/v2/jobs?domain=netflix.com&query=intern

type NetflixJob struct {
	ID       json.Number `json:"id"`
	Name     string      `json:"name"`
	Location string      `json:"location"` // "Los Gatos,California,United States of America"
	URL      string      `json:"canonicalPositionUrl"`
}

func (n NetflixJob) ToJob() Job {
	return Job{
		ID:       n.ID.String(),
		Source:   "netflix",
		Position: n.Name,
		Link:     n.URL,
		Region:   strings.ReplaceAll(n.Location, ",", ", "),
	}
}

// ── Oracle Recruiting Cloud (custom, generic across tenants) ─────────
// GET https://{tenant}.fa.oraclecloud.com/hcmRestApi/resources/latest/recruitingCEJobRequisitions
//     ?onlyData=true&finder=findReqs;siteNumber={site},keyword=internship
// One scraper covers every ORC bank (JPMorgan, etc.) via per-company {tenant, site}.

type ORCJob struct {
	ID              json.Number `json:"Id"`
	Title           string      `json:"Title"`
	PrimaryLocation string      `json:"PrimaryLocation"` // "New York, NY, United States"
	Country         string      `json:"PrimaryLocationCountry"`
	PostedDate      string      `json:"PostedDate"` // "2025-12-29"
	WorkplaceType   string      `json:"WorkplaceType"`
}

func (o ORCJob) ToJob(tenant, site string) Job {
	date, _ := time.Parse("2006-01-02", o.PostedDate)
	link := ""
	if o.ID.String() != "" {
		link = "https://" + tenant + ".fa.oraclecloud.com/hcmUI/CandidateExperience/en/sites/" + site + "/job/" + o.ID.String()
	}
	return Job{
		ID:       o.ID.String(),
		Source:   "oracle",
		Position: o.Title,
		Date:     date,
		Link:     link,
		Region:   o.PrimaryLocation,
		Country:  o.Country,
		Modality: o.WorkplaceType,
	}
}

// ── Simplify aggregator ─────────────────────────────────────────────
// raw.githubusercontent.com/SimplifyJobs/Summer2026-Internships/dev/.github/scripts/listings.json

type SimplifyJob struct {
	ID          string   `json:"id"`
	CompanyName string   `json:"company_name"`
	Position    string   `json:"title"`
	Locations   []string `json:"locations"`
	Link        string   `json:"url"`
	DatePosted  int64    `json:"date_posted"` // epoch seconds
	Active      bool     `json:"active"`
	IsVisible   bool     `json:"is_visible"`
	Category    string   `json:"category"`
	Sponsorship string   `json:"sponsorship"`
	Terms       []string `json:"terms"`
	Src         string   `json:"source"` // Simplify's own provenance tag
}

func (s SimplifyJob) ToJob() Job {
	var date time.Time
	if s.DatePosted > 0 {
		date = time.Unix(s.DatePosted, 0).UTC()
	}
	return Job{
		ID:       s.ID,
		Company:  s.CompanyName,
		Source:   "simplify",
		Position: s.Position,
		Date:     date,
		Link:     s.Link,
		Region:   strings.Join(s.Locations, " / "),
		Category: s.Category, // Simplify's own categorization; may be refined by filter
	}
}
