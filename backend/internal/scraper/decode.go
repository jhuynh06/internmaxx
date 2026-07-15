package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// doJSON executes req, checks the status, and decodes the JSON body into dst.
func doJSON(client *http.Client, req *http.Request, dst any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Drain a little of the body for context without reading megabytes.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return &HTTPError{URL: req.URL.String(), Status: resp.StatusCode, Body: string(snippet)}
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode %s: %w", req.URL.String(), err)
	}
	return nil
}

// HTTPError carries the status code so the scheduler can distinguish 404
// (dead slug) from 429/5xx (transient, back off).
type HTTPError struct {
	URL    string
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("http %d for %s", e.Status, e.URL)
}
