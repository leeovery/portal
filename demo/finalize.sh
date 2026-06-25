#!/usr/bin/env bash
# Grade a VHS master recording and emit the delivery formats.
#
# VHS reproduces portal's EXACT sRGB palette (verified pixel-for-pixel), which
# reads muted next to a wide-gamut terminal — Ghostty on a P3 Mac renders the
# same hex values more vividly because it isn't colour-managed. This bakes a
# saturation/contrast boost so the recording matches that on-screen vividness.
#
# Primary deliverable is an animated WebP: true 24-bit colour AND it renders
# inline on GitHub (incl. the iOS app), which a committed <video> does not.
#
# Usage:  finalize.sh <basename>      # e.g. finalize.sh portal-cold
# Input:  demo/out/<basename>.mp4     (the VHS master — run `vhs <tape>` first)
# Output: demo/out/<basename>-vivid.{webp,mp4}
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
name="${1:?usage: finalize.sh <basename>  (e.g. portal-cold)}"
src="$here/out/$name.mp4"
grade="eq=saturation=1.55:contrast=1.08"   # the chosen "Ghostty pop" grade

[ -f "$src" ] || { echo "missing master: $src — run 'vhs demo/$name.tape' first"; exit 1; }

echo "==> grading $name  ($grade)"

# Primary: animated WebP (true-colour, loops, inline-renderable on GitHub).
# This ffmpeg has no libwebp encoder, so go via graded PNG frames + img2webp.
frames="$(mktemp -d)"
trap 'rm -rf "$frames"' EXIT
ffmpeg -y -loglevel error -i "$src" -vf "$grade,fps=15,scale=1000:-1:flags=lanczos" "$frames/f%04d.png"
img2webp -loop 0 -d 66 -q 80 -m 6 "$frames"/f*.png -o "$here/out/$name-vivid.webp" >/dev/null

# Secondary: vivid MP4 (true-colour download; usable for a GitHub attachment embed).
ffmpeg -y -loglevel error -i "$src" -vf "$grade" \
  -c:v libx264 -pix_fmt yuv420p -crf 18 -movflags +faststart \
  "$here/out/$name-vivid.mp4"

echo "==> done:"
for f in "$name-vivid.webp" "$name-vivid.mp4"; do
  printf "   %-28s %s\n" "$f" "$(du -h "$here/out/$f" | cut -f1)"
done
