// Command internmaxx is the long-running daemon: it polls the scoped company
// registry plus the Simplify aggregator, dedupes, and sends Discord alerts.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jhuynh06/internmaxx/backend/internal/api"
	"github.com/jhuynh06/internmaxx/backend/internal/cli"
	"github.com/jhuynh06/internmaxx/backend/internal/config"
	"github.com/jhuynh06/internmaxx/backend/internal/notify"
	"github.com/jhuynh06/internmaxx/backend/internal/registry"
	"github.com/jhuynh06/internmaxx/backend/internal/scheduler"
	"github.com/jhuynh06/internmaxx/backend/internal/scraper"
	"github.com/jhuynh06/internmaxx/backend/internal/store"
)

func main() {
	fs := flag.NewFlagSet("internmaxx", flag.ExitOnError)
	companiesPath := fs.String("companies", "companies.yaml", "path to companies.yaml")
	scopeFlags := cli.RegisterScope(fs)
	_ = fs.Parse(os.Args[1:])

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()

	scope, err := scopeFlags.Scope()
	if err != nil {
		log.Error("bad scope", "err", err)
		os.Exit(1)
	}

	reg, err := registry.Load(*companiesPath)
	if err != nil {
		log.Error("load registry", "err", err)
		os.Exit(1)
	}
	companies := reg.Filter(scope)
	if len(companies) == 0 {
		log.Error("no companies match scope", "companies_file", *companiesPath)
		os.Exit(1)
	}
	log.Info("registry loaded", "total", len(reg.Companies), "scoped", len(companies))

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Error("open store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	var notifier notify.Notifier
	if cfg.DiscordWebhookID != "" && cfg.DiscordWebhookSecret != "" {
		notifier = notify.NewDiscord(cfg.DiscordWebhookID, cfg.DiscordWebhookSecret)
	} else {
		log.Warn("no Discord webhook configured; running without notifications (dedup/seeding still active)")
		notifier = notify.Nop{}
	}

	client := scraper.NewClient(cfg.HostMinGap)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.HealthcheckURL != "" {
		go pingHealthcheck(ctx, client, cfg.HealthcheckURL, log)
	}

	if cfg.APIAddr != "" {
		apiSrv := api.New(cfg.APIAddr, st, log)
		go func() {
			if err := apiSrv.Run(ctx); err != nil {
				log.Error("api server stopped", "err", err)
			}
		}()
	}

	aggregators := []*scraper.Simplify{scraper.NewSimplify(), scraper.NewVansh()}
	sched := scheduler.New(cfg, companies, aggregators, reg.KnownNames(), st, notifier, client, log)
	log.Info("starting scheduler", "aggregators", len(aggregators))
	sched.Run(ctx)
	log.Info("shutdown complete")
}

func pingHealthcheck(ctx context.Context, client *http.Client, url string, log *slog.Logger) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if resp, err := client.Do(req); err != nil {
				log.Warn("healthcheck ping failed", "err", err)
			} else {
				resp.Body.Close()
			}
		}
	}
}
