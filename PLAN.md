# internmaxx — Build Plan & Reference

Goal: match (and beat) the Pathway pitch — detect engineering/SWE internship postings within
minutes of going live, send instant alerts for companies you care about, track applications,
and keep quality high (curated companies, no spam). Backend-first; UI comes later.

Target roles: SWE / engineering internships and co-ops (software, ML/AI, data, infra, quant dev,
embedded/EE-adjacent). Keep the existing Go structure (`backend/cmd`, `backend/internal/{scraper,models,discord}`)
and grow it — no rewrite.

> **2026-07-08:** ~200 company boards were probe-verified against the live public ATS APIs and
> seeded into `backend/companies.yaml`. Section 4 reflects verified reality, not guesses.

---

## 1. Current state

- `internal/scraper/scrape.go` — Ashby + Greenhouse fetchers, shared title-regex intern filter.
- `internal/models/job.go` — canonical `Job` + per-ATS raw types with `ToJob()` converters. This
  pattern (raw ATS struct → `ToJob()`) is good; every new source follows it.
- `internal/discord/webhook.go` — embed batching (10/message) works.
- `cmd/main.go` — one-shot run for a single hardcoded company.
- `backend/companies.yaml` — verified seed registry (~170 entries, tiered).

Immediate fixes before building further:

1. **Rotate the Discord webhook** — the ID/secret are hardcoded and committed in `webhook.go`.
   Move to env vars (`DISCORD_WEBHOOK_ID`, `DISCORD_WEBHOOK_SECRET`), rotate the old one in
   Discord's settings.
2. Greenhouse `updated_at` is *not* the posted date — it changes on any edit. Prefer
   `first_published` (present in the boards API response) and fall back to `updated_at`.
3. Ashby gives `employmentType` — treat `employmentType == "Intern"` as a strong positive signal
   alongside the title regex (catches titles like "Software Engineer, University Program").

---

## 2. Architecture

```
                 ┌────────────────────────────────────────────┐
 companies.yaml ─┤  Scheduler (per-tier tickers + jitter)      │
                 │    └─ worker pool (bounded concurrency)     │
                 │         └─ Scraper (per-ATS implementation) │
                 │              └─ filter (intern + SWE + geo) │
                 │                   └─ Store (SQLite dedup)   │
                 │                        └─ Notifier fan-out  │
                 │                            ├─ Discord       │
                 │                            └─ Email (later) │
                 └────────────────────────────────────────────┘
```

New packages under `backend/internal/`:

| Package     | Responsibility |
|-------------|----------------|
| `registry`  | Load `companies.yaml` (name, ats, slug, tier, group, overrides) + apply run-scope filters |
| `scraper`   | One file per source: `ashby.go`, `greenhouse.go`, `lever.go`, `workday.go`, `smartrecruiters.go`, `simplify.go`, custom one-offs |
| `filter`    | Intern detection, SWE-role classification, negative filters, location/season filters |
| `store`     | SQLite: seen-job dedup, first_seen timestamps, (later) application tracking |
| `notify`    | `Notifier` interface; `discord` becomes one implementation, email another |
| `scheduler` | Tier-based tickers, jitter, backoff, per-host rate limits |

### Scraper interface

```go
type Scraper interface {
    // Source is the ATS/aggregator key used in companies.yaml ("ashby", "greenhouse", ...)
    Source() string
    Fetch(ctx context.Context, client *http.Client, c registry.Company) ([]models.Job, error)
}
```

Registry maps `ats:` string → implementation. Adding a source = one file + one map entry.

### Run scoping — choose which companies a run polls

Every `companies.yaml` entry has a `group` (already assigned: `ai`, `quant`, `fintech`,
`software`, `entertainment`, `aero`, `health`). Scoping lives in `registry` as a filter applied
right after load, so the daemon, `cmd/scrape-once`, and `cmd/verify` all share it:

