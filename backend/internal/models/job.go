package models

import "time"

type job struct {
	Position string    `json:"position_title"`
	Date     time.Time `json:"time_scraped"`
	Link     string    `json:"link"`
	Location string    `json:"location"`
	Modality string    `json:"work_model"`
	Company  string    `json:"company"`
	Season   string    `json:"season"`
	Salary   string    `json:"salary"`
	Tags     *string   `json:"tags"`
}
