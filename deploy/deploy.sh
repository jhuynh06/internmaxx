#!/usr/bin/env bash
# Repeatable deploy: cross-compile a static linux binary and push it + the
# registry to a host running the internmaxx systemd service. Provider-agnostic
# — works for a DigitalOcean Droplet, an Azure VM, or anything with SSH.
#
# One-time host setup is in deploy/README.md. This script only handles ongoing
# updates (new binary / new companies.yaml).
#
# Usage:
#   HOST=root@203.0.113.10 ./deploy/deploy.sh
set -euo pipefail

: "${HOST:?set HOST=user@ip (e.g. HOST=root@203.0.113.10)}"
APP_DIR="${APP_DIR:-/opt/internmaxx}"

cd "$(dirname "$0")/../backend"

echo "==> building static linux/amd64 binaries"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/internmaxx.build ./cmd/internmaxx
# imx / imxtui: the read-only browse CLI + TUI, so `./deploy/imx.sh` and
# `./deploy/imxtui.sh` can run them on the box (where they reach the
# localhost-bound /jobs API without a tunnel).
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/imx.build ./cmd/imx
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/imxtui.build ./cmd/imxtui

echo "==> copying to $HOST:$APP_DIR"
# Copy to temp names first; moving over a running binary avoids "text file busy".
scp /tmp/internmaxx.build "$HOST:/tmp/internmaxx.new"
scp /tmp/imx.build "$HOST:/tmp/imx.new"
scp /tmp/imxtui.build "$HOST:/tmp/imxtui.new"
scp companies.yaml "$HOST:/tmp/companies.yaml.new"

echo "==> installing + restarting service"
ssh "$HOST" "sudo install -m755 /tmp/internmaxx.new $APP_DIR/internmaxx \
  && sudo install -m755 /tmp/imx.new $APP_DIR/imx \
  && sudo install -m755 /tmp/imxtui.new $APP_DIR/imxtui \
  && sudo install -m644 /tmp/companies.yaml.new $APP_DIR/companies.yaml \
  && rm -f /tmp/internmaxx.new /tmp/imx.new /tmp/imxtui.new /tmp/companies.yaml.new \
  && sudo systemctl restart internmaxx \
  && sleep 2 && sudo systemctl --no-pager --lines=8 status internmaxx"

rm -f /tmp/internmaxx.build /tmp/imx.build /tmp/imxtui.build
echo "==> done"