```
internmaxx --groups ai,quant            # only these groups
internmaxx --tiers 1,2                  # only these tiers
internmaxx --only openai,janestreet     # explicit allowlist by slug
internmaxx --exclude spacex             # drop noisy boards without editing yaml
```

- Flags combine as intersection (`--groups quant --tiers 1` = tier-1 quant only); `--exclude`
  is applied last. Same values accepted via env (`SCOPE_GROUPS=...`) for the deployed daemon.
- Custom groupings beyond the built-in categories: a `groups:` alias block at the top of the
  yaml, e.g. `dream: [openai, anthropic, janestreet, figma, ramp]` — an alias expands to a slug
  list, so `--groups dream` works without retagging entries. This is how "companies I'm
  actively targeting this season" stays separate from the category taxonomy.
- `cmd/scrape-once --only <slug>` becomes the standard way to test a new company before
  committing it to the registry.

### Dedup & the no-spam rule (critical)

SQLite table:

```sql
CREATE TABLE seen_jobs (
    key         TEXT PRIMARY KEY,  -- source + "/" + company + "/" + (ats job id, else canonical URL)
    company     TEXT NOT NULL,
    title       TEXT NOT NULL,
    url         TEXT NOT NULL,
    first_seen  TEXT NOT NULL,     -- RFC3339
    notified_at TEXT               -- NULL until alert sent
);
```

- Prefer the ATS's own job ID as the key (Ashby `id`, Greenhouse `id`, Lever `id`,
  SmartRecruiters `id`, Workday req id). URL as fallback. Title alone is not unique (your
  `job.go` comment already notes this).
- **Seeding mode:** the first ever scrape of a company inserts everything as seen with
  `notified_at = first_seen` but sends *no* notifications. Only deltas after seeding alert.
  This is what prevents a 200-message blast every time you add a company. (Non-negotiable now:
  the seeded registry alone covers ~15,000 live postings — SpaceX has 1,859, Anduril 2,164.)
- A job disappearing and reappearing (board glitch) must not re-alert: key already exists.

### Scheduling & frequency

Per-company `tier` in `companies.yaml`, each tier a ticker:

| Tier | Who | Interval (start) | Floor (aggressive) |
|------|-----|------------------|--------------------|
| 1 | dream companies (~50 seeded) | 5 min | 2 min |
| 2 | strong companies (~90 seeded) | 15 min | 5 min |
| 3 | long tail (~40 seeded) | 60 min | 30 min |
| aggregators | Simplify/vanshb03/cvrve JSON | 5 min | 2–3 min |

- Add ±20% jitter per request so you don't fire synchronized bursts at one ATS host.
- Per-host rate limit (e.g. max 2 concurrent + 300ms spacing to `boards-api.greenhouse.io`) —
  these public endpoints are unauthenticated but will 429/ban abusive clients. ~125 of the
  seeded companies are on Greenhouse, so tier ticks must queue through the per-host limiter,
  not fire all at once.
- Exponential backoff per company on errors (429/5xx): 2× interval up to 24h; 404 three times
  in a row → mark slug `broken` in the DB and log for manual review (companies migrate ATS —
  the probe found four migrations in one pass, see §4 gotchas).
- Aggregator polling is the cheap high-leverage path: Simplify scrapes top companies hourly and
  the JSON is one HTTP GET covering thousands of listings. Use conditional requests
  (`If-None-Match` ETag) or check the latest commit via GitHub API first — the raw file is
  10MB+, don't re-download it unchanged.
- Notification cap per cycle (e.g. 25 jobs); overflow goes into one digest message. Instant
  alerts should stay instant *because* they're rare.

---

## 3. Source implementations (endpoint reference)

### Already done
| ATS | Endpoint |
|-----|----------|
| Ashby | `GET https://api.ashbyhq.com/posting-api/job-board/{slug}?includeCompensation=true` |
| Greenhouse | `GET https://boards-api.greenhouse.io/v1/boards/{slug}/jobs?content=true` |

