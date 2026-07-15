package models

import (
	"strings"
	"time"
)

// ── Apple (custom site, server-rendered) ────────────────────────────
// GET https://jobs.apple.com/en-us/search?team=internships-STDNT-INTRN&page=1
// jobs.apple.com embeds the full search state in the HTML as
// window.__staticRouterHydrationData; searchResults carries these postings.

type AppleJob struct {
	PositionID              string          `json:"positionId"` // "200313970", used in the public URL
	ReqID                   string          `json:"reqId"`      // "PIPE-200313970"
	PostingTitle            string          `json:"postingTitle"`
	TransformedPostingTitle string          `json:"transformedPostingTitle"` // URL slug
	PostDateInGMT           time.Time       `json:"postDateInGMT"`
	Locations               []appleLocation `json:"locations"`
	Team                    appleTeam       `json:"team"`
	HomeOffice              bool            `json:"homeOffice"`
}

type appleLocation struct {
	Name        string `json:"name"` // "Cupertino", "India", ...
	CountryName string `json:"countryName"`
}

type appleTeam struct {
	TeamName string `json:"teamName"`
}

func (a AppleJob) ToJob() Job {
	var names []string
	country := ""
	for _, l := range a.Locations {
		if l.Name != "" {
			names = append(names, l.Name)
		}
		if country == "" {
			country = l.CountryName
		}
	}
	id := a.ReqID
	if id == "" {
		id = a.PositionID
	}
	link := ""
	if a.PositionID != "" {
		link = "https://jobs.apple.com/en-us/details/" + a.PositionID
		if a.TransformedPostingTitle != "" {
			link += "/" + a.TransformedPostingTitle
		}
	}
	modality := ""
	if a.HomeOffice {
		modality = "Remote"
	}
	return Job{
		ID:       id,
		Source:   "apple",
		Position: a.PostingTitle,
		Date:     a.PostDateInGMT,
		Link:     link,
		Region:   strings.Join(names, " / "),
		Country:  country,
		Modality: modality,
	}
}
