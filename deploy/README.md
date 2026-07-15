# Deploying internmaxx

The daemon is a single static Linux binary plus one SQLite file. Nothing about
it is provider-specific, so "DigitalOcean now, migrate later" is trivial: copy
two files (three if you want to keep dedup history) to the new box.

## Why this is portable

- **No CGO, no runtime deps** — `CGO_ENABLED=0` static binary (pure-Go SQLite).
- **All state is `internmaxx.db`** — dedup + seeding live in that one file.
- **Config is env vars** — `/opt/internmaxx/.env`, not baked into the binary.

So migration = spin up the new VM, run the one-time setup below, `scp` the DB
over, done. See "Migrating" at the bottom.

---

## 1. Create a DigitalOcean Droplet

Web console: **Create → Droplet → Ubuntu 24.04 LTS → Basic → Regular, $6/mo
(1 GB / 1 vCPU)**. That's plenty; the daemon idles near-zero. Add your SSH key.

Or with `doctl`:

```bash
doctl compute droplet create internmaxx \
  --image ubuntu-24-04-x64 --size s-1vcpu-1gb --region nyc1 \
  --ssh-keys <your-key-fingerprint> --wait
doctl compute droplet list   # note the public IP
```

> Your DO credits (e.g. the $200 GitHub Student Pack credit) apply automatically
> to the Droplet's monthly cost — nothing special to enable. A $6/mo Droplet
> runs ~33 months on $200.

## 2. One-time host setup

From your laptop, with `HOST=root@<droplet-ip>`:

```bash
# copy the systemd unit + a starter env file
scp deploy/internmaxx.service "$HOST:/tmp/"
scp backend/.env.example "$HOST:/tmp/"

ssh "$HOST" '
  sudo mkdir -p /opt/internmaxx
  sudo mv /tmp/internmaxx.service /etc/systemd/system/
  sudo mv /tmp/.env.example /opt/internmaxx/.env
  sudo systemctl daemon-reload
'
```

Now edit the real secrets on the box (put in the rotated Discord webhook):

```bash
ssh "$HOST" sudo nano /opt/internmaxx/.env
```

## 3. Ship the binary + registry, start it

```bash
HOST=root@<droplet-ip> ./deploy/deploy.sh    # builds, copies, restarts
ssh "$HOST" sudo systemctl enable --now internmaxx
ssh "$HOST" journalctl -u internmaxx -f       # watch it seed (0 alerts) then run
```

The first run seeds silently; you'll see `new=0 notified=0` on the seeding pass.
Real alerts start on subsequent cycles.

## 4. Ongoing updates

Any time you change code or `companies.yaml`:

```bash
HOST=root@<droplet-ip> ./deploy/deploy.sh
```

Scope which companies run via `/opt/internmaxx/.env` (`SCOPE_GROUPS=ai,quant`,
`SCOPE_TIERS=1,2`, etc.) or by editing the `ExecStart` flags in the unit.

## 5. Backups

The DB is the only state worth keeping. Easiest options:

- **DO Droplet snapshots** (console → Droplet → Snapshots) — whole-disk, weekly.
- **File copy** to your laptop on a cron:
  ```bash
  ssh "$HOST" "sqlite3 /opt/internmaxx/internmaxx.db 'PRAGMA wal_checkpoint(TRUNCATE);'"
  scp "$HOST:/opt/internmaxx/internmaxx.db" ./backups/internmaxx-$(date +%F).db
  ```

## 6. Dead-man's switch (recommended)

An alerter that dies silently is worse than none. Create a free check at
healthchecks.io, put its ping URL in `.env` as `HEALTHCHECK_URL=...`, and it'll
notify you if the daemon stops pinging.

---

## Migrating (to another Droplet, or to Azure later)

Because state is one file, migration is:

```bash
OLD=root@<old-ip>; NEW=root@<new-ip>

# 1. Do the one-time setup (steps 2–3) on NEW, but DON'T start it yet.
# 2. Stop the old daemon so the DB is quiescent, checkpoint the WAL:
ssh "$OLD" "sudo systemctl stop internmaxx && \
  sudo sqlite3 /opt/internmaxx/internmaxx.db 'PRAGMA wal_checkpoint(TRUNCATE);'"

# 3. Move the DB (preserves all dedup/seeding history → zero missed, zero dup):
scp "$OLD:/opt/internmaxx/internmaxx.db" /tmp/internmaxx.db
scp /tmp/internmaxx.db "$NEW:/tmp/"
ssh "$NEW" "sudo install -m644 /tmp/internmaxx.db /opt/internmaxx/internmaxx.db"

# 4. Start on NEW, decommission OLD.
ssh "$NEW" "sudo systemctl enable --now internmaxx"
```

If you *don't* copy the DB, the new host just re-seeds on first run — that's
**safe** (no spam), you'd only miss deltas that happened during the brief
handover window. So even a lazy migration won't blast you.

The Azure path (Azure for Students B1s VM) is identical from step 2 onward — see
PLAN.md §8 for the Azure-specific VM-creation notes and cost caveats.
