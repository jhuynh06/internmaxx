package models

import (
	"encoding/json"
	"time"
)

// ── Google (custom site, server-rendered) ───────────────────────────
// GET https://www.google.com/about/careers/applications/jobs/results?employment_type=INTERN&page=1
// Google embeds the search results in the HTML as an AF_initDataCallback
// payload (key 'ds:1'). Each job is a positional JSON array, so GoogleJob
// unmarshals by index: 0=id, 1=title, 9=locations, 12=create time.

type GoogleJob struct {
	ID        string
	Position  string
	Locations []string // display names, e.g. "Mountain View, CA, USA"
	Country   string   // ISO code of the first location
	Date      time.Time
}

func (g *GoogleJob) UnmarshalJSON(b []byte) error {
	var f []json.RawMessage
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	// Best-effort per index: absent/null/mistyped fields stay zero rather than
	// failing the whole page (the payload is positional and undocumented).
	get := func(i int, dst any) {
		if i < len(f) && string(f[i]) != "null" {
			_ = json.Unmarshal(f[i], dst)
		}
	}
	get(0, &g.ID)
	get(1, &g.Position)

	// Index 9: list of locations, each itself positional:
	// [display, [addresses], city, zip, state, countryCode]
	var locs [][]json.RawMessage
	get(9, &locs)
	for _, l := range locs {
		var name string
		if len(l) > 0 {
			_ = json.Unmarshal(l[0], &name)
		}
		if name != "" {
			g.Locations = append(g.Locations, name)
		}
		if g.Country == "" && len(l) > 5 {
			_ = json.Unmarshal(l[5], &g.Country)
		}
	}

	// Index 12: creation timestamp as [epochSeconds, nanos].
	var ts []int64
	get(12, &ts)
	if len(ts) > 0 && ts[0] > 0 {
		g.Date = time.Unix(ts[0], 0).UTC()
	}
	return nil
}

func (g GoogleJob) ToJob() Job {
	link := ""
	if g.ID != "" {
		link = "https://www.google.com/about/careers/applications/jobs/results/" + g.ID
	}
	region := ""
	if len(g.Locations) > 0 {
		region = g.Locations[0]
		for _, l := range g.Locations[1:] {
			region += " / " + l
		}
	}
	return Job{
		ID:       g.ID,
		Source:   "google",
		Position: g.Position,
		Date:     g.Date,
		Link:     link,
		Region:   region,
		Country:  g.Country,
	}
}
