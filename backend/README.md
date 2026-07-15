# internmaxx (backend)

Watches curated company job boards plus the Simplify/vanshb03 aggregators,
filters to SWE/engineering internships, dedupes, and pushes a Discord alert
within minutes of a posting going live. A small read-only HTTP API exposes what
it has seen, browsable from a CLI (`imx`) or a live TUI (`imxtui`).

One static Go binary + one SQLite file — no external services beyond a Discord
webhook. See [`../PLAN.md`](../PLAN.md) for the full design and roadmap.

## How it works

```
registry (companies.yaml) ─┐
                           ├─►  scrape  ─►  filter  ─►  dedup   ─►  notify (Discord)
Simplify / vanshb03 feeds ─┘   (per ATS)   (intern +   (SQLite     + record in
                                            SWE/eng)    seen_jobs)    seen_jobs
```

- **Scrape** — each company declares an ATS (`ashby`, `greenhouse`, `lever`,
  `smartrecruiters`, `workday`, Oracle/Eightfold, …); the matching scraper pulls
  its public board. Two GitHub-hosted aggregators are polled alongside. All HTTP
  goes through a per-host rate-limited transport.
- **Filter** — keeps internships classified as SWE/engineering, with negative
  and location guards (`US_ONLY`, `ALLOW_PHD`).
