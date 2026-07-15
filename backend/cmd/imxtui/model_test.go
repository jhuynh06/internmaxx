package main

import (
	"testing"
	"time"

	"github.com/jhuynh06/internmaxx/backend/internal/jobsclient"
)

func testModel() model {
	return initialModel(config{pageLimit: 50, refresh: 30 * time.Second, daysToggle: 7})
}

func job(key string) jobsclient.SeenJob {
	return jobsclient.SeenJob{Key: key, Company: "OpenAI", Title: "Intern", URL: "u/" + key, FirstSeen: "2026-07-14T20:50:20Z"}
}

func resp(items []jobsclient.SeenJob, total int, next *int) jobsclient.JobsResponse {
	return jobsclient.JobsResponse{Items: items, Limit: 50, Total: total, NextOffset: next}
}

func TestFirstLoadDoesNotHighlight(t *testing.T) {
	m := testModel()
	m.applyJobs(jobsMsg{seq: 1, resp: resp([]jobsclient.SeenJob{job("a"), job("b")}, 2, nil)})
	if len(m.items) != 2 {
		t.Fatalf("items = %d, want 2", len(m.items))
	}
	if len(m.newKeys) != 0 {
		t.Errorf("first load should highlight nothing, got %v", m.newKeys)
	}
	if m.loading {
		t.Error("loading should be cleared after a replace load")
	}
}

func TestNewRowHighlightedOnNextPoll(t *testing.T) {
	m := testModel()
	m.applyJobs(jobsMsg{seq: 1, resp: resp([]jobsclient.SeenJob{job("a")}, 1, nil)})
	// A fresh posting "c" arrives on top; "a" is unchanged.
	m.applyJobs(jobsMsg{seq: 1, resp: resp([]jobsclient.SeenJob{job("c"), job("a")}, 2, nil)})
	if !m.newKeys["c"] {
		t.Error("newly-seen key c should be highlighted")
	}
	if m.newKeys["a"] {
		t.Error("previously-seen key a should not be highlighted")
	}
}

func TestAppendGrowsWithoutHighlight(t *testing.T) {
	m := testModel()
	next := 1
	m.applyJobs(jobsMsg{seq: 1, resp: resp([]jobsclient.SeenJob{job("a")}, 3, &next)})
	m.loadingMore = true
	m.applyJobs(jobsMsg{seq: 1, append: true, resp: resp([]jobsclient.SeenJob{job("b"), job("c")}, 3, nil)})
	if len(m.items) != 3 {
		t.Fatalf("items = %d, want 3 (appended)", len(m.items))
	}
	if len(m.newKeys) != 0 {
		t.Errorf("appended older rows should not highlight, got %v", m.newKeys)
	}
	if m.nextOffset != nil {
		t.Error("nextOffset should clear when the last page appended")
	}
	if m.loadingMore {
		t.Error("loadingMore should clear after append")
	}
}

func TestStaleResponseDropped(t *testing.T) {
	m := testModel()
	m.fetchSeq = 5
	newModel, _ := m.Update(jobsMsg{seq: 3, resp: resp([]jobsclient.SeenJob{job("x")}, 1, nil)})
	if len(newModel.(model).items) != 0 {
		t.Error("a jobsMsg with a stale seq must be ignored")
	}
}

func TestViewRendersInVariousStates(t *testing.T) {
	// Exercise View across sizes/states to catch truncate/scroll panics.
	for _, w := range []int{20, 80, 200} {
		for _, h := range []int{6, 24} {
			m := testModel()
			m.width, m.height = w, h
			m.recalcListHeight()
			// empty state
			_ = m.View()
			// populated + selection near the end
			var items []jobsclient.SeenJob
			for i := 0; i < 60; i++ {
				items = append(items, job(string(rune('a'+i%26))+string(rune('0'+i/26))))
			}
			m.applyJobs(jobsMsg{seq: 1, resp: resp(items, 60, nil)})
			m.cursor = 59
			m.clampScroll()
			_ = m.View()
			// filtering + error states
			m.filtering = true
			m.errText = "boom"
			m.recalcListHeight()
			if s := m.View(); s == "" {
				t.Fatalf("View returned empty at %dx%d", w, h)
			}
		}
	}
}

func TestErrorKeepsItems(t *testing.T) {
	m := testModel()
	m.applyJobs(jobsMsg{seq: 1, resp: resp([]jobsclient.SeenJob{job("a")}, 1, nil)})
	out, _ := m.Update(errMsg{seq: 1, err: &jobsclient.APIError{Status: 500}})
	got := out.(model)
	if len(got.items) != 1 {
		t.Error("existing items should survive a fetch error")
	}
	if got.errText == "" {
		t.Error("errText should be set")
	}
}