### Phase 1 — public JSON, same shape of work as what you have
| ATS | Endpoint | Notes |
|-----|----------|-------|
| **Lever** | `GET https://api.lever.co/v0/postings/{slug}?mode=json` | Supports `commitment`, `team`, `location` query filters at the source; `createdAt` is epoch ms. Verified boards: Palantir, Spotify, Zoox, Shield AI, Mistral, Hermeus, Anchorage, Wealthfront, Ro, Belvedere, Valkyrie |
| **SmartRecruiters** | `GET https://api.smartrecruiters.com/v1/companies/{slug}/postings?limit=100&offset=0` | ⚠️ **Returns 200 + empty for ANY slug** — validity check must be `totalFound > 0`, not HTTP status. Probe of ~100 companies found only ServiceNow (412), Canva (212), Visa, Uber, Glean actually publishing here. Niche source, implement but don't expect volume |
| ~~Workable~~ / ~~Recruitee~~ | — | Deprioritized: zero target companies verified on them; revisit only if a wanted company shows up there |

### Phase 2 — Workday (biggest coverage jump: banks, entertainment, chipmakers)
`POST https://{tenant}.{instance}.myworkdayjobs.com/wday/cxs/{tenant}/{site}/jobs`
with JSON body `{"appliedFacets":{},"limit":20,"offset":0,"searchText":"intern"}`.
- Each company needs three config values: `tenant`, `instance` (wd1/wd3/wd5/wd12…), `site`.
  **Discovery method:** open the company's careers page, click any job — the URL becomes
  `https://{tenant}.{instance}.myworkdayjobs.com/en-US/{site}/job/...`. Record all three in
  `companies.yaml` as `workday: {tenant, instance, site}`. (Example — NVIDIA:
  `nvidia` / `wd5` / `NVIDIAExternalCareerSite`.)
- `searchText=intern` server-side cuts response size massively for 10k-job boards.
- Target sets (find tenant per method above; none are probe-verifiable without it):
  - **Chips/hardware:** NVIDIA, AMD, Intel, Qualcomm, Broadcom, Micron, Texas Instruments,
    Analog Devices, Marvell, Arm, Dell, HP/HPE, Western Digital, Seagate
  - **Banks/finance (Workday):** Capital One, Discover, Fidelity, Vanguard, BlackRock,
    Charles Schwab, State Street, BNY, PNC, U.S. Bank, Truist, RBC, TD, Wells Fargo,
    Nasdaq, CME Group, ICE/NYSE, S&P Global, Moody's, MSCI, FactSet, Fannie Mae, Freddie Mac
  - **Entertainment/media:** Disney (tenant `disney`), NBCUniversal/Comcast, Warner Bros.
    Discovery, Paramount, Sony Interactive (PlayStation), Nintendo of America, Live Nation/
    Ticketmaster, SiriusXM, Electronic Arts, Unity, Snap
  - **Enterprise software:** Salesforce, Adobe, Workday, Intuit, Splunk, CrowdStrike, Box,
    Nutanix, VMware/Broadcom
  - **Defense/aero primes:** Lockheed Martin, Northrop Grumman, RTX, Boeing, L3Harris,
    General Dynamics, BAE US, Leidos, Booz Allen, SAIC, Blue Origin (moved off Lever — its
    Lever board is live but empty), Boston Dynamics
  - **Consulting/other:** Accenture, Deloitte, Mastercard, PayPal, Expedia, Zillow, Target,
    Home Depot

