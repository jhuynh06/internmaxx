#!/usr/bin/env bash
# Run the imx browse CLI against the deployed daemon, over SSH.
#
# imx runs ON the VM, where it reaches the localhost-bound /jobs API directly —
# no tunnel needed. Requires a prior `./deploy/redeploy.sh` (which ships imx to
# $APP_DIR alongside the daemon).
#
# Usage:
#   ./deploy/imx.sh recent --days 7
#   ./deploy/imx.sh company openai
#   ./deploy/imx.sh company "Jane Street" --json
#   HOST=root@1.2.3.4 ./deploy/imx.sh recent
#
# Target host resolves as: HOST env > gitignored deploy/.host > error.
set -euo pipefail

HOST="$("$(dirname "$0")/resolve-host.sh")"
APP_DIR="${APP_DIR:-/opt/internmaxx}"

# Forward the local IANA timezone so first_seen renders in your local time
# (the VM is almost certainly UTC). Falls back to the server default if we
# can't resolve it.
tz="$(readlink /etc/localtime 2>/dev/null | sed -E 's#.*/zoneinfo/##')"

# Build a safely-quoted remote command so args with spaces (e.g. a company
# name) survive the extra shell hop.
remote="cd $(printf '%q' "$APP_DIR") &&"
[ -n "$tz" ] && remote+=" TZ=$(printf '%q' "$tz")"
remote+=" ./imx"
for arg in "$@"; do
  remote+=" $(printf '%q' "$arg")"
done

exec ssh "$HOST" "$remote"
