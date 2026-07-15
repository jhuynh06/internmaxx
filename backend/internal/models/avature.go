package models

// ── Avature portals (Bloomberg, Two Sigma) ──────────────────────────
// Avature career sites (bloomberg.avature.net, careers.twosigma.com) render
// job lists server-side and expose no public JSON API, so the scraper parses
// the stable `<article class="article article--result">` list markup. Fields
// are limited to what the list page shows: title, detail URL, and location.

type AvatureJob struct {
	ID       string // trailing numeric segment of the JobDetail URL, e.g. "18147"
	Title    string
	Link     string // absolute .../careers/JobDetail/{slug}/{id}
	Location string // "Brasilia, Distrito Federal, Brazil" / "United States - NY New York"
}

// ToJob converts an Avature list item. source is the per-portal key
// ("bloomberg", "twosigma") since one parser serves several companies.
func (a AvatureJob) ToJob(source string) Job {
	return Job{
		ID:       a.ID,
		Source:   source,
		Position: a.Title,
		Link:     a.Link,
		Region:   a.Location,
	}
}
