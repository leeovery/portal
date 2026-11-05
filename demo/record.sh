#!/usr/bin/env bash
# Record the README's demos, and make their art: the tapes named, or every
# one, through vhs, then finalize.sh. Run it from anywhere; it works from the
# repository's root, where the tapes' paths start. It builds capturetool, the
# program the tapes run, and never portal.
#
# vhs records a tape's frames into a directory of its own under $TMPDIR, then
# moves it to the tape's Output. Where $TMPDIR is on another volume than the
# repository, the move fails without a word and no frames come, so vhs is
# given a TMPDIR in demo/out. It can also fail to write the frames without a
# word, so a run that leaves none newer than its start fails here.
#
# Usage:  demo/record.sh                every tape
#         demo/record.sh tour help      the tapes named
set -euo pipefail

cd "$(dirname "$0")/.."
stills=" sessions-by-tag preview loading help-modal projects edit-project filtering kill-confirm resume-panel theme-panel "

names=("$@")
if [ ${#names[@]} -eq 0 ]; then
  for tape in demo/*.tape; do
    [ -f "$tape" ] || {
      echo "no tapes in demo/" >&2
      exit 1
    }
    names+=("$(basename "$tape" .tape)")
  done
fi

for name in "${names[@]}"; do
  [ -f "demo/$name.tape" ] || {
    echo "no tape demo/$name.tape" >&2
    exit 1
  }
done

mkdir -p demo/out/.vhs
go build -o demo/out/capturetool ./cmd/capturetool
stamp="demo/out/.vhs/stamp"
for name in "${names[@]}"; do
  rm -rf "demo/out/$name"
  touch "$stamp"
  TMPDIR="$PWD/demo/out/.vhs" vhs --quiet "demo/$name.tape"
  [ -n "$(find "demo/out/$name" -name 'frame-text-*.png' -newer "$stamp" -print -quit 2>/dev/null)" ] || {
    echo "vhs wrote no frames for demo/$name.tape" >&2
    exit 1
  }
  case "$stills" in
  *" $name "*) demo/finalize.sh --still "$name" ;;
  *) demo/finalize.sh "$name" ;;
  esac
done
