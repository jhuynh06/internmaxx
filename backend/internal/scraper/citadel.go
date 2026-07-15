package scraper

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// Citadel scrapes the WordPress careers listing shared by citadel.com and
// citadelsecurities.com (both sit behind Cloudflare, which currently passes
// plain HTTP with ordinary browser headers). One scraper covers both firms:
// the registry slug selects the host (www.{slug}.com).
//
// The admin-ajax endpoint returns JSON whose `content` field is rendered HTML
// job cards, parsed here with regexps — a theme redesign will break this.
// experience-filter=internships narrows at the source (their own nav link).
type Citadel struct{}

func (Citadel) Source() string { return "citadel" }

const (
	citadelPerPage  = 100 // honored by the endpoint (all ~40-50 intern posts in one page)
	citadelMaxPages = 10  // safety bound (=1000 postings)
)

type citadelResponse struct {
	NumberOfPost int    `json:"number_of_post"`
	FoundPosts   int    `json:"found_posts"`
	Content      string `json:"content"` // rendered HTML job cards
}

// citadelCardRe pairs each card's link, title, and location. href and
// data-position sit in the same <a> tag ([^>]* cannot escape it), and cards
// are emitted sequentially, so lazy matching keeps the fields associated.
// The location element is a <span> on citadel.com but a <div> on
// citadelsecurities.com, hence the alternation.
var citadelCardRe = regexp.MustCompile(
	`(?s)href="(https://[^"]+/careers/details/[^"]+)"[^>]*data-position="([^"]*)".*?careers-listing-card__location">\s*(.*?)\s*</(?:span|div)>`)

func (Citadel) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	if c.Slug == "" {
		return nil, fmt.Errorf("citadel: %s missing slug (citadel or citadelsecurities)", c.Name)
	}
	host := "www." + c.Slug + ".com"

	var jobs []models.Job
	for page := 1; page <= citadelMaxPages; page++ {
		form := url.Values{}
		form.Set("action", "careers_listing_filter")
		form.Set("experience-filter", "internships")
		form.Set("current_page", strconv.Itoa(page))
		form.Set("per_page", strconv.Itoa(citadelPerPage))
		form.Set("sort_order", "DESC")

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			"https://"+host+"/wp-admin/admin-ajax.php", strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Origin", "https://"+host)
		req.Header.Set("Referer", "https://"+host+"/careers/open-opportunities/")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")

		var resp citadelResponse
		if err := doJSON(client, req, &resp); err != nil {
			return nil, err
		}

		cards := citadelCardRe.FindAllStringSubmatch(resp.Content, -1)
		for _, m := range cards {
			j := models.CitadelJob{
				Title:    html.UnescapeString(m[2]),
				Link:     m[1],
				Location: html.UnescapeString(m[3]),
			}
			job := j.ToJob()
			job.Company = c.Name
			jobs = append(jobs, job)
		}
		if len(cards) == 0 || resp.NumberOfPost < citadelPerPage || len(jobs) >= resp.FoundPosts {
			break
		}
	}
	return jobs, nil
}
