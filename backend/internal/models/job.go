package models

import "time"

// TODO: Secondary locations for further information

type Job struct {
	AshbyJob
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