### Phase 3 — custom career sites (one-off scrapers, high value)
Big tech (all confirmed NOT on any public ATS API):
| Company | Approach |
|---------|----------|
| Google | old `careers.google.com/api/v3` is dead (404 — verified). Current site is `google.com/about/careers/applications/jobs/results?q=intern`; JSON API behind it, inspect network tab |
| Amazon | `amazon.jobs/en/search.json?base_query=intern&category=software-development` — **verified 200** |
| Microsoft | `gcsservices.careers.microsoft.com/search/api/v1/search?q=intern` — times out for bare curl (verified); needs browser-like headers (Origin/Referer from careers.microsoft.com) |
| Meta | metacareers.com GraphQL (POST with doc_id; inspect network tab) |
| Apple | `jobs.apple.com/api/role/search` (POST JSON) |
| Netflix | Eightfold API `explore.jobs.netflix.net/api/apply/v2/jobs?domain=netflix.com&query=intern` — **verified 200** |
| Uber | `uber.com/api/loadSearchJobsResults` (POST) — SmartRecruiters board exists but has 1 posting |
| Tesla | `tesla.com/cua-api/apps/careers/state` — 403 for bare curl (verified); bot protection, needs full browser headers |
| TikTok/ByteDance | lifeattiktok.com internal API |
| LinkedIn, IBM, Oracle, Cisco, Shopify, GitHub, Atlassian | own sites (Atlassian's Lever board is live but empty — don't trust it) |

Banks with custom sites (not Workday):
| Company | Notes |
|---------|-------|
| Goldman Sachs | higher.gs.com (own platform, JSON-backed search) |
| JPMorgan Chase | Oracle Recruiting Cloud: `jpmc.fa.oraclecloud.com` — ORC has a semi-public REST layer (`/hcmRestApi/resources/latest/recruitingCEJobRequisitions`); one generic ORC scraper covers JPMC + other ORC tenants |
| Morgan Stanley | own (Eightfold-backed) |
| Bank of America | own careers site |
| Citi | own (`jobs.citi.com`, Phenom-backed — Phenom sites expose a JSON search endpoint, one generic scraper covers many Phenom tenants) |
| American Express | own (Phenom) |
| Barclays / UBS / Deutsche Bank / HSBC | own / SAP SuccessFactors |
| Bloomberg | `bloomberg.com/careers` (Avature-backed, JSON search) |

Quant firms with custom sites (the ones NOT on Greenhouse — verified absent in probe):
Citadel & Citadel Securities (GraphQL on own site), DE Shaw, Two Sigma, DRW, SIG/Susquehanna,
Millennium, Balyasny, XTX Markets, G-Research, Headlands, Chicago Trading Company, Radix,
PEAK6, Wolverine, GTS, Voloridge, Tower's non-GH roles. Most post intern roles once a year in
Aug–Sep — worth one-off scrapers for the big five (Citadel, DE Shaw, Two Sigma, DRW, SIG).

### Aggregators (do these FIRST — cheapest wins)
| Source | How | Freshness |
|--------|-----|-----------|
| **SimplifyJobs/Summer2026-Internships** | `https://raw.githubusercontent.com/SimplifyJobs/Summer2026-Internships/dev/.github/scripts/listings.json` — fields: `id`, `company_name`, `title`, `locations`, `url`, `date_posted`, `date_updated`, `active`, `sponsorship`, `terms`, `source`. File is 10MB+; poll the GitHub commits API for the `dev` branch first and only download on new commit. Stream-decode. | Simplify auto-scrapes top companies hourly; community PRs continuously |
| **vanshb03/Summer2026-Internships** | Same repo structure (fork lineage), same `listings.json` path — verify on `dev` branch. Maintained with CSCareers; catches things Simplify misses. Also `Summer2027-Internships` when the season flips. | daily+ |
| **cvrve Summer 2026 repo** | Same format family; third overlap source | daily+ |
| **speedyapply/2026-SWE-College-Jobs** | GitHub repo, structured README + JSON | daily |
| **jobright.ai/minisites-jobs/intern/us/swe** | JS-rendered; data comes from an internal API (inspect network calls). Lower priority — mostly re-lists what the ATS scrapers catch, and is scrape-hostile. | hourly-ish |
| **hiring.cafe** | Has a public-ish search API aggregating direct ATS postings; good discovery source for companies to add to the registry | continuous |
| **SWEList / intern-list.com / levels.fyi internships** | Discovery/cross-check only | — |

