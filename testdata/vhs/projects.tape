# vhs tape: Projects page (MV reskin), DARK owned canvas.
#
# Drives the offline capture harness (cmd/capturetool) with the deterministic
# `projects` fixture: a rich project store (14 projects with real-looking absolute
# paths, matching the §6 Projects (MV) reference frame). The fixture opens on the
# Sessions page (the production no-arg default), and this tape types `x` to switch
# to the Projects page — the exact path a user takes — then screenshots it. The
# capture shows the §3.1 PORTAL header, the state.green `Projects 14` section header
# (text.detail count + right-aligned `/ to filter` hint), the two-line MV rows (name
# text.primary heavy / path text.detail dim), the full-height accent.violet left-bar
# selection over the bg.selection tint on the cursor row (flow-v1-api), and the §6.3
# condensed footer. Like the other fixtures this NEVER opens a tmux server or touches
# ~/.config/portal.
#
# Run it (sandbox MUST be disabled — vhs needs loopback networking for ttyd;
# see testdata/vhs/README.md):
#
#   vhs testdata/vhs/projects.tape
#
# NOTE: vhs tape paths containing a slash MUST be quoted (the GIF Output and the
# Screenshot path below) — an unquoted slashed path is a parser error.

# Output is required by vhs even though we only care about the PNG screenshot.
Output "testdata/vhs/.gifcache/projects.gif"

Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 800
# A fixed shell prompt keeps the captured frame deterministic across machines.
Set Shell "bash"

# Build then launch the capture tool with the projects fixture. The fixture opens on
# the Sessions page; give the compile + first render time to settle.
Type "go run ./cmd/capturetool --fixture projects"
Enter
Sleep 4s

# Switch Sessions -> Projects with the `x` key (the real user path), then let the
# Projects page render.
Type "x"
Sleep 2s

Screenshot "testdata/vhs/projects.png"
