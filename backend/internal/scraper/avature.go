package scraper

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// avature.go holds the shared machinery for Avature-hosted career portals
// (Bloomberg, Two Sigma). Avature renders search results as server-side HTML
// with no public JSON variant (Accept: application/json is ignored), so we
// parse the list markup and paginate with ?jobOffset=N until a page has no
// results. Page size is portal-fixed (Bloomberg 12, Two Sigma 10) and the
// jobRecordsPerPage parameter is ignored, so the offset advances by however
// many articles each page actually returned.

const avatureMaxPages = 60 // safety bound; portals here list 100-450 postings

var (
	avatureArticleRe = regexp.MustCompile(`(?s)<article class="article article--result".*?</article>`)
	avatureLinkRe    = regexp.MustCompile(`(?s)<a class="link" href="([^"]*?/careers/JobDetail/[^"]+)"\s*>(.*?)</a>`)
	// Bloomberg puts the location in list-item-location; Two Sigma's first
	// paragraph_inner-span is the location (later spans are team/level).
	avatureLocationRe = regexp.MustCompile(`(?s)<span class="(?:list-item-location|paragraph_inner-span)">(.*?)</span>`)
	avatureSpaceRe    = regexp.MustCompile(`\s+`)
)

// fetchAvature pages through base (which may already carry a query string),
// converting each parsed article via models.AvatureJob.ToJob(source).
func fetchAvature(ctx context.Context, client *http.Client, base, source string, c registry.Company) ([]models.Job, error) {
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	var jobs []models.Job
	seen := map[string]bool{}
	offset := 0
	for page := 0; page < avatureMaxPages; page++ {
		url := fmt.Sprintf("%s%sjobOffset=%d", base, sep, offset)
		body, err := getHTML(ctx, client, url)
		if err != nil {
			return nil, err
		}
		articles := avatureArticleRe.FindAllString(body, -1)
		if len(articles) == 0 {
			break // past the last page
		}
		added := 0
		for _, a := range articles {
			raw, ok := parseAvatureArticle(a)
			if !ok || seen[raw.ID] {
				continue
			}
			seen[raw.ID] = true
			job := raw.ToJob(source)
			job.Company = c.Name
			jobs = append(jobs, job)
			added++
		}
		if added == 0 {
			break // repeating content; don't loop forever
		}
		offset += len(articles)
	}
	return jobs, nil
}

// parseAvatureArticle extracts one job from an article--result block.
func parseAvatureArticle(article string) (models.AvatureJob, bool) {
	m := avatureLinkRe.FindStringSubmatch(article)
	if m == nil {
		return models.AvatureJob{}, false
	}
	link, title := m[1], avatureClean(m[2])
	id := link[strings.LastIndex(link, "/")+1:]
	if id == "" || title == "" {
		return models.AvatureJob{}, false
	}
	location := ""
	if lm := avatureLocationRe.FindStringSubmatch(article); lm != nil {
		location = avatureClean(lm[1])
	}
	return models.AvatureJob{ID: id, Title: title, Link: link, Location: location}, true
}

// avatureClean unescapes entities and collapses the template's whitespace.
func avatureClean(s string) string {
	return strings.TrimSpace(avatureSpaceRe.ReplaceAllString(html.UnescapeString(s), " "))
}

// getHTML fetches a page body as a string (capped at 8MB — deshaw.com/careers,
// the largest page scraped this way, is ~1.8MB). Shared by the HTML-scraping
// sources (Avature portals, D. E. Shaw).
func getHTML(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "internmaxx/1.0 (+https://github.com/jhuynh06/internmaxx)")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return "", &HTTPError{URL: url, Status: resp.StatusCode, Body: string(snippet)}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", url, err)
	}
	return string(b), nil
}
