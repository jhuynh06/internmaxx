// Package jobsclient is the shared read-only client for the /jobs browse API,
// used by the imx CLI and the imxtui TUI. It centralizes the response shape,
// server-address resolution, company-name resolution, and time formatting so
// the two front-ends can't drift.
package jobsclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// SeenJob is one posting as returned by GET /jobs.
type SeenJob struct {
	Key       string `json:"key"`
	Company   string `json:"company"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	FirstSeen string `json:"first_seen"`
}

// JobsResponse is one page of postings plus pagination metadata. NextOffset is
// nil on the last page.
type JobsResponse struct {
	Items      []SeenJob `json:"items"`
	Limit      int       `json:"limit"`
	Offset     int       `json:"offset"`
	Total      int       `json:"total"`
	NextOffset *int      `json:"next_offset"`
}

// Query mirrors the /jobs query params. Zero values are omitted (Limit defaults
// to 20 when unset).
type Query struct {
	Company string
	Days    int
	Limit   int
	Offset  int
}

// APIError is a non-2xx response carrying the server's {"error"} message.
type APIError struct {
	Status int
	Msg    string
}

func (e *APIError) Error() string {
	if e.Msg != "" {
		return fmt.Sprintf("server error (%d): %s", e.Status, e.Msg)
	}
	return fmt.Sprintf("server returned %d", e.Status)
}

// Client talks to one /jobs endpoint.
type Client struct {
	Base string
	HTTP *http.Client
}

// New builds a Client, resolving the base address (see ResolveAddr).
func New(addrFlag string) *Client {
	return &Client{Base: ResolveAddr(addrFlag), HTTP: &http.Client{Timeout: 10 * time.Second}}
}

// Jobs fetches one page. It returns the decoded response, the raw body (so
// callers can emit --json verbatim), and an error (*APIError on non-2xx).
func (c *Client) Jobs(ctx context.Context, q Query) (JobsResponse, []byte, error) {
	vals := url.Values{}
	limit := q.Limit
	if limit < 1 {
		limit = 20
	}
	vals.Set("limit", strconv.Itoa(limit))
	vals.Set("offset", strconv.Itoa(q.Offset))
	if q.Days > 0 {
		vals.Set("days", strconv.Itoa(q.Days))
	}
	if q.Company != "" {
		vals.Set("company", q.Company)
	}
	endpoint := strings.TrimRight(c.Base, "/") + "/jobs?" + vals.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return JobsResponse{}, nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return JobsResponse{}, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode/100 != 2 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		return JobsResponse{}, body, &APIError{Status: resp.StatusCode, Msg: e.Error}
	}

	var jr JobsResponse
	if err := json.Unmarshal(body, &jr); err != nil {
		return JobsResponse{}, body, err
	}
	if jr.Items == nil {
		jr.Items = []SeenJob{}
	}
	return jr, body, nil
}

// ResolveAddr picks the API base URL (--addr > IMX_API_ADDR > API_ADDR >
// localhost) and normalizes it to an http:// URL.
func ResolveAddr(flagVal string) string {
	v := flagVal
	if v == "" {
		v = os.Getenv("IMX_API_ADDR")
	}
	if v == "" {
		v = os.Getenv("API_ADDR")
	}
	if v == "" {
		v = "http://127.0.0.1:8080"
	}
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		return v
	}
	if strings.HasPrefix(v, ":") { // bare ":8080" (the API_ADDR shorthand)
		return "http://127.0.0.1" + v
	}
	return "http://" + v
}

// ResolveCompany maps a slug or loose name to the registry display name stored
// in seen_jobs. Best-effort: if the registry is absent or has no match, the
// argument passes through (the server matches company case-insensitively). The
// returned *registry.Registry is nil only when the file couldn't be loaded.
func ResolveCompany(arg, path string) (string, *registry.Registry) {
	reg, err := registry.Load(path)
	if err != nil {
		return arg, nil
	}
	want := registry.NormalizeName(arg)
	lower := strings.ToLower(strings.TrimSpace(arg))
	for _, co := range reg.Companies {
		if strings.ToLower(co.Slug) == lower || registry.NormalizeName(co.Name) == want {
			return co.Name, reg
		}
	}
	return arg, reg
}

// NearCompanies returns registry display names loosely matching arg, for a
// "did you mean" hint on an empty result (capped at 8).
func NearCompanies(arg string, reg *registry.Registry) []string {
	if reg == nil {
		return nil
	}
	want := registry.NormalizeName(arg)
	var near []string
	for _, co := range reg.Companies {
		n := registry.NormalizeName(co.Name)
		if strings.Contains(n, want) || strings.Contains(want, n) {
			near = append(near, co.Name)
		}
	}
	if len(near) > 8 {
		near = near[:8]
	}
	return near
}

// FormatSeen renders an RFC3339 first_seen in local time, falling back to the
// raw string so a malformed row never crashes rendering.
func FormatSeen(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.Local().Format("2006-01-02 15:04")
}
