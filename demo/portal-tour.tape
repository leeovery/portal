# Portal demo tour — the real portal binary running inside the isolated demo
# container (see demo/README.md). VHS records on the host; portal + tmux + the
# daemon all run sandboxed in the container.
#
# Re-record:  vhs demo/portal-tour.tape   (after demo/build.sh)
# VHS renders the colour-accurate master MP4; demo/finalize.sh grades it to the
# vivid delivery formats (portal-tour-vivid.{mp4,gif}).
Output demo/out/portal-tour.mp4
Require docker

Set Shell "zsh"
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 16
Set Width 1280
Set Height 800
Set Padding 20
Set TypingSpeed 110ms

# --- hidden: boot the container and clear to a fresh prompt ---
Hide
Type "docker run --rm -it portal-demo" Enter
Sleep 7s
Type "clear" Enter
Sleep 800ms
Show

# 1. open the picker
Sleep 800ms
Type "x" Sleep 300ms Enter
Sleep 2500ms
Screenshot demo/out/t1-sessions.png

# 2. group by project
Type "s"
Sleep 1800ms
Screenshot demo/out/t2-by-project.png

# 3. group by tag
Type "s"
Sleep 1800ms
Screenshot demo/out/t3-by-tag.png

# back to flat
Type "s"
Sleep 1200ms

# 4. filter
Type "/"
Sleep 400ms
Type "pay"
Sleep 1500ms
Screenshot demo/out/t4-filter.png
Escape
Sleep 1000ms

# 5. scrollback preview
Down Sleep 200ms Down
Sleep 500ms
Space
Sleep 2200ms
Screenshot demo/out/t5-preview.png
Space
Sleep 800ms

# 6. filter to a portal session and attach
Type "/"
Sleep 300ms
Type "portal-a3"
Sleep 1000ms
Enter
Sleep 1200ms
Enter
Sleep 2500ms
Screenshot demo/out/t6-attached.png
Sleep 1500ms
