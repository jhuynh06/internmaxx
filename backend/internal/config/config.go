// Package config loads runtime configuration from environment variables.
// Everything has a sensible default so the daemon runs with just the two
// Discord secrets set. Load .env yourself (e.g. `set -a; . ./.env`) before
// starting — this package only reads the process environment.
package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Discord webhook (required for notifications to actually send).
	DiscordWebhookID     string
	DiscordWebhookSecret string

	// SQLite database path (dedup + seeding state live here).
	DBPath string

	// Poll intervals per tier.
	Tier1Interval time.Duration
	Tier2Interval time.Duration
	Tier3Interval time.Duration
	AggInterval   time.Duration // aggregator poll cadence, independent of tiers

	// Worker pool size (bounded concurrency across all hosts).
	Workers int

	// Per-host politeness applied by the rate-limited HTTP transport.
	HostMinGap time.Duration

	// Filter behaviour.
	USOnly   bool
	AllowPhD bool

	// Max jobs to alert per company per cycle; overflow is summarized.
	NotifyCap int

	// Optional dead-man's-switch URL pinged after each successful cycle.
	HealthcheckURL string

	// Application-tracking API bind address. Localhost by default (no auth);
	// set to ":8080" to expose, or "" to disable the API entirely.
	APIAddr string
}

func Load() Config {
	return Config{
		DiscordWebhookID:     env("DISCORD_WEBHOOK_ID", ""),
		DiscordWebhookSecret: env("DISCORD_WEBHOOK_SECRET", ""),
		DBPath:               env("DB_PATH", "internmaxx.db"),
		Tier1Interval:        envDur("TIER1_INTERVAL", 5*time.Minute),
		Tier2Interval:        envDur("TIER2_INTERVAL", 15*time.Minute),
		Tier3Interval:        envDur("TIER3_INTERVAL", 60*time.Minute),
		AggInterval:          envDur("AGG_INTERVAL", 5*time.Minute),
		Workers:              envInt("WORKERS", 8),
		HostMinGap:           envDur("HOST_MIN_GAP", 300*time.Millisecond),
		USOnly:               envBool("US_ONLY", true),
		AllowPhD:             envBool("ALLOW_PHD", false),
		NotifyCap:            envInt("NOTIFY_CAP", 25),
		HealthcheckURL:       env("HEALTHCHECK_URL", ""),
		APIAddr:              env("API_ADDR", "127.0.0.1:8080"),
	}
}

// IntervalFor returns the poll interval for a tier (falls back to tier 3).
func (c Config) IntervalFor(tier int) time.Duration {
	switch tier {
	case 1:
		return c.Tier1Interval
	case 2:
		return c.Tier2Interval
	default:
		return c.Tier3Interval
	}
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func envDur(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