Aggregator jobs get lower notification priority than direct ATS hits (direct = minutes-fresh;
aggregator = catch-all + discovery of companies you don't poll yet). When an aggregator surfaces
a company not in the registry → log it as a candidate to add. **This is also how the big-tech
customs are covered until Phase 3 ships** — Simplify tracks Google/Amazon/Meta/Microsoft, so
you're never blind on them, just ~1h slower.

---

## 4. Company registry — VERIFIED (probe run 2026-07-08)

`backend/companies.yaml` holds the live registry: ~170 entries, every one confirmed against
its public ATS endpoint (HTTP 200 + real board identity + job count). Highlights by category:

- **AI:** OpenAI (Ashby, 710 live jobs), Anthropic (GH, 404), xAI (GH, 214), Mistral (Lever,
  170), Cohere (Ashby, 130), Perplexity, Cursor (Ashby, 113), Cognition, Thinking Machines
  (GH), SSI (Ashby), Scale, Together, CoreWeave (GH, 276), Cerebras, ElevenLabs, Harvey
  (Ashby, 333), Sierra, plus ~30 more AI infra/product startups.
- **Quant on public boards:** Jane Street (GH `janestreet`, 217!), HRT (GH `wehrtyou`), Jump,
  IMC, Optiver US, Point72, Akuna, Five Rings, Tower, Virtu, Flow Traders, AQR, WorldQuant,
  PDT, Squarepoint, Schonfeld, Old Mission, Belvedere, DV, Geneva, TransMarket, 3Red, Voleon.
- **Fintech/crypto:** Stripe (GH, 502), Ramp, Plaid (Ashby now!), Coinbase, Robinhood, Block,
  Brex, Mercury, Chime, Affirm, SoFi, Carta, Adyen, Gemini, Ripple, Nubank, + long tail.
- **Big software:** Airbnb, Databricks (GH, 786), Snowflake (Ashby now, 418), Figma, Notion,
  Datadog, Cloudflare, Reddit, Discord, Roblox, Pinterest, DoorDash, MongoDB, GitLab, Twilio,
  Okta, Zscaler, Palantir (Lever, 270), Spotify (Lever), ServiceNow + Canva (SmartRecruiters).
- **Gaming/entertainment:** Epic (GH, 127), Riot (GH, 162), Rockstar, Take-Two, Roku, Twitch,
  NYT, Axios, Vox.
- **Aero/defense/autonomy:** SpaceX (GH, 1859), Anduril (GH, 2164), Waymo (GH, 374), Zoox
  (Lever, 217), Neuralink, Shield AI (Lever, 406), Nuro, Aurora, Skydio, Saronic (Ashby, 275),
  Rocket Lab (363), Relativity, Astranis, Varda, Zipline, Hermeus, Boom, Epirus.

### Gotchas the probe caught (encode these as tests/comments)

1. **Slug collisions are real:** GH `figure` = Figure *Lending*; the robotics company is
   `figureai`. GH `purestorage` is a board named "Everpure" (not Pure Storage). GH `optiver`
   is an abandoned empty board; the live one is `optiverus`. Never trust a 200 alone — the
   `cmd/verify` tool must compare the board's `name` field against the expected company name.
2. **SmartRecruiters returns 200 for nonexistent companies** (empty result set). Validity =
   `totalFound > 0`.
3. **Migrations found in one probe:** Anduril Lever→Greenhouse, Plaid Lever→Ashby, Snowflake
   GH→Ashby, Blue Origin Lever→(Workday), Spotify →Lever. Several companies keep *both* old
   and new boards live during migration (Nubank on GH + Ashby, Verkada/Vercel/Reddit have
   empty Ashby boards + live GH boards). Prefer the board with jobs; `cmd/verify` should flag
   any registry entry whose board goes to 0 jobs while a sibling board has >0.
4. **Empty-but-live boards worth keeping:** SSI, Wiz (Ashby), Niantic — the board exists with
   0 public postings; keep at low tier so the first posting triggers an alert.

### Not reachable via public ATS APIs (→ Workday phase 2 / custom phase 3)

Confirmed absent from Greenhouse/Ashby/Lever/SmartRecruiters public APIs in the probe:
Google, Amazon, Meta, Apple, Microsoft, Netflix, Tesla, TikTok, Snap, EA, Unity, Shopify,
GitHub, Atlassian, Salesforce, Adobe, Intuit, Splunk, CrowdStrike, Nutanix, Box, HashiCorp
(moved post-IBM), Grammarly, Rippling, Retool, Whatnot, Klarna, Groq, Applied Intuition,
Luminar, Boston Dynamics, Firefly, Stoke, Sierra Space, Axiom, Castelion, Hadrian, Hugging
Face, Weights & Biases, Luma, all bulge-bracket banks, and the custom-site quant firms listed
in §3. Coverage path: aggregators immediately → Workday/custom scrapers in M4/M5.

### Growing the list

The aggregator feeds are the discovery mechanism — every listing whose company isn't in the
registry gets logged; review that log weekly, find the company's ATS (careers page → where the
"Apply" button points), verify with `cmd/verify`, add to YAML. The probe scripts that built the
seed list are checked in under `scripts/` (`candidates.txt` + `probe.sh` 4-endpoint curl probe +
`verify.sh` identity/count pass, plus the raw `results.csv`/`verified.txt` from the 2026-07-08
run) — port to Go as `cmd/discover` when convenient.

