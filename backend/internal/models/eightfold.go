package models

import (
	"encoding/json"
	"strings"
)

// ── Eightfold (generic, per-company {host, domain, query}) ──────────
// GET https://{host}/api/apply/v2/jobs?domain={domain}&query={query}
// Same payload as the Netflix scraper (Netflix predates this generic source and
// keeps its own source key so existing dedup keys stay stable).

type EightfoldJob struct {
	ID       json.Number `json:"id"`
	Name     string      `json:"name"`
	Location string      `json:"location"` // "New York,New York,United States of America"
	URL      string      `json:"canonicalPositionUrl"`
}

func (e EightfoldJob) ToJob() Job {
	return Job{
		ID:       e.ID.String(),
		Source:   "eightfold",
		Position: e.Name,
		Link:     e.URL,
		Region:   strings.ReplaceAll(e.Location, ",", ", "),
	}
}
