package scraper

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// rateLimited is an http.RoundTripper that enforces a minimum gap between
// requests to the same host. Public ATS endpoints are unauthenticated but will
// 429/ban abusive clients, and ~125 registry companies share
// boards-api.greenhouse.io — so politeness is per host, not global.
type rateLimited struct {
	base   http.RoundTripper
	minGap time.Duration

	mu    sync.Mutex
	gates map[string]*sync.Mutex
	last  map[string]time.Time
}

func (r *rateLimited) gate(host string) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.gates[host]
	if !ok {
		g = &sync.Mutex{}
		r.gates[host] = g
	}
	return g
}

func (r *rateLimited) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	g := r.gate(host)

	// Serialize per host and space requests by minGap. Holding the per-host
	// lock during the wait is intentional: it enforces spacing without a
	// separate scheduler, while other hosts proceed in parallel.
	g.Lock()
	r.mu.Lock()
	wait := time.Until(r.last[host].Add(r.minGap))
	r.mu.Unlock()
	if wait > 0 {
		select {
		case <-time.After(wait):
		case <-req.Context().Done():
			g.Unlock()
			return nil, req.Context().Err()
		}
	}
	r.mu.Lock()
	r.last[host] = time.Now()
	r.mu.Unlock()
	g.Unlock()

	return r.base.RoundTrip(req)
}

// NewClient returns an HTTP client whose transport is per-host rate limited.
func NewClient(minGap time.Duration) *http.Client {
	if minGap <= 0 {
		minGap = 300 * time.Millisecond
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &rateLimited{
			base:   http.DefaultTransport,
			minGap: minGap,
			gates:  map[string]*sync.Mutex{},
			last:   map[string]time.Time{},
		},
	}
}

// getJSON performs a GET and decodes JSON into dst. It returns a non-nil error
// for non-2xx responses so callers can back off.
func getJSON(ctx context.Context, client *http.Client, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "internmaxx/1.0 (+https://github.com/jhuynh06/internmaxx)")
	return doJSON(client, req, dst)
}