---

## 5. Filtering (quality > quantity)

Layered, in `internal/filter`:

1. **Intern signal** (have): title regex `intern|co-op|apprentice|trainee|student` — keep, plus
   ATS-native fields (Ashby `employmentType == "Intern"`, Lever `commitment` contains
   "Intern", SmartRecruiters `typeOfEmployment`).
2. **Role classification** (new): is it SWE/eng? Positive title terms: `software, engineer,
   developer, SWE, SDE, backend, frontend, full.?stack, mobile, iOS, android, data, machine
   learning, ML, AI, infrastructure, platform, DevOps, SRE, security, embedded, firmware,
   systems, compiler, research, quant`. Tag each job with a category so alerts can say what it is.
3. **Negative filters** (new): drop `marketing, sales, recruiting, HR, legal, finance intern
   (non-quant), design (unless wanted), MBA, PhD-only` (make PhD a config flag). Drop
   `unpaid` if detectable.
4. **Location/eligibility** (config): default US + Remote-US; parse Ashby region/country,
   Greenhouse `location.name`, Simplify `locations` + `sponsorship` field.
5. **Season/date**: drop postings older than N days on first sight via aggregators (stale
   re-lists); ATS-direct jobs are new by construction of the dedup.
6. **Quality gate**: only registry companies alert at full priority; unknown-company aggregator
   hits go to a separate low-priority digest channel. This is the "no spam/random companies"
   guarantee.

Keep filters as pure functions over `models.Job` → unit-testable with a fixtures file of real
titles (build the fixture list from a Simplify listings.json snapshot).

## 6. Notifications

- `Notifier` interface: `Notify(ctx, []models.Job) error`. Discord is impl #1 (exists —
  parameterize the webhook via env).
- Discord rate limit: ~30 msgs/min/webhook — the 10-embed batching already helps; add a
  token-bucket just in case.
- Channel routing: tier-1 direct hits → `#alerts` (ping @everyone or a role), everything else →
  `#digest`.
