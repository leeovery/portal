# The theme panel: each built-in previewed as the cursor reaches it.
# capturetool plays the themes scenario (internal/capture/scenarios.go), whose
# panel rows keep their own palettes, through the real picker, in real time.
#
# t at 1s opens the panel on tokyo-night, fifth of the six. ↑ every 0.8s from
# 2s, four times, to gitlab-dark-grey; ↓ every 0.8s from 5.2s, five times, to
# tokyo-night-day; esc at 9.6s, back to tokyo-night. Never Enter: it would
# commit.
#
# Record it, and make art/themes.webp and art/themes.mp4:
#   demo/record.sh themes

Output "demo/out/themes/"

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
Type "TZ=UTC demo/out/capturetool --theme tokyo-night --scenario themes"
Enter
Wait+Screen /P O R T A L/
Show

Sleep 1s
Type "t"
Sleep 950ms
Up@800ms 4
Down@800ms 5
Sleep 400ms
Escape
Sleep 1850ms

Hide
Sleep 500ms
Type "q"
