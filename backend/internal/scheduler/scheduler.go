// Package scheduler drives the poll loop: per-tier tickers feed a bounded
// worker pool, each worker runs scrape -> filter -> dedup -> notify for one
// company, with per-company backoff and per-request jitter. The Simplify
// aggregator runs on its own ticker.
package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/jhuynh06/internmaxx/backend/internal/config"
	"github.com/jhuynh06/internmaxx/backend/internal/filter"
	"github.com/jhuynh06/internmaxx/backend/internal/models"
	"github.com/jhuynh06/internmaxx/backend/internal/notify"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
	"github.com/jhuynh06/internmaxx/backend/internal/scraper"
	"github.com/jhuynh06/internmaxx/backend/internal/store"
)

const (
	maxBackoff   = 24 * time.Hour
	maxJitter    = 5 * time.Second
	brokenAfter  = 3 // consecutive 404s before a slug is flagged broken
	pendingLimit = 200
)

type Scheduler struct {
	cfg         config.Config
	companies   []registry.Company
	scrapers    map[string]scraper.Scraper
	aggregators []*scraper.Simplify
	known       map[string]bool // normalized registry names, for discovery log
	store       *store.Store
	notifier    notify.Notifier
	client      *http.Client
	filterCfg   filter.Config
	log         *slog.Logger

	mu           sync.Mutex
	failures     map[string]int
	nextEligible map[string]time.Time

	notifyMu sync.Mutex // serializes delivery so concurrent workers can't double-send
}

func New(cfg config.Config, companies []registry.Company, aggregators []*scraper.Simplify, known map[string]bool, st *store.Store, n notify.Notifier, client *http.Client, log *slog.Logger) *Scheduler {
	return &Scheduler{
		cfg:          cfg,
		companies:    companies,
		scrapers:     scraper.All(),
		aggregators:  aggregators,
		known:        known,
		store:        st,
		notifier:     n,
		client:       client,
		filterCfg:    filter.Config{USOnly: cfg.USOnly, AllowPhD: cfg.AllowPhD},
		log:          log,
		failures:     map[string]int{},
		nextEligible: map[string]time.Time{},
	}
}

// Run blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	jobs := make(chan registry.Company, 256)
	var wg sync.WaitGroup

	// Worker pool.
	for i := 0; i < s.cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range jobs {
				s.process(ctx, c)
			}
		}()
	}

	// Group companies by tier and start a ticker per present tier.
	byTier := map[int][]registry.Company{}
	for _, c := range s.companies {
		byTier[c.Tier] = append(byTier[c.Tier], c)
	}
	var tickers sync.WaitGroup
	for tier, cos := range byTier {
		tickers.Add(1)
		go func(tier int, cos []registry.Company) {
			defer tickers.Done()
			s.runTier(ctx, tier, cos, jobs)
		}(tier, cos)
	}

	// Aggregator loop (Simplify + vanshb03 …) on the tier-1 cadence.
	if len(s.aggregators) > 0 {
		tickers.Add(1)
		go func() {
			defer tickers.Done()
			s.runAggregators(ctx)
		}()
	}

	<-ctx.Done()
	tickers.Wait()
	close(jobs)
	wg.Wait()
}

func (s *Scheduler) runTier(ctx context.Context, tier int, cos []registry.Company, jobs chan<- registry.Company) {
	interval := s.cfg.IntervalFor(tier)
	enqueue := func() {
		for _, c := range cos {
			select {
			case jobs <- c:
			case <-ctx.Done():
				return
			}
		}
	}
	s.log.Info("tier started", "tier", tier, "companies", len(cos), "interval", interval.String())
	enqueue() // immediate first pass (seeds the store)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			enqueue()
		}
	}
}

func (s *Scheduler) runAggregators(ctx context.Context) {
	interval := s.cfg.AggInterval
	if interval <= 0 {
		interval = s.cfg.Tier1Interval
	}
	names := make([]string, len(s.aggregators))
	for i, a := range s.aggregators {
		names[i] = a.Scope
	}
	s.log.Info("aggregators started", "sources", names, "interval", interval.String())
	for _, a := range s.aggregators {
		s.pollAggregator(ctx, a)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, a := range s.aggregators {
				s.pollAggregator(ctx, a)
			}
		}
	}
}

func (s *Scheduler) pollAggregator(ctx context.Context, agg *scraper.Simplify) {
	jobs, changed, err := agg.Fetch(ctx, s.client)
	if err != nil {
		s.log.Warn("aggregator fetch failed", "source", agg.Scope, "err", err)
		return
	}
	if !changed {
		return
	}
	filtered := filter.Apply(jobs, s.filterCfg)
	s.recordCandidates(ctx, filtered)

	toNotify, err := s.store.Diff(ctx, agg.Scope, filtered)
	if err != nil {
		s.log.Error("aggregator diff failed", "source", agg.Scope, "err", err)
		return
	}
	sent := s.deliver(ctx, toNotify)
	s.log.Info("aggregator cycle", "source", agg.Scope,
		"active", len(jobs), "eng_intern", len(filtered), "new", len(toNotify), "notified", sent)
}

// recordCandidates logs aggregator companies that aren't in the registry, so the
// long tail can be reviewed and promoted. Distinct-per-cycle to keep it cheap.
func (s *Scheduler) recordCandidates(ctx context.Context, jobs []models.Job) {
	if len(s.known) == 0 {
		return
	}
	seen := map[string]bool{}
	var unknown []string
	for _, j := range jobs {
		n := registry.NormalizeName(j.Company)
		if n == "" || s.known[n] || seen[n] {
			continue
		}
		seen[n] = true
		unknown = append(unknown, j.Company)
	}
	fresh, err := s.store.RecordCandidates(ctx, unknown)
	if err != nil {
		s.log.Warn("record candidates failed", "err", err)
		return
	}
	if len(fresh) > 0 {
		sample := fresh
		if len(sample) > 15 {
			sample = sample[:15]
		}
		s.log.Info("discovered untracked companies (candidates to add)",
			"new", len(fresh), "total_this_cycle", len(unknown), "sample", sample)
	}
}

