# The README's demos

Records the animated clips and stills in the project README, `art/*.webp`, by playing scripted
scenarios through the real picker, or running the fixtures the golden frames draw:
`cmd/capturetool --scenario` and `--fixture`, the visual capture harness's tool (see
[testdata/vhs/README.md](../testdata/vhs/README.md)), build the picker through `tui.Build`, as
`portal open` does, in-process, with the backend and every other seam faked. Nothing runs the
`portal` binary: no zmx, no daemon, no bootstrap, and nothing read or written under
`~/.config/portal`. So nothing needs a container, and it's safe to record beside a live Portal.

Every tape draws the picker in `tokyo-night`, capturetool's `--theme`, in a terminal of Tokyo
Night's own colours, its `Set Theme`; the themes clip previews the other built-ins as it goes.

```bash
demo/record.sh                        # every tape: record it, then make its art
demo/record.sh portal-cold loading    # the tapes named
```

## What's recorded

| Tape | Scenario or fixture | Size | Art |
|---|---|---|---|
| `portal-cold` | `portal-cold` | 120 × 40 | `art/portal-cold.webp`, `.mp4`: the README's lead: the first launch after a reboot, the loading page restoring twelve sessions, the picker landing on them, and a restored session's scrollback previewed |
| `portal-tour` | `portal-tour` | 120 × 40 | `art/portal-tour.webp`, `.mp4`: the tour: the list flat, by project and by tag, the filter, the preview, and Projects |
| `themes` | `themes` | 120 × 40 | `art/themes.webp`, `.mp4`: the theme panel, previewing each built-in over the sessions, from tokyo-night and back |
| `loading` | `portal-cold` | 120 × 40 | `art/loading.webp`, a still: the loading page, 8 of 12 sessions restored |
| `sessions-by-tag` | fixture `sessions-by-tag` | 120 × 40 | `art/sessions-by-tag.webp`, a still: the sessions grouped by tag |
| `preview` | fixture `preview-screen` | 120 × 40 | `art/preview.webp`, a still: a session's scrollback, previewed |
| `help-modal` | fixture `sessions-help` | 120 × 40 | `art/help-modal.webp`, a still: the keys, over the sessions |
| `projects` | fixture `projects` | 120 × 40 | `art/projects.webp`, a still: the Projects page |
| `edit-project` | fixture `projects-edit` | 120 × 40 | `art/edit-project.webp`, a still: a project's name, aliases and tags, open to edit |
| `filtering` | fixture `sessions-filtering` | 120 × 40 | `art/filtering.webp`, a still: the sessions narrowed as a filter is typed |
| `kill-confirm` | fixture `sessions-kill-confirm` | 120 × 40 | `art/kill-confirm.webp`, a still: a kill, asked to confirm |
| `resume-panel` | fixture `resume-panel-waiting` | 80 × 24 | `art/resume-panel.webp`, a still: a restored session waiting to resume |
| `theme-panel` | fixture `theme-panel-open` | 120 × 40 | `art/theme-panel.webp`, a still: the theme panel over the sessions, its cursor on tokyo-night |

A tape's art is named after it. `record.sh`'s `stills` list says which tapes are stills: every
tape that runs a fixture, and `loading`, which stops the `portal-cold` scenario at 2 seconds. A
fixture's still types the fixture's keys while hidden, so its last frame is the one the golden frame
draws.

## The scenarios

A scenario, in `internal/capture/scenarios.go`, is a fixture's world and its cues: what the
bootstrap streams to the loading page, each step at its offset in seconds from the start, and the
moment the restore lands. There are three:

- **`portal-cold`**, the flat page's twelve sessions as a reboot leaves them, three waiting to
  resume. zmx is checked at 0.25s; the saver's steps come at 0.5, 0.65 and 0.8; the sessions are
  restored one every 0.15s from 0.9, 8 of 12 at 1.95 as the loading frame draws it; the restore is
  done at 2.7 and the latch written at 2.8; and the bootstrap completes at 2.9. The lister lists
  no session before then and all twelve after, so the picker lands on them, the waiting three
  showing their dots.
- **`portal-tour`**, the by-tag page's eight sessions, their projects tagged work, client and
  personal, opened flat, so `s` steps through every view.
- **`themes`**, the tour's world, with a theme source whose rows keep each built-in's own palette,
  so the panel previews each one as its cursor reaches it.

```go
checks(0.25),             // zmx checked
saverStep(0.5, 2),        // the first of the steps the loading page shows as starting the saver
restores(0.9, 0.15, 12),  // twelve sessions restored, one every 0.15s from 0.9
completes(coldCompletes), // the bootstrap complete, at 2.9: the picker lands on the restored sessions
```

A scenario's world leaves every spawn, detector, persister, runner and hydrating seam nil, as a
fixture does, so nothing it plays reaches a terminal window, a session or a file, and nothing is
ever written. The preview reads each session's scrollback from the scenario's histories: generic
shell output, naming no tool and nothing personal.

The picker runs as it does live. `Scenario.Model` builds the world first, then reads the clock, so
a cue at 2.5 seconds plays 2.5 seconds after the tape's first frame; the picker's own timers, such
as the loading page's minimum time on screen, run in real time beside the cues. The scenario plays
the same way every time: what it shows is set by its cues and the keys pressed. In the tests the
same model plays on `testing/synctest`'s virtual clock, so a scenario's seconds pass in no time,
with no clock seam added to `internal/tui`.