- **Dedup + seed** — every posting has a stable key; the first sighting is
  recorded in `seen_jobs`. See [Seeding](#seeding) for why the first run is
  quiet.
- **Notify** — new postings fire a Discord webhook (capped per company per
  cycle; overflow is summarized). Undelivered alerts self-heal on the next cycle.

## Quick start

```bash
cd backend
cp .env.example .env                # then fill in the two Discord values
set -a; . ./.env; set +a            # load env into the shell

go run ./cmd/verify --only openai   # sanity-check a board resolves
go run ./cmd/scrape-once --only openai   # one pass, prints matches, no DB/notify
go run ./cmd/internmaxx --groups ai,quant   # run the daemon, scoped
```

Requires Go 1.26+. The SQLite driver is pure Go (`modernc.org/sqlite`), so
everything cross-compiles with `CGO_ENABLED=0`.

## Layout

```
cmd/
  internmaxx/    long-running daemon (poll → filter → dedup → notify + API)
  scrape-once/   single pass, prints matches, no DB/notify — test a slug
  verify/        checks registry slugs resolve to live boards (+ GH name match)
  imx/           read-only CLI: browse seen postings via the /jobs API
  imxtui/        interactive TUI: live newest-first feed over the same API
internal/
  config/        env-var configuration (see below)
  registry/      companies.yaml loader + run scoping (groups/tiers/only/exclude)
  scraper/       Scraper interface + per-ATS implementations + aggregators
                 (per-host rate-limited HTTP transport lives here)
  filter/        intern detection + SWE/eng classification + negative/location guards
  store/         SQLite: seen_jobs dedup/seeding, candidate log, applications, /jobs reads
  notify/        Notifier interface + Discord implementation
  scheduler/     tier tickers, worker pool, per-company backoff, aggregators, discovery
  api/           read-only /jobs browse API + application-tracking API
  jobsclient/    shared HTTP client for /jobs (used by imx and imxtui)
companies.yaml   verified registry (~230 entries, tiered + grouped)
```

## Configuration

All via environment variables (load `.env` first). Every value has a default, so
the daemon runs with just the two Discord secrets set.

| Variable | Default | Purpose |
|---|---|---|
| `DISCORD_WEBHOOK_ID` / `DISCORD_WEBHOOK_SECRET` | — | Webhook halves; without them the daemon runs but sends nothing |
| `DB_PATH` | `internmaxx.db` | SQLite file (dedup + seeding state) — keep across restarts |
| `TIER1_INTERVAL` / `TIER2_INTERVAL` / `TIER3_INTERVAL` | `5m` / `15m` / `60m` | Poll cadence per company tier |
| `AGG_INTERVAL` | `5m` | Aggregator cadence (file re-downloaded only when its commit changes) |
| `WORKERS` | `8` | Bounded concurrency across all hosts |
| `HOST_MIN_GAP` | `300ms` | Minimum gap between requests to the same host |
| `US_ONLY` | `true` | Drop non-US postings |
| `ALLOW_PHD` | `false` | Include PhD-targeted roles |
| `NOTIFY_CAP` | `25` | Max alerts per company per cycle; overflow summarized |
| `HEALTHCHECK_URL` | — | Optional dead-man's-switch pinged every minute |
| `API_ADDR` | `127.0.0.1:8080` | HTTP API bind address; `""` disables it |
| `SCOPE_GROUPS` / `SCOPE_TIERS` / `SCOPE_ONLY` / `SCOPE_EXCLUDE` | — | Default run scope (flags override) |

## Run scoping

The daemon, `scrape-once`, and `verify` share scoping flags (they intersect;
`--exclude` applies last), which also read the `SCOPE_*` env vars above:

```bash
go run ./cmd/internmaxx --groups ai,quant     # by category / alias group
go run ./cmd/internmaxx --tiers 1,2           # only tiers 1 and 2
go run ./cmd/internmaxx --only openai,janestreet
go run ./cmd/internmaxx --exclude spacex      # drop a noisy board
```

Custom alias groups: convert `companies.yaml` to the mapping form and add a
`groups:` block (see the commented example at the top of the file), then
`--groups dream`.

## HTTP API

Served alongside the daemon when `API_ADDR` is set (localhost by default). There
is **no auth** — bind it to localhost and reach it over an SSH tunnel, or front
it with your own auth.

```
GET    /healthz                   liveness
GET    /jobs                      seen postings, newest first
                                  ?company= &days=N &limit= (≤100) &offset=
                                  → {items, limit, offset, total, next_offset?}
GET    /applications              list (optional ?status=applied)
POST   /applications              {job_key, company, title, url, status, notes}
GET    /applications/{key...}     (key may contain slashes)
PUT    /applications/{key...}     upsert
DELETE /applications/{key...}
GET    /statuses                  saved→applied→oa→phone→onsite→offer/rejected/ghosted
```

`first_seen` is when the scraper first saw a posting (its "new to me" time), not
the ATS-native posted date (which isn't persisted). A freshly-seeded company
therefore shows a wall of identical `first_seen` times — its whole board was
recorded in one seeding pass.

## Browsing: `imx` (CLI) and `imxtui` (TUI)

Both are read-only clients for `/jobs` sharing `internal/jobsclient`. They
resolve the server address from `--addr` > `IMX_API_ADDR` > `API_ADDR` >
`http://127.0.0.1:8080`, so a shell that sourced the daemon's `.env` just works;
over SSH, tunnel and point `IMX_API_ADDR` at the forwarded port.

**`imx`** — scriptable one-shot queries (add `--json` for raw output):

```bash
go run ./cmd/imx recent --days 7            # newest across all companies, paged
go run ./cmd/imx recent --days 7 --page 2   # next page (--limit sets page size)
go run ./cmd/imx company openai             # one company (slug or display name)
go run ./cmd/imx company "Jane Street" --json
```

**`imxtui`** — a live, auto-refreshing feed over the same API, newest first,
with arrivals since the last poll highlighted:

```bash
go run ./cmd/imxtui --days 7 --refresh 15s
```

| Key | Action |
|---|---|
| `↑`/`k`, `↓`/`j`, `g`/`G` | navigate |
| `y` | yank the selected URL to your **local** clipboard (OSC 52) |
| `/` | filter by company (slug or name) |
| `r` | refresh · `d` toggle days window · `?` help · `q` quit |

The `y` yank works even when `imxtui` runs on the VM: the OSC 52 escape travels
back over the SSH pty to your terminal's clipboard. Terminals like
iTerm2/kitty/WezTerm/Ghostty support it; inside tmux add `set -g set-clipboard on`.
The full URL is always shown in the detail pane as a fallback.

## Seeding

The first time a source/company (or an aggregator) is scraped, every current
posting is recorded as **already-notified and nothing is sent** — otherwise
adding the whole registry would fire ~15k messages at once. Only postings that
appear *after* seeding trigger alerts. The state lives in `DB_PATH`; keep it
across restarts. A crash-restart re-reads it and re-sends nothing it already
knows. Delete the DB only if you intend to re-seed (and suppress one cycle).

## Deploy

The app is one static binary + `companies.yaml` + a SQLite file, so any Linux
host with SSH works (see [`../deploy/`](../deploy) and `PLAN.md §8`). Ongoing
updates are scripted:

```bash
./deploy/redeploy.sh                 # build linux/amd64, ship, restart the service
HOST=root@1.2.3.4 ./deploy/redeploy.sh   # override the target host
```

`redeploy.sh` cross-compiles and ships all three binaries (`internmaxx`, `imx`,
`imxtui`) plus `companies.yaml`, then restarts the systemd unit (which also
applies any new DB indexes on startup). Once deployed, browse the VM's data
without a tunnel — the tools run on the box against its localhost API:

```bash
./deploy/imx.sh recent --days 7      # run imx over SSH
./deploy/imxtui.sh                   # run the TUI over ssh -t (needs a pty)
```

Both wrappers forward your local timezone so `first_seen` renders in local time.

## Tests

```bash
go test ./...        # unit tests (store, jobsclient, imxtui, filter)
```

Scraper `*_live_test.go` files hit real boards and are best run selectively
(`go test ./internal/scraper -run TestGoogleLive`), not in CI.
