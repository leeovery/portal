# A still: the loading page restoring sessions, 8 of 12. capturetool plays the
# portal-cold scenario (internal/capture/scenarios.go); its frame at 2s,
# between the 8th session restored at 1.95s and the 9th at 2.1s, is the art, as
# testdata/vhs/frames/loading-screen-120x40.txt draws it.
#
# Record it, and make art/loading.webp:
#   demo/record.sh loading

Output "demo/out/loading/"

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

Sleep 2s

Hide
Sleep 500ms
# Till the restore lands, the loading page takes no key but Ctrl+C.
Ctrl+C
