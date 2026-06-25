#!/usr/bin/env bash
# Produce demo/.seed-state — a real captured sessions.json + scrollback dumps —
# by booting the warm demo, running the save daemon briefly so it dumps every
# pane's scrollback, then snapshotting the state dir. Baked into the cold image
# by Dockerfile.cold. Invoked by build.sh; runnable standalone.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
img="${1:-portal-demo:base}"
name="portal-demo-seedgen-$$"

echo "==> seed-gen container ($img)"
docker rm -f "$name" >/dev/null 2>&1 || true
docker run -d --name "$name" "$img" sleep 120 >/dev/null
trap 'docker rm -f "$name" >/dev/null 2>&1 || true' EXIT

sleep 3   # let the entrypoint create + stamp the 12 warm sessions

# Run the real daemon for a few seconds. It captures sessions.json AND dumps
# each pane's scrollback; it self-ejects after a few ticks (it is not the saver
# pane), which is fine — the dumps are already on disk by then.
echo "==> capturing sessions.json + scrollback"
docker exec "$name" sh -c 'portal state daemon >/dev/null 2>&1 & sleep 5; true'

rm -rf "$here/.seed-state"
mkdir -p "$here/.seed-state"
docker cp "$name:/home/demo/.config/portal/state/sessions.json" "$here/.seed-state/sessions.json"
docker cp "$name:/home/demo/.config/portal/state/scrollback"     "$here/.seed-state/scrollback"

echo "==> seed: $(du -sh "$here/.seed-state" | cut -f1) total, $(find "$here/.seed-state/scrollback" -name '*.bin' | wc -l | tr -d ' ') scrollback files"
