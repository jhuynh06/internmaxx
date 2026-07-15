package models

import (
	"encoding/json"
	"strings"
	"time"
)

// ── Microsoft (custom site, Eightfold PCSX) ─────────────────────────
// GET https://apply.careers.microsoft.com/api/pcsx/search?domain=microsoft.com&query=intern&start=0
// Microsoft migrated jobs.careers.microsoft.com to Eightfold's newer "PCSX"
// candidate portal; the classic /api/apply/v2/jobs endpoint 403s for this
// tenant, but /api/pcsx/search is public JSON.

type MicrosoftJob struct {
	ID           json.Number `json:"id"`           // Eightfold position id (used in the public URL)
	DisplayJobID string      `json:"displayJobId"` // ATS req id, e.g. "200038382" — stable across re-list
	Name         string      `json:"name"`
	Locations    []string    `json:"locations"` // "Redmond, Washington, United States"
	PostedTs     int64       `json:"postedTs"`  // epoch seconds
	WorkOption   string      `json:"workLocationOption"`
}

func (m MicrosoftJob) ToJob() Job {
	var date time.Time
	if m.PostedTs > 0 {
		date = time.Unix(m.PostedTs, 0).UTC()
	}
	id := m.DisplayJobID
	if id == "" {
		id = m.ID.String()
	}
	link := ""
	if m.ID.String() != "" {
		link = "https://jobs.careers.microsoft.com/careers/job/" + m.ID.String()
	}
	return Job{
		ID:       id,
		Source:   "microsoft",
		Position: m.Name,
		Date:     date,
		Link:     link,
		Region:   strings.Join(m.Locations, " / "),
		Modality: m.WorkOption, // "onsite" / "hybrid" / "remote"
	}
}
