# vhs tape: Sessions — Multi-Select Pre-flight Abort (the §6.7 gone-session abort).
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-multi-select-preflight-abort` fixture — the sessions-flat set opened in
# multi-select mode with three sessions marked (agentic-workflows-codify,
# fab-flowx-explore, designlab-web-r8suyU), the cursor anchored on the gone row
# fab-flowx-explore, and the §6.7 pre-flight abort seeded on fab-flowx-explore — and
# screenshots the first painted frame: the red `⚠ 'fab-flowx-explore' is gone —
# nothing opened` banner (+ dim `esc dismiss`) owning the section-header row, the
# gone row's red ⚠ + `session gone` badge, the two survivors' violet ● markers, and
# the multi-select footer. The harness imports Portal's real internal/tui and binds
# every tmux seam to an in-memory fake, so this NEVER opens a tmux server or touches
# ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/sessions-multi-select-preflight-abort.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/sessions-multi-select-preflight-abort.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the deterministic fixture. `go run`
# compiles + runs in one step; no tmux server, no bootstrap, no daemon.
Type "go run ./cmd/capturetool --fixture sessions-multi-select-preflight-abort"
Enter

# First paint: the model fetches the fixture sessions on Init, enters multi-select
# with the seeded marked set, anchors the cursor on the gone row, and renders the
# seeded abort banner + gone flag. The 4s pad covers the `go run` compile (cold
# build cache) + first render — don't trim it or a cold-cache run flakes.
Sleep 4s

Screenshot "testdata/vhs/sessions-multi-select-preflight-abort.png"