- **Email (Pathway parity, later phase)**: Resend (free tier 100/day, trivial API) or AWS SES.
  Requires subscriptions: `subscriptions(email, company_pattern, role_category)` table +
  a tiny HTTP API to manage them. Only build once Discord flow is proven.

## 7. Application tracking (later, backend-only for now)

Tables: `applications(job_key, status, applied_at, notes, updated_at)` with status enum
`saved → applied → oa → phone → onsite → offer/rejected/ghosted`. Expose via small HTTP API
(`net/http`, JSON) so any future UI (or curl) works. Unlimited by construction — it's your DB.

## 8. Ops

- Config: env vars via a single `internal/config` loader (webhook creds, DB path, tier
  intervals, filter flags). `.env` in `.gitignore`.
- `cmd/main.go` becomes the long-running daemon; add `cmd/scrape-once` (current behavior, for
  testing) and `cmd/verify` (registry checker: endpoint 200 + board name matches + job count,
  catches the §4 gotchas).
- Structured logging (`log/slog`): every cycle logs per-company {jobs_seen, new, notified, errs}.
- Deploy: it's one binary + one SQLite file, so any always-on Linux box works. A dead-man's-
  switch ping (healthchecks.io free) alerts you if the daemon stops — an alert system that
  silently dies is worse than none.

### Deploying

The app is a static binary + one SQLite file, so any Linux VM works and moving
between providers is a file copy. **`deploy/README.md` has the concrete
DigitalOcean walkthrough** (Droplet create → systemd → `deploy/deploy.sh` push)
plus a migration section (DO → another Droplet or → Azure). DO credits (e.g. the
$200 Student Pack) apply automatically to the Droplet cost. Azure-specific notes:

#### Deploying on Azure for Students (free school credit)

Azure for Students gives $100/year credit, no credit card, renewable annually while enrolled,
plus the standard free-tier services. For a 24/7 poller:

- **Recommended: one B1s Linux VM (Ubuntu LTS).** B1s (1 vCPU / 1 GB) is in the free 750
  hrs/month tier for the first 12 months — effectively $0; after that it's ~$8–9/mo, or drop
  to B1ls (~$3.80/mo), which is plenty for a Go daemon. Egress for JSON polling is pennies.
  Setup: `az vm create` → `scp` the linux/amd64 binary + `companies.yaml` → systemd unit with
  `Restart=always` and an `EnvironmentFile` for the webhook creds → SQLite lives on the OS
  disk (back it up with a nightly cron `sqlite3 .backup` to Azure Blob — student credit
  includes plenty of storage).
- **Avoid the tempting serverless routes:** App Service F1 has no Always-On (daemon gets
  killed); Container Apps scale-to-zero defeats a poller, and pinning `minReplicas: 1` burns
  through the monthly free grant. Azure Functions timer-triggers *could* replace the scheduler
  (cron every 5 min), but then SQLite must move to Azure Table Storage/Files and the tier
  system gets awkward — a rearchitecture for no benefit at this scale.
- Two VM gotchas: don't enable the portal's auto-shutdown "cost saving" suggestion (it kills
  the daemon nightly), and set a budget alert at ~$8/mo in Cost Management so a mistake never
  silently drains the credit.
- Deploy loop: a GitHub Action on push to main that `go build`s and `scp`s + restarts the
  systemd unit over SSH is all this project needs.

## 9. Milestones

> **Status (2026-07-09):** M1–M7 are **built and validated**, deployed on a
> DigitalOcean droplet. Sources live: Ashby, Greenhouse, Lever, SmartRecruiters,
> Workday (38 banks/chips/entertainment/defense cos), Amazon, Netflix, Oracle ORC
> (JPMorgan), plus Simplify + vanshb03 aggregators with an untracked-company
> discovery log. Filtering, seeding-mode dedup, per-company backoff, per-host rate
> limiting, and an application-tracking HTTP API (no email — descoped by user) all
> verified. Deploy docs in `deploy/README.md` (DO + migration). Discord webhook
> has been rotated. Cross-compiles to a static linux/amd64 binary.
>
> Deliberately NOT built (fragile / low-yield): custom scrapers for Google, Apple,
> Meta, Microsoft, Tesla, TikTok (bot-protection or reverse-engineered tokens) —
> the aggregators already cover these at ~hourly latency. Big-five quant custom
> sites (Citadel/DE Shaw/Two Sigma/DRW/SIG) also deferred.

