package models

import "time"

// Currently not handling dedup, a position could have same title

type Job struct {
	Position    string
	Date        time.Time
	Link        string
	Region      string
	Country     string
	Modality    string
	Description string
}

type AshbyJob struct {
	Position       string    `json:"title"`
	Date           time.Time `json:"publishedAt"`
	Link           string    `json:"jobURL"`
	Region         string    `json:"addressRegion"`
	Country        string    `json:"addressCountry"`
	Modality       string    `json:"workplaceType"`
	EmploymentType string    `json:"employmentType"`
	Description    string    `json:"descriptionHtml"`
}

func (a AshbyJob) ToJob() Job {
	return Job{
		Position:    a.Position,
		Date:        a.Date,
		Link:        a.Link,
		Region:      a.Region,
		Country:     a.Country,
		Modality:    a.Modality,
		Description: a.Description,
	}
}

type GreenhouseJob struct {
	Position    string    `json:"title"`
	Date        time.Time `json:"updated_at"`
	Link        string    `json:"absolute_url"`
	Region      location  `json:"location"`
	Description string    `json:"content"`
}

type location struct {
	Name string `json:"name"`
}

func (g GreenhouseJob) ToJob() Job {
	return Job{
		Position:    g.Position,
		Date:        g.Date,
		Link:        g.Link,
		Region:      g.Region.Name,
		Description: g.Description,
	}
}
