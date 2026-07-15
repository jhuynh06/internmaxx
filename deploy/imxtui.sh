#!/usr/bin/env bash
# Run the imxtui browse TUI on the deployed VM over SSH.
#
# Uses `ssh -t` because a TUI needs a pty. The API binds localhost on the VM, so
# no tunnel is needed. The 'y' yank travels back through the SSH pty via OSC 52
# to your local terminal's clipboard. Requires a prior `./deploy/redeploy.sh`
# (which ships imxtui to $APP_DIR).
#
# Usage:
#   ./deploy/imxtui.sh
#   ./deploy/imxtui.sh --days 7 --refresh 15s
#   HOST=root@1.2.3.4 ./deploy/imxtui.sh
#
# Target host resolves as: HOST env > gitignored deploy/.host > error.
set -euo pipefail

HOST="$("$(dirname "$0")/resolve-host.sh")"
APP_DIR="${APP_DIR:-/opt/internmaxx}"

# Forward the local IANA timezone so first_seen renders in your local time
# (the VM is almost certainly UTC).
tz="$(readlink /etc/localtime 2>/dev/null | sed -E 's#.*/zoneinfo/##')"

remote="cd $(printf '%q' "$APP_DIR") &&"
[ -n "$tz" ] && remote+=" TZ=$(printf '%q' "$tz")"
remote+=" exec ./imxtui"
for arg in "$@"; do
  remote+=" $(printf '%q' "$arg")"
done

exec ssh -t "$HOST" "$remote"
