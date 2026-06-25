# Portal cold-boot / resurrection demo. The container starts with a baked restore
# seed and NO running tmux server, so `x` triggers a real cold boot: the honest
# loading screen, a live restore of every saved session (with its scrollback),
# then the repopulated picker. Fully sandboxed (see demo/README.md).
#
# Re-record:  vhs demo/portal-cold.tape   (after demo/build.sh)
# VHS renders the colour-accurate master MP4; demo/finalize.sh grades it to the
# vivid delivery formats (portal-cold-vivid.{mp4,gif}).
Output demo/out/portal-cold.mp4
Require docker

Set Shell "zsh"
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 16
Set Width 1280
Set Height 800
Set Padding 20
Set TypingSpeed 110ms

# --- hidden: boot the cold container and clear to a fresh prompt ---
Hide
Type "docker run --rm -it -e DEMO_COLD=1 portal-demo" Enter
Sleep 5s
Type "clear" Enter
Sleep 800ms
Show

# cold boot: x -> loading screen (restoring N/M) -> repopulated picker
Sleep 900ms
Type "x" Sleep 300ms Enter
Sleep 600ms
Screenshot demo/out/c1-loading.png
Sleep 2600ms
Screenshot demo/out/c2-landed.png

# cold boot lands on Projects; switch to the restored Sessions list
Type "x"
Sleep 1600ms
Screenshot demo/out/c3-sessions.png

# attach a restored session — its scrollback came back too
Type "/"
Sleep 300ms
Type "portal-a3"
Sleep 900ms
Enter
Sleep 1000ms
Enter
Sleep 2500ms
Screenshot demo/out/c4-attached.png
Sleep 1500ms
