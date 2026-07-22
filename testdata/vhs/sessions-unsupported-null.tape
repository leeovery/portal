# vhs tape: Sessions — Unsupported NULL (the §7 no-host-terminal banner split).
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `sessions-unsupported-null` fixture — the sessions-flat set opened in the NORMAL
# list (no multi-select) with the host-terminal detection cache seeded to an EMPTY
# spawn.Identity{} (empty BundleID → IsNull() true → resolves unsupported-NULL) —
# and screenshots the first painted frame: the STANDARD `Sessions ··· N` section
# header with NO banner, visually identical to sessions-flat. This is the
# regression anchor that the resolved-unsupported NULL seed path does not intrude a
# banner. The harness imports Portal's real internal/tui and binds every tmux seam
# to an in-memory fake, so this NEVER opens a tmux server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/sessions-unsupported-null.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/sessions-unsupported-null.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the deterministic fixture. `go run`
# compiles + runs in one step; no tmux server, no bootstrap, no daemon.
Type "go run ./cmd/capturetool --fixture sessions-unsupported-null"
Enter

# First paint: the model fetches the fixture sessions on Init and renders the
# standard header from the seeded resolved-NULL detection cache (no banner). The 4s
# pad covers the `go run` compile (cold build cache) + first render — don't trim it
# or a cold-cache run flakes.
Sleep 4s

Screenshot "testdata/vhs/sessions-unsupported-null.png"
