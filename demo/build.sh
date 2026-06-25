#!/usr/bin/env bash
# Build the demo image in two stages:
#   1. portal-demo:base    — cross-compiled linux/arm64 portal + tmux + seed.
#   2. portal-demo:latest  — base + a baked restore seed (sessions.json +
#                            scrollback), captured from the base image.
#
# The demo runs ENTIRELY inside the container. The container's own PID namespace
# means the in-container `pgrep -fx '^portal state daemon'` (bootstrap's orphan
# sweep) cannot see the host's daemon, so recording the demo CANNOT touch the
# host tmux server or its daemon. See demo/README.md for the full safety model.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/.." && pwd)"
ver="${PORTAL_DEMO_VERSION:-0.8.0}"

echo "==> cross-compiling linux/arm64 portal (version=$ver)"
( cd "$repo" && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath \
    -ldflags "-s -w -X github.com/leeovery/portal/cmd.version=$ver" \
    -o "$here/bin/portal" . )
ls -lh "$here/bin/portal"

echo "==> building base image (portal-demo:base)"
docker build -t portal-demo:base "$here"

echo "==> recording restore seed"
bash "$here/record-seed.sh" portal-demo:base

echo "==> building final image (portal-demo:latest = base + restore seed)"
docker build -t portal-demo:latest -f "$here/Dockerfile.cold" "$here"

echo "==> done: portal-demo:latest  (warm: plain run · cold: -e DEMO_COLD=1)"
