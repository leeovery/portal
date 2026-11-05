# The README's tour: the picker's views, its filter, the preview and Projects.
# capturetool plays the portal-tour scenario (internal/capture/scenarios.go)
# through the real picker, in real time, with every seam faked.
#
# Eight sessions, their projects tagged work, client and personal, opened flat.
# s at 1.5s, 3.5s and 5.5s: by project, by tag, flat again. / at 7s, sync typed
# from 7.25s, and esc at 9.5s. ↓↓ at 10.5s, ␣ at 11.5s to preview, ↓ at 13.5s
# to the next session's, ␣ at 15s back. x at 16s to Projects, x at 18s back. It
# ends on a selected row, at 20s, its keys faded.
#
# Record it, and make art/portal-tour.webp and art/portal-tour.mp4:
#   demo/record.sh portal-tour

Output "demo/out/portal-tour/"

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
Type "TZ=UTC demo/out/capturetool --theme tokyo-night --scenario portal-tour"
Enter
Wait+Screen /P O R T A L/
Show

# By project, by tag, then flat again.
Sleep 1500ms
Type "s"
Sleep 1950ms
Type "s"
Sleep 1950ms
Type "s"
Sleep 1450ms
# The filter, a fuzzy term typed, then cleared.
Type "/"
Sleep 200ms
Type "sync"
Sleep 2050ms
Escape
Sleep 950ms
# The preview, the next session's, and back.
Down 2
Sleep 900ms
Space
Sleep 1950ms
Down
Sleep 1450ms
Space
Sleep 950ms
# Projects, and back.
Type "x"
Sleep 1950ms
Type "x"
Sleep 1950ms

Hide
Sleep 500ms
Type "q"
