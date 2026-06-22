# vhs tape: Projects page — command-pending banner (§11.4 MV reskin), DARK owned canvas.
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `projects-command-pending` fixture: the SAME rich project store as the `projects`
# fixture (14 projects with real-looking absolute paths), but built with a pending
# command so the model opens DIRECTLY on the command-pending Projects page
# (WithCommand sets PageProjects). The capture shows the §11.4 banner — a violet `▌`
# left-bar + `▸ Pick a project to run` + the command in an accent.orange chip, on a
# subtle tinted band under the title separator, ABOVE the green `Projects 14`
# section header — over the FULL Projects chrome (green header + `/ to filter`), with
# the swapped `⏎ run here · n run in cwd · esc cancel` footer. Mirrors
# testdata/vhs/reference/projects-command-pending-mv.png.
#
# The seeded command is a GENERIC build command (`npm run dev`) — Portal's
# run-a-command mechanism is tool-agnostic, so no Portal artifact references any
# specific tool. The reference frame's chip shows a different example command; only
# the chip's command text differs (generic vs the frame's example) — the banner
# structure, colours, chrome, and footer match.
#
# Like the other fixtures this NEVER opens a tmux server or touches ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/projects-command-pending.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/projects-command-pending.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the command-pending fixture. The fixture
# opens DIRECTLY on the command-pending Projects page (no key drive needed — the
# pending command sets PageProjects at construction); give the compile + first
# render time to settle.
Type "go run ./cmd/capturetool --fixture projects-command-pending"
Enter
Sleep 4s

Screenshot "testdata/vhs/projects-command-pending.png"
