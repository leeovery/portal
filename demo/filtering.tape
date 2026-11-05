# A still: the sessions narrowed as a filter is typed. capturetool runs the
# sessions-filtering fixture (internal/capture/fixtures.go) live; its last
# frame is the art, as testdata/vhs/frames draws the fixture.
#
# Record it, and make art/filtering.webp:
#   demo/record.sh filtering

Output "demo/out/filtering/"

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
Type "TZ=UTC demo/out/capturetool --theme tokyo-night --fixture sessions-filtering"
Enter
Wait+Screen /P O R T A L/
# Reached by sessions-filtering's keys, typed while hidden.
Type "/relay"
Sleep 500ms
Show

Sleep 1s

Hide
Sleep 500ms
Escape
Type "q"
