#!/usr/bin/env bash
# One-call redeploy to the default VM. Wraps deploy/deploy.sh (which builds the
# static linux binary, ships it + companies.yaml, and restarts the service).
#
# Target host resolves as: HOST env var > the gitignored deploy/.host file >
# the placeholder below. Put your VM in deploy/.host (e.g. `root@1.2.3.4`) once,
# and it stays out of git.
#
# Usage:
#   ./deploy/redeploy.sh                       # host from deploy/.host
#   HOST=root@1.2.3.4 ./deploy/redeploy.sh     # override the target
set -euo pipefail

export HOST="$("$(dirname "$0")/resolve-host.sh")"
echo "==> redeploying to $HOST"
exec "$(dirname "$0")/deploy.sh"
