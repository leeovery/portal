# The README's lead clip: Portal's first launch after a reboot. capturetool
# plays the portal-cold scenario (internal/capture/scenarios.go) through the
# real picker, in real time, with every seam faked; the keys below are timed to
# its cues.
#
# The loading page checks zmx, starts the saver and restores the twelve
# sessions, 8 of 12 at 1.95s, then lands on them at 2.9s, three waiting to
# resume. ↓↓ at 4.5s to kiln-relayx-explore, ␣ at 5.5s to preview its replayed
# scrollback, and ↓ at 8.5s to loom-p84cXa's. It ends in the preview, at 10.5s.
#
# Record it, and make art/portal-cold.webp and art/portal-cold.mp4:
#   demo/record.sh portal-cold

Output "demo/out/portal-cold/"

Set Shell "bash"
Set FontFamily "JetBrains Mono"
Set FontSize 17
Set LetterSpacing 0
Set LineHeight 1
# 120 × 40 cells of 10 × 22 px, and the margins about them.
Set Width 1260
Set Height 924
Set Padding 16
Set CursorBlink false
Set Framerate 30
Set Theme { "name": "tokyo-night", "background": "#0B0C14", "foreground": "#C0CAF5", "cursor": "#0B0C14", "selection": "#28243A", "black": "#15161E", "red": "#F7768E", "green": "#9ECE6A", "yellow": "#E0AF68", "blue": "#7AA2F7", "magenta": "#BB9AF7", "cyan": "#7DCFFF", "white": "#A9B1D6", "brightBlack": "#414868", "brightRed": "#F7768E", "brightGreen": "#9ECE6A", "brightYellow": "#E0AF68", "brightBlue": "#7AA2F7", "brightMagenta": "#BB9AF7", "brightCyan": "#7DCFFF", "brightWhite": "#C0CAF5" }
Set WaitTimeout 2m

Hide
# In UTC, as the frames are drawn, whatever the recording machine's zone.
Type "TZ=UTC demo/out/capturetool --theme tokyo-night --scenario portal-cold"
Enter
Wait+Screen /Checking zmx/
Show

# Restored at 2.9s, the waiting sessions' dots shown.
Sleep 4500ms
Down 2
Sleep 900ms
# kiln-relayx-explore's scrollback, at 5.5s.
Space
Sleep 2950ms
# The next session's, loom-p84cXa's, at 8.5s.
Down
Sleep 1950ms

Hide
Sleep 500ms
Escape
Type "q"