Each key pressed is shown for a moment as a keycap in the bottom gutter row, right-aligned to the
gutter's inset, so a reader sees what was typed: the capture tool draws them, not the picker. A cap
shows for 1.5 seconds after the last key, the latest four at most, each named as Portal's footer
names keys (`⏎ ␣ ↑ ↓ ← → ^↑ ^↓ shift-tab`), bold, in the canvas colour on the theme's `accent.key`.
They follow the theme the screen shows, the theme panel's previews included, and are drawn in
reverse video under `NO_COLOR`. They never widen the row or touch the footer above it.

Each tape runs the capture tool under `TZ=UTC`, as capture's tests draw the frames, so the art
never depends on the recording machine's zone.

The keys are the tape's, timed to the cues: each tape's comment says which, and each scenario's
comment names the keys its cues are timed against. The themes walk, for one: `t` at 1s opens the
panel on tokyo-night, fifth of the six; `↑` every 0.8s from 2s, four times, to gitlab-dark-grey;
`↓` every 0.8s from 5.2s, five times, to tokyo-night-day; and `esc` at 9.6s, back to tokyo-night.
It never presses `Enter`, which would commit the theme.

## How a tape becomes art

`record.sh` builds the capture tool into `demo/out/` (never `portal`), and for each tape removes
`demo/out/<name>/`, runs the tape through vhs, checks it wrote frames newer than its start, and
hands them to `finalize.sh`, which makes the art:

- **The frames, not a video.** vhs writes every frame it shows to `demo/out/<name>/`, a layer of
  text and one of the cursor, exactly as the terminal drew them. A video's compression shifts every
  pixel a little from frame to frame, which an animated WebP then stores whole; from the frames
  themselves it stores only what changed, losslessly.
- **The art.** The cursor's layer goes over the text's, in a 24 px margin of the screen's own
  canvas, read from each frame's top-right pixel so it follows the theme panel's previews. A clip
  becomes a lossless animated WebP, which GitHub shows inline, at the tapes' 30 frames a second, and
  an H.264 MP4 of the same frames; a still becomes a lossless WebP of its last frame.
- **The size.** JetBrains Mono at 17px makes a cell 10 × 22 px, so a 120 × 40 tape is 1248 × 928
  with its margin, and the resume panel's 80 × 24 is 848 × 576. The README shows the clips at 840 px
  and the stills at 430, so a high-density screen draws them sharp.
- **The colours** are the themes' own: nothing is graded.

`demo/out/` is remade each time and never committed: `art/` is.

## The tape test

`go test ./internal/capture` plays every tape in `demo/` as it's recorded. `tape_test.go` parses the
tapes themselves, so a tape and its test can't drift: a strict subset of vhs's language (comments,
`Output`, the header's `Set`s, `Hide` and `Show`, `Type`, `Sleep`, the named keys with a count,
`Shift+Tab`, `Ctrl+<key>` and `Wait+Screen`), anything else refused. The clock starts at the first
`Wait+Screen`, and each key typed takes the tape's typing speed, 50 ms where it sets none.

`tapes_test.go` holds each tape, in `held`, to what it launches and what its screen shows at
checkpoints: text it says, text it no longer says, a golden frame whose text it is, or the built-in
it's drawn in, at a moment into its play, and as its last `Show` ends, which is a still's art. Each
tape plays under `testing/synctest` at its header's grid, its keys pressed at their times. A
fixture's still is held to its golden frame's text. The tests also hold every tape to the header,
its grid to the size its program draws, and its `Set Theme` to the theme's canvas, text and
selection; to running nothing but `demo/out/capturetool`, so never `portal`, zmx or tmux; to naming
a scenario or fixture that exists; and to quitting its program while hidden, so no frame shows a
quit and nothing it types reaches the shell. `record.sh` lists stills only of tapes there are.

**The quarter-second rule.** No checkpoint sits within a quarter second of anything that changes
the screen: a key, a cue, the restore, or a run of keycaps fading. A recording starts its clock when
vhs sees the program's first screen, a little after the program's own, and its keys land a little
late, so a screen checked closer than that could be recorded either side of the change. A tape whose
art has to sit closer, by design, is listed in `tightCheckpoints` with its window and why: `loading`,
whose 8 of 12 shows only from the 8th restore, at 1.95s, to the 9th, at 2.1s. A test holds its
screen the same throughout the window, and an entry whose tape no longer sits close to a change
fails, to be deleted.

Change a cue's moment and the tape's sleeps follow it, or the tape test fails.

## Setting up

```bash
brew install vhs ffmpeg webp
ls ~/Library/Fonts /Library/Fonts | grep JetBrainsMono-Regular
```

The tapes need JetBrains Mono installed, as the size above depends on its cell.

vhs records a tape's frames into a directory of its own under `$TMPDIR`, then moves it to the tape's
`Output`. Where `$TMPDIR` is on another volume than the repository, the move fails without a word,
and no frames come: `record.sh` gives vhs a `TMPDIR` in `demo/out` for that, and fails a tape that
leaves no frames newer than its start.

vhs drives a headless browser that connects to `ttyd` over loopback, which an agent's sandbox
refuses: run `record.sh` with the sandbox off, as
[testdata/vhs/README.md](../testdata/vhs/README.md) says.
