package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
)

// Simplify polls a GitHub internship-list repo's listings.json — one GET
// covering thousands of listings across companies not in the direct-ATS
// registry. It is an aggregator, not a per-company Scraper: it holds state (the
// last commit SHA) so it can skip re-downloading the 10MB+ file when unchanged.
//
// The SimplifyJobs, vanshb03, and cvrve repos all share the same listings.json
// schema and path, so this one type serves all of them via different Owner/Repo.
type Simplify struct {
	Scope  string // dedup/seeding scope + log tag, e.g. "simplify" / "vanshb03"
	Owner  string // GitHub owner, e.g. "SimplifyJobs"
	Repo   string // e.g. "Summer2026-Internships"
	Branch string // e.g. "dev"

	mu      sync.Mutex
	lastSHA string
}

// NewSimplify returns the SimplifyJobs/Pitt-CSC Summer 2026 source (largest).
func NewSimplify() *Simplify {
	return &Simplify{Scope: "simplify", Owner: "SimplifyJobs", Repo: "Summer2026-Internships", Branch: "dev"}
}

// NewVansh returns the vanshb03/CSCareers Summer 2026 source (catches misses).
func NewVansh() *Simplify {
	return &Simplify{Scope: "vanshb03", Owner: "vanshb03", Repo: "Summer2026-Internships", Branch: "dev"}
}

func (s *Simplify) Source() string { return "simplify" }

// Fetch returns active, visible listings. If the branch HEAD is unchanged since
// the last call it returns (nil, false, nil) — no download, nothing new.
func (s *Simplify) Fetch(ctx context.Context, client *http.Client) (jobs []models.Job, changed bool, err error) {
	sha, err := s.headSHA(ctx, client)
	if err != nil {
		return nil, false, err
	}

	s.mu.Lock()
	unchanged := sha != "" && sha == s.lastSHA
	s.mu.Unlock()
	if unchanged {
		return nil, false, nil
	}

	raw, err := s.download(ctx, client)
	if err != nil {
		return nil, false, err
	}

	jobs = make([]models.Job, 0, len(raw))
	for _, r := range raw {
		if !r.Active || !r.IsVisible {
			continue
		}
		jobs = append(jobs, r.ToJob())
	}

	s.mu.Lock()
	s.lastSHA = sha
	s.mu.Unlock()
	return jobs, true, nil
}

func (s *Simplify) headSHA(ctx context.Context, client *http.Client) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s", s.Owner, s.Repo, s.Branch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.sha")
	req.Header.Set("User-Agent", "internmaxx/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Non-fatal: fall back to always downloading.
		return "", nil
	}
	// With the .sha media type the body is the raw SHA string.
	var buf [64]byte
	n, _ := resp.Body.Read(buf[:])
	return string(buf[:n]), nil
}

func (s *Simplify) download(ctx context.Context, client *http.Client) ([]models.SimplifyJob, error) {
	url := fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/%s/.github/scripts/listings.json",
		s.Owner, s.Repo, s.Branch,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "internmaxx/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{URL: url, Status: resp.StatusCode}
	}
	var raw []models.SimplifyJob
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode simplify listings: %w", err)
	}
	return raw, nil
}
