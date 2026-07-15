package models

import "strings"

// ── Rippling ATS ────────────────────────────────────────────────────
// GET https://api.rippling.com/platform/api/ats/v1/board/{slug}/jobs
// Returns a bare JSON array; the board exposes no dates or employment type.

type RipplingJob struct {
	UUID         string        `json:"uuid"`
	Name         string        `json:"name"`
	Link         string        `json:"url"` // https://ats.rippling.com/{board}/jobs/{uuid}
	Department   ripplingLabel `json:"department"`
	WorkLocation ripplingLabel `json:"workLocation"` // label e.g. "Remote (New Jersey, US)"
}

type ripplingLabel struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func (r RipplingJob) ToJob() Job {
	// The board has no explicit modality field; remote roles are flagged in the
	// work-location label ("Remote (New Jersey, US)").
	modality := ""
	if strings.HasPrefix(r.WorkLocation.Label, "Remote") {
		modality = "Remote"
	}
	return Job{
		ID:       r.UUID,
		Source:   "rippling",
		Position: r.Name,
		Link:     r.Link,
		Region:   r.WorkLocation.Label,
		Modality: modality,
	}
}
