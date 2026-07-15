package scraper

import (
	"context"
	"net/http"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// TwoSigma scrapes careers.twosigma.com, an Avature portal on Two Sigma's own
// domain (see avature.go for the shared parsing/pagination). Its search page
// is /careers/OpenRoles (the usual Avature /careers/SearchJobs path 404s on
// this portal) and lists every open role — ~100 postings at a locked 10/page,
// so no keyword narrowing is needed. List items carry no posted date, so
// Job.Date is zero (dedup keys on the numeric job id, like Workday).
type TwoSigma struct{}

func (TwoSigma) Source() string { return "twosigma" }

const twoSigmaBase = "https://careers.twosigma.com/careers/OpenRoles"

func (TwoSigma) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	return fetchAvature(ctx, client, twoSigmaBase, "twosigma", c)
}
