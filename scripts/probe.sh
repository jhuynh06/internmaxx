#!/bin/bash
# Probe public ATS endpoints for candidate slugs. Output CSV: company,ats,slug,http_code
SCRATCH="$(cd "$(dirname "$0")" && pwd)"
IN="$SCRATCH/candidates.txt"
OUT="$SCRATCH/results.csv"
JOBS="$SCRATCH/probe_jobs.txt"

: > "$JOBS"
while IFS='|' read -r name slugs; do
  [[ "$name" =~ ^#.*$ || -z "$name" || "$slugs" == "skipme" ]] && continue
  IFS=',' read -ra arr <<< "$slugs"
  for slug in "${arr[@]}"; do
    echo "$name|$slug" >> "$JOBS"
  done
done < "$IN"

probe_one() {
  local name="$1" slug="$2"
  local gh ash lev sr
  gh=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 "https://boards-api.greenhouse.io/v1/boards/$slug/jobs")
  ash=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 "https://api.ashbyhq.com/posting-api/job-board/$slug")
  lev=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 "https://api.lever.co/v0/postings/$slug?mode=json&limit=1")
  sr=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 "https://api.smartrecruiters.com/v1/companies/$slug/postings?limit=1")
  echo "$name,greenhouse,$slug,$gh"
  echo "$name,ashby,$slug,$ash"
  echo "$name,lever,$slug,$lev"
  echo "$name,smartrecruiters,$slug,$sr"
}
export -f probe_one

: > "$OUT"
# 8-way parallel over companies; 4 sequential requests each keeps per-host politeness
xargs -P 8 -I{} bash -c 'IFS="|" read -r n s <<< "{}"; probe_one "$n" "$s"' < "$JOBS" >> "$OUT"

echo "--- hits ---"
grep ",200$" "$OUT" | sort | uniq
echo "--- total probed: $(wc -l < "$JOBS") slugs ---"
