// Package api exposes a small JSON HTTP API for unlimited application tracking.
// It runs alongside the scraper daemon. There's no auth, so it binds to
// localhost by default (config API_ADDR) — expose it only behind your own auth
// or an SSH tunnel.
//
// Routes:
//
//	GET    /healthz
//	GET    /jobs                    (?company= &days= &limit= &offset=; seen postings, newest first)
//	GET    /applications            (optional ?status=applied)
//	POST   /applications            {job_key, company, title, url, status, notes}
//	GET    /applications/{key...}
//	PUT    /applications/{key...}    (upsert; body may omit job_key)
//	DELETE /applications/{key...}
//	GET    /statuses                (the allowed status pipeline)
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/store"
)

type Server struct {
	store *store.Store
	log   *slog.Logger
	srv   *http.Server
}

func New(addr string, st *store.Store, log *slog.Logger) *Server {
	s := &Server{store: st, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /statuses", s.statuses)
	mux.HandleFunc("GET /jobs", s.jobs)
	mux.HandleFunc("GET /applications", s.list)
	mux.HandleFunc("POST /applications", s.upsert)
	// {key...} (trailing wildcard) because job keys contain slashes
	// (source/company/id), which a single-segment {key} would not match.
	mux.HandleFunc("GET /applications/{key...}", s.get)
	mux.HandleFunc("PUT /applications/{key...}", s.upsertKey)
	mux.HandleFunc("DELETE /applications/{key...}", s.del)
	s.srv = &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return s
}

// Run serves until ctx is cancelled, then shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutCtx)
	}()
	s.log.Info("application tracking API listening", "addr", s.srv.Addr)
	if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) statuses(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, models.ApplicationStatuses)
}

// jobsResponse is one page of seen postings plus pagination metadata.
// NextOffset is nil on the last page.
type jobsResponse struct {
	Items      []store.SeenJob `json:"items"`
	Limit      int             `json:"limit"`
	Offset     int             `json:"offset"`
	Total      int             `json:"total"`
	NextOffset *int            `json:"next_offset,omitempty"`
}

// jobs lists seen postings newest-first, optionally filtered by company and a
// recency window (?days=N), with offset pagination.
func (s *Server) jobs(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit", 20)
	if err != nil || limit < 1 {
		writeErr(w, http.StatusBadRequest, "invalid limit")
		return
	}
	if limit > 100 {
		limit = 100
	}
	offset, err := queryInt(r, "offset", 0)
	if err != nil || offset < 0 {
		writeErr(w, http.StatusBadRequest, "invalid offset")
		return
	}
	days, err := queryInt(r, "days", 0)
	if err != nil || days < 0 {
		writeErr(w, http.StatusBadRequest, "invalid days")
		return
	}
	if days > 3650 {
		days = 3650
	}

	opts := store.ListJobsOpts{Company: r.URL.Query().Get("company"), Limit: limit, Offset: offset}
	if days > 0 {
		opts.Since = time.Now().UTC().AddDate(0, 0, -days)
	}

	items, total, err := s.store.ListJobs(r.Context(), opts)
	if err != nil {
		s.fail(w, err)
		return
	}
	if items == nil {
		items = []store.SeenJob{}
	}
	resp := jobsResponse{Items: items, Limit: limit, Offset: offset, Total: total}
	if next := offset + len(items); next < total {
		resp.NextOffset = &next
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status != "" && !models.ValidStatus(status) {
		writeErr(w, http.StatusBadRequest, "invalid status filter")
		return
	}
	apps, err := s.store.ListApplications(r.Context(), status)
	if err != nil {
		s.fail(w, err)
		return
	}
	if apps == nil {
		apps = []models.Application{}
	}
	writeJSON(w, http.StatusOK, apps)
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	app, ok, err := s.store.GetApplication(r.Context(), r.PathValue("key"))
	if err != nil {
		s.fail(w, err)
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (s *Server) upsert(w http.ResponseWriter, r *http.Request) {
	s.doUpsert(w, r, "")
}

func (s *Server) upsertKey(w http.ResponseWriter, r *http.Request) {
	s.doUpsert(w, r, r.PathValue("key"))
}

func (s *Server) doUpsert(w http.ResponseWriter, r *http.Request, key string) {
	var app models.Application
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&app); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if key != "" {
		app.JobKey = key
	}
	if app.JobKey == "" {
		writeErr(w, http.StatusBadRequest, "job_key is required")
		return
	}
	if app.Status == "" {
		app.Status = "saved"
	}
	if !models.ValidStatus(app.Status) {
		writeErr(w, http.StatusBadRequest, "invalid status; see GET /statuses")
		return
	}
	if err := s.store.UpsertApplication(r.Context(), app); err != nil {
		s.fail(w, err)
		return
	}
	saved, _, err := s.store.GetApplication(r.Context(), app.JobKey)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) del(w http.ResponseWriter, r *http.Request) {
	ok, err := s.store.DeleteApplication(r.Context(), r.PathValue("key"))
	if err != nil {
		s.fail(w, err)
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	s.log.Error("api error", "err", err)
	writeErr(w, http.StatusInternalServerError, "internal error")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// queryInt parses an optional integer query param, returning def when absent
// and an error when present but non-integer.
func queryInt(r *http.Request, name string, def int) (int, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, nil
	}
	return strconv.Atoi(v)
}
