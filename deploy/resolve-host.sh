#!/usr/bin/env bash
# Prints the deploy target host (user@host) for the other deploy scripts.
# Resolution order: HOST env var > the gitignored deploy/.host file > error.
#
# Keep your VM out of git: put `user@ip` (one line) in deploy/.host.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"

if [ -n "${HOST:-}" ]; then
  printf '%s\n' "$HOST"
  exit 0
fi

if [ -f "$here/.host" ]; then
  # first non-comment, non-blank line, whitespace stripped
  h="$(sed -e 's/#.*//' -e '/^[[:space:]]*$/d' "$here/.host" | head -n1 | tr -d '[:space:]')"
  if [ -n "$h" ]; then
    printf '%s\n' "$h"
    exit 0
  fi
fi

echo "deploy: no target host. Create deploy/.host with 'user@ip', or set HOST=user@ip." >&2
exit 1
