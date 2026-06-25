#!/usr/bin/env bash
# Grade a VHS master recording and emit the delivery formats.
#
# VHS reproduces portal's EXACT sRGB palette (verified pixel-for-pixel), which
# reads muted next to a wide-gamut terminal — Ghostty on a P3 Mac renders the
# same hex values more vividly because it isn't colour-managed. This bakes a
# saturation/contrast boost so the recording matches that on-screen vividness.
#
# Usage:  finalize.sh <basename>      # e.g. finalize.sh portal-tour
# Input:  demo/out/<basename>.mp4     (the VHS master — run `vhs <tape>` first)
# Output: demo/out/<basename>-vivid.{mp4,gif}
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
name="${1:?usage: finalize.sh <basename>  (e.g. portal-tour)}"
src="$here/out/$name.mp4"
grade="eq=saturation=1.55:contrast=1.08"   # the chosen "Ghostty pop" grade

[ -f "$src" ] || { echo "missing master: $src — run 'vhs demo/$name.tape' first"; exit 1; }

echo "==> grading $name  ($grade)"

# Primary: vivid MP4 (crisp, true-colour, smallest). Embed via <video>.
ffmpeg -y -loglevel error -i "$src" -vf "$grade" \
  -c:v libx264 -pix_fmt yuv420p -crf 18 -movflags +faststart \
  "$here/out/$name-vivid.mp4"

# Fallback: vivid GIF (inline ![](), 256-colour with a full-stats palette + dither).
ffmpeg -y -loglevel error -i "$src" \
  -vf "$grade,fps=15,scale=1100:-1:flags=lanczos,palettegen=stats_mode=full" "$here/out/_pal.png"
ffmpeg -y -loglevel error -i "$src" -i "$here/out/_pal.png" \
  -lavfi "[0:v]$grade,fps=15,scale=1100:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=sierra2_4a" \
  "$here/out/$name-vivid.gif"
rm -f "$here/out/_pal.png"

echo "==> done:"
for f in "$name-vivid.mp4" "$name-vivid.gif"; do
  printf "   %-28s %s\n" "$f" "$(du -h "$here/out/$f" | cut -f1)"
done
