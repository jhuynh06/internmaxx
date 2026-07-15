package scraper

import (
	"context"
	"net/http"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// Bloomberg scrapes bloomberg.avature.net, the Avature portal behind
// careers.bloomberg.com (see avature.go for the shared parsing/pagination).
// It uses the /intern keyword search path: the unfiltered list is ~430
// postings at a locked 12/page while the keyword cut is ~260 — same
// narrow-at-source rationale as Amazon; the filter package does the precise
// cut. List items carry no posted date, so Job.Date is zero (dedup keys on
// the numeric job id, like Workday).
type Bloomberg struct{}

func (Bloomberg) Source() string { return "bloomberg" }

const bloombergBase = "https://bloomberg.avature.net/careers/SearchJobs/intern/"

func (Bloomberg) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	return fetchAvature(ctx, client, bloombergBase, "bloomberg", c)
}