1. **M1 — Daemonize (do first)** ✅ built: config/env (rotate webhook!), registry loader for the
   already-seeded `companies.yaml` **including run scoping** (`--groups/--tiers/--only/
   --exclude` + `groups:` aliases, see §2), SQLite dedup store with seeding mode, scheduler
   with tiers/jitter + per-host limits, refactor Ashby+Greenhouse behind the `Scraper`
   interface, `cmd/verify`. *Outcome: 24/7 alerts for ~140 verified Ashby/Greenhouse companies
   (incl. OpenAI, Anthropic, Jane Street, SpaceX, Stripe) with zero duplicate pings, scopable
   to any group/slug subset per run.*
1.5. **M1.5 — Ship it**: deploy to an Azure for Students B1s VM per §8 (systemd + env file +
   healthchecks.io ping + nightly SQLite backup to Blob). Do this before adding more sources —
   a scraper that only runs when your laptop is open misses the 15-minute window this whole
   project exists for.
2. **M2 — Lever + SmartRecruiters** ✅ built: both fetchers live; verified against Palantir
   (43 interns), ServiceNow, etc. SmartRecruiters validity is `totalFound`-based and paginated.
3. **M3 — Aggregators** ✅ Simplify built (commit-check-before-download via GitHub API,
   active+visible filter, seeded as its own scope). vanshb03 + cvrve are trivial config clones
   of `scraper.NewSimplify()` (same listings.json format) — add when wanted. Unknown-company
   candidate log still TODO.
4. **M4 — Workday** ✅ built: generic CXS fetcher (searchText=internship) + per-company
   `{tenant, instance, site}`; 38 verified cos across chips/banks/entertainment/defense.
   Coordinates mined from Simplify's Workday URLs.
5. **M5 — Custom sites** ✅ partial: Amazon (US-filtered), Netflix, and a generic Oracle ORC
   scraper (+JPMorgan, per-company `orc:{tenant, site}`). Google/Apple/Meta/Microsoft/Tesla/
   TikTok skipped (bot-protected / token-gated; aggregators cover them). Phenom + big-five
   quant sites deferred.
6. **M6 — Application tracking API** ✅ built (email descoped by user): `applications` table +
   HTTP JSON CRUD (`internal/api`), status pipeline, localhost-bound by default.
7. **M7 — Interval tuning** ✅: separate `AGG_INTERVAL`; conservative defaults with documented
   aggressive floors (tier-1 2m, agg 3m) once 429-free.

## Sources

- Ashby posting API: https://developers.ashbyhq.com/docs/public-job-posting-api
- Greenhouse boards API: https://developers.greenhouse.io/job-board.html
- Lever postings API: https://github.com/lever/postings-api
- SmartRecruiters postings API: https://developers.smartrecruiters.com/docs/read-postings
- ATS platforms with public job APIs: https://cavuno.com/blog/ats-platforms-public-job-posting-apis and https://fantastic.jobs/article/ats-with-api
- SimplifyJobs Summer 2026 repo (listings.json on `dev`): https://github.com/SimplifyJobs/Summer2026-Internships
- vanshb03 Summer 2026 repo: https://github.com/vanshb03/Summer2026-Internships
- vanshb03 Summer 2027 repo (next season): https://github.com/vanshb03/Summer2027-Internships
- Jobright intern SWE minisite: https://jobright.ai/minisites-jobs/intern/us/swe
- Pathway (the competitor pitch this matches): https://www.trypathway.app/
