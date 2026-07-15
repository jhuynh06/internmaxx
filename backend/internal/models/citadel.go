package models

import "strings"

// ── Citadel / Citadel Securities (WordPress) ────────────────────────
// POST https://www.{citadel,citadelsecurities}.com/wp-admin/admin-ajax.php
// (action=careers_listing_filter). The response embeds job cards as HTML,
// which the scraper parses into these fields. No posting date is exposed;
// dedup keys on the details-page slug.

type CitadelJob struct {
	Title    string // decoded data-position, e.g. "Software Engineer – Intern (US)"
	Link     string // https://www.citadel.com/careers/details/{slug}/
	Location string // "Greenwich, Miami, New York"
}

func (c CitadelJob) ToJob() Job {
	// The details-page slug is the only stable identifier the cards carry.
	id := strings.TrimSuffix(c.Link, "/")
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	return Job{
		ID:       id,
		Source:   "citadel",
		Position: c.Title,
		Link:     c.Link,
		Region:   c.Location,
	}
}
