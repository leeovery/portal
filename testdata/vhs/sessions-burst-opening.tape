# vhs tape: Sessions — In-Burst Opening band (the §6.5 `Opening n/N…` affordance).
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-burst-opening` fixture — the sessions-flat set opened in multi-select
# mode with three sessions marked (agentic-workflows-codify, fab-flowx-explore,
# designlab-web-r8suyU) and the in-burst Opening band seeded as (2, 3) — and
# screenshots the first painted frame: the accent.violet `Opening 2/3…` band (the
# HIGHEST section-header claimant) owning the section-header row while the three
# marked rows keep their violet ● beneath it, over the multi-select footer. This
# frame is a design RESIDUAL (no delivered Paper oracle); this capture is the new
# reference. The harness imports Portal's real internal/tui and binds every tmux
# seam to an in-memory fake, so this NEVER opens a tmux server or touches
# ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/sessions-burst-opening.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/sessions-burst-opening.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the deterministic fixture. `go run`
# compiles + runs in one step; no tmux server, no bootstrap, no daemon.
Type "go run ./cmd/capturetool --fixture sessions-burst-opening"
Enter

# First paint: the model fetches the fixture sessions on Init, enters multi-select
# with the seeded marked set, and renders the seeded Opening band. The 4s pad covers
# the `go run` compile (cold build cache) + first render — don't trim it or a
# cold-cache run flakes.
Sleep 4s

Screenshot "testdata/vhs/sessions-burst-opening.png"