func (s *Scheduler) process(ctx context.Context, c registry.Company) {
	if !s.eligible(c.Slug) {
		return
	}
	sc, ok := s.scrapers[c.ATS]
	if !ok {
		s.log.Warn("no scraper for ats", "company", c.Name, "ats", c.ATS)
		return
	}

	// Desynchronize bursts across companies sharing a host.
	select {
	case <-time.After(jitter()):
	case <-ctx.Done():
		return
	}

	raw, err := sc.Fetch(ctx, s.client, c)
	if err != nil {
		s.handleErr(c, err)
		return
	}
	s.onSuccess(c.Slug)

	filtered := filter.Apply(raw, s.filterCfg)
	scope := c.ATS + "/" + c.Name
	toNotify, err := s.store.Diff(ctx, scope, filtered)
	if err != nil {
		s.log.Error("diff failed", "company", c.Name, "err", err)
		return
	}
	sent := s.deliver(ctx, toNotify)
	if len(raw) > 0 || len(toNotify) > 0 {
		s.log.Info("company cycle",
			"company", c.Name, "ats", c.ATS,
			"raw", len(raw), "eng_intern", len(filtered), "new", len(toNotify), "notified", sent)
	}
}

// deliver sends the fresh jobs (applying the per-cycle cap) then retries any
// still-pending alerts from earlier failed cycles. Serialized across workers so
// two workers can't send the same pending rows twice. Returns fresh jobs sent.
func (s *Scheduler) deliver(ctx context.Context, fresh []models.Job) int {
	s.notifyMu.Lock()
	defer s.notifyMu.Unlock()

	sent := 0
	if len(fresh) > 0 {
		send := fresh
		if len(send) > s.cfg.NotifyCap {
			// Overflow: alert the cap, the rest stay recorded (seen,
			// notified_at NULL) so they won't re-alert or loop. Anomaly guard.
			s.log.Warn("notify cap hit; suppressing overflow",
				"total", len(fresh), "cap", s.cfg.NotifyCap)
			send = fresh[:s.cfg.NotifyCap]
		}
		if err := s.notifier.Notify(ctx, send); err != nil {
			// Leave them NULL; flushPending retries next cycle.
			s.log.Error("notify failed; will retry pending next cycle", "err", err)
		} else {
			keys := make([]string, len(send))
			for i, j := range send {
				keys[i] = j.Key()
			}
			if err := s.store.MarkNotified(ctx, keys); err != nil {
				s.log.Error("mark notified failed", "err", err)
			}
			sent = len(send)
		}
	}

	// Retry anything still unsent (this cycle's failures or prior outages).
	// Runs after the fresh send+mark above, so freshly-notified rows are not
	// re-sent here.
	s.flushPending(ctx)
	return sent
}

func (s *Scheduler) flushPending(ctx context.Context) {
	pending, err := s.store.Pending(ctx, pendingLimit)
	if err != nil {
		s.log.Warn("pending lookup failed", "err", err)
		return
	}
	if len(pending) == 0 {
		return
	}
	jobs := make([]models.Job, len(pending))
	keys := make([]string, len(pending))
	for i, p := range pending {
		jobs[i] = models.Job{Company: p.Company, Position: p.Title, Link: p.URL}
		keys[i] = p.Key
	}
	if err := s.notifier.Notify(ctx, jobs); err != nil {
		s.log.Warn("pending resend failed", "count", len(jobs), "err", err)
		return
	}
	if err := s.store.MarkNotified(ctx, keys); err != nil {
		s.log.Error("mark notified (pending) failed", "err", err)
	}
	s.log.Info("resent pending alerts", "count", len(jobs))
}

// ── backoff / eligibility ───────────────────────────────────────────

func (s *Scheduler) eligible(slug string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	next, ok := s.nextEligible[slug]
	if !ok {
		return true
	}
	return time.Now().After(next)
}

func (s *Scheduler) onSuccess(slug string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.failures, slug)
	delete(s.nextEligible, slug)
}

func (s *Scheduler) handleErr(c registry.Company, err error) {
	s.mu.Lock()
	s.failures[c.Slug]++
	n := s.failures[c.Slug]
	// Exponential backoff from the company's tier interval, capped.
	backoff := s.cfg.IntervalFor(c.Tier)
	for i := 1; i < n && backoff < maxBackoff; i++ {
		backoff *= 2
	}
	if backoff > maxBackoff {
		backoff = maxBackoff
	}
	s.nextEligible[c.Slug] = time.Now().Add(backoff)
	s.mu.Unlock()

	var he *scraper.HTTPError
	if errors.As(err, &he) && he.Status == http.StatusNotFound && n >= brokenAfter {
		s.log.Error("slug appears broken (repeated 404) — check registry",
			"company", c.Name, "ats", c.ATS, "slug", c.Slug, "failures", n)
		return
	}
	s.log.Warn("fetch failed; backing off",
		"company", c.Name, "ats", c.ATS, "err", err, "failures", n, "backoff", backoff.String())
}

// jitter returns a short random delay to desynchronize bursts. Exposed for the
// worker to call before fetching.
func jitter() time.Duration {
	return time.Duration(rand.Int63n(int64(maxJitter)))
}
