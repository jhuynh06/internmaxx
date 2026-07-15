package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
)

// Shopify scrapes shopify.com/careers via its React Router "single fetch"
// endpoint /careers.data. The response is a turbo-stream payload: one flat
// JSON array in which objects are encoded as {"_<keyIndex>": <valueIndex>},
// arrays as lists of indices, and scalars as themselves — so the decoder below
// resolves indices recursively. All listed postings arrive in one request
// (there is no pagination; the careers page filters client-side).
//
// Fragility note: turbo-stream is an implementation detail of the site's
// framework; a redesign breaks this loudly (marker not found), not silently.
type Shopify struct{}

func (Shopify) Source() string { return "shopify" }

const (
	shopifyDataURL = "https://www.shopify.com/careers.data"
	shopifyKey     = `"jobPostingsWithJobs"`
	shopifyDepth   = 8 // jobPosting objects are ≤3 levels deep; bound recursion
)

func (Shopify) Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error) {
	var arr []json.RawMessage
	if err := getJSON(ctx, client, shopifyDataURL, &arr); err != nil {
		return nil, err
	}
	raws, err := shopifyPostings(arr)
	if err != nil {
		return nil, err
	}
	jobs := make([]models.Job, 0, len(raws))
	for _, r := range raws {
		job := r.ToJob()
		job.Company = c.Name
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// shopifyPostings locates the jobPostingsWithJobs list in the flat array and
// resolves each {jobPosting, job} pair into a models.ShopifyJob.
func shopifyPostings(arr []json.RawMessage) ([]models.ShopifyJob, error) {
	keyIdx := -1
	for i, raw := range arr {
		if string(bytes.TrimSpace(raw)) == shopifyKey {
			keyIdx = i
			break
		}
	}
	if keyIdx < 0 {
		return nil, fmt.Errorf("shopify: %s marker not found in careers.data", shopifyKey)
	}
	ref := "_" + strconv.Itoa(keyIdx)
	for _, raw := range arr {
		var obj map[string]int
		if json.Unmarshal(raw, &obj) != nil {
			continue
		}
		listIdx, ok := obj[ref]
		if !ok {
			continue
		}
		list, ok := turboValue(arr, listIdx, shopifyDepth).([]any)
		if !ok {
			continue
		}
		var out []models.ShopifyJob
		for _, it := range list {
			item, ok := it.(map[string]any)
			if !ok {
				continue
			}
			posting, ok := item["jobPosting"].(map[string]any)
			if !ok {
				continue
			}
			b, err := json.Marshal(posting)
			if err != nil {
				continue
			}
			var sj models.ShopifyJob
			if json.Unmarshal(b, &sj) == nil && sj.ID != "" {
				out = append(out, sj)
			}
		}
		if len(out) > 0 {
			return out, nil
		}
	}
	return nil, fmt.Errorf("shopify: jobPostingsWithJobs list did not resolve to any postings")
}

// turboValue materializes the value at index idx of a turbo-stream flat array.
// Negative indices are the format's sentinels (undefined/null/NaN/...) and
// resolve to nil, which is fine for the string fields extracted here.
func turboValue(arr []json.RawMessage, idx, depth int) any {
	if depth <= 0 || idx < 0 || idx >= len(arr) {
		return nil
	}
	raw := arr[idx]

	// Object: every key is "_<index of key string>", every value an index.
	var obj map[string]int
	if json.Unmarshal(raw, &obj) == nil && len(obj) > 0 {
		out := make(map[string]any, len(obj))
		for k, vi := range obj {
			if !strings.HasPrefix(k, "_") {
				return decodeLiteral(raw) // not reference-encoded; keep as-is
			}
			ki, err := strconv.Atoi(k[1:])
			if err != nil || ki < 0 || ki >= len(arr) {
				continue
			}
			var key string
			if json.Unmarshal(arr[ki], &key) != nil {
				continue
			}
			out[key] = turboValue(arr, vi, depth-1)
		}
		return out
	}

	// Array: elements are indices.
	var idxs []int
	if json.Unmarshal(raw, &idxs) == nil && bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
		out := make([]any, 0, len(idxs))
		for _, i := range idxs {
			out = append(out, turboValue(arr, i, depth-1))
		}
		return out
	}

	return decodeLiteral(raw)
}

func decodeLiteral(raw json.RawMessage) any {
	var v any
	_ = json.Unmarshal(raw, &v)
	return v
}
