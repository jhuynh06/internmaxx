#!/bin/bash
# Pass 2: verify identity + job counts for pass-1 hits.
SCRATCH="$(cd "$(dirname "$0")" && pwd)"
RES="$SCRATCH/results.csv"

echo "=== GREENHOUSE (board name | live jobs) ==="
grep ",greenhouse,.*,200$" "$RES" | cut -d, -f3 | sort -u | while read -r slug; do
  name=$(curl -s --max-time 10 "https://boards-api.greenhouse.io/v1/boards/$slug" | python3 -c "import sys,json;print(json.load(sys.stdin).get('name','?'))" 2>/dev/null)
  cnt=$(curl -s --max-time 15 "https://boards-api.greenhouse.io/v1/boards/$slug/jobs" | python3 -c "import sys,json;print(len(json.load(sys.stdin).get('jobs',[])))" 2>/dev/null)
  echo "gh|$slug|$name|$cnt"
  sleep 0.15
done

echo "=== ASHBY (live jobs) ==="
grep ",ashby,.*,200$" "$RES" | cut -d, -f3 | sort -u | while read -r slug; do
  cnt=$(curl -s --max-time 15 "https://api.ashbyhq.com/posting-api/job-board/$slug" | python3 -c "import sys,json;print(len(json.load(sys.stdin).get('jobs',[])))" 2>/dev/null)
  echo "ashby|$slug|$cnt"
  sleep 0.15
done

echo "=== LEVER (live jobs) ==="
grep ",lever,.*,200$" "$RES" | cut -d, -f3 | sort -u | while read -r slug; do
  cnt=$(curl -s --max-time 15 "https://api.lever.co/v0/postings/$slug?mode=json" | python3 -c "import sys,json;d=json.load(sys.stdin);print(len(d) if isinstance(d,list) else 'ERR')" 2>/dev/null)
  echo "lever|$slug|$cnt"
  sleep 0.15
done

echo "=== SMARTRECRUITERS (company name | totalFound) — only cos with no gh/ashby/lever hit ==="
covered=$(grep -E ",(greenhouse|ashby|lever),.*,200$" "$RES" | cut -d, -f1 | sort -u)
grep ",smartrecruiters,.*,200$" "$RES" | while IFS=, read -r co ats slug code; do
  echo "$covered" | grep -qxF "$co" && continue
  echo "$co|$slug"
done | sort -u | while IFS='|' read -r co slug; do
  out=$(curl -s --max-time 10 "https://api.smartrecruiters.com/v1/companies/$slug/postings?limit=1" | python3 -c "
import sys,json
d=json.load(sys.stdin)
tf=d.get('totalFound',0)
name=(d.get('content') or [{}])[0].get('company',{}).get('name','?') if tf else '?'
print(f'{tf}|{name}')" 2>/dev/null)
  echo "sr|$co|$slug|$out"
  sleep 0.15
done
