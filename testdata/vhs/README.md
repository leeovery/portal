# Portal TUI — visual capture harness (`vhs`)

This directory is where the TUI is checked against its frames. `frames/` keeps them: what every
fixture draws, as text and as 24-bit ANSI, which the capture tests hold the code to cell for cell,
so every screen change shows up as a frame diff. And by eye, for what a test can't judge, how a page
reads against its design: a task that changes what the picker draws can write a `vhs` tape here,
screenshot a fixture through it, and judge the frame against a committed design reference where one
exists.

The TUI is drawn by `cmd/capturetool`, a program of its own, not a `portal` subcommand. It builds
the production model through the shared `tui.Build` constructor, as `cmd/open.go` does, with the
backend and every other seam faked by `internal/capture`: the session lister and its pending set,
the history reader, the killer, labeller and creator, the project and alias stores, the theme
enumeration, and the loading page's progress. It never runs zmx, never starts a daemon, never runs
the bootstrap, and never reads or writes `~/.config/portal`, so it's safe to run beside a live
Portal. The `portal` binary never links `internal/capture`: `internal/capture` is listed in
`TestOnlyPackages`, and `internal/sourceguardtest/testonly_linkage_guard_test.go` fails if anything
the `portal` binary builds ever imports it.

This matters more than it looks: Portal can't be run from a scratch build to check a visual change,
since a scratch build disturbs the running daemon and its bootstrap touches real state. The harness
is the only route to seeing a change before release.

## What lives here, and for how long

**PNGs and the tapes that render them are scaffolding, not a durable asset.** The regression
baseline is `frames/`, which the tests read; nothing diffs a PNG against anything, so a permanent
image set would only be a directory of files that rot the first time a colour token is renamed. The
rule:

> A capture and its tape are **created as work proceeds**, **committed while the work is being
> collaborated on** (so the reviewer and the human open the same frame the implementer did), and
> **cleared out at sign-off**.

Sign-off means the *feature's* sign-off, not a phase's: work that re-captures a screen in a later
phase owns clearing what it re-captured. So an empty-looking directory is the normal resting state.
If you are here to change a screen, you are expected to add a tape, capture, and take it away again
when the work lands.

| Path | What | Lifetime |
|---|---|---|
| `<fixture>.tape` | The `vhs` tape that drives one fixture and screenshots it | Scaffolding: cleared at sign-off |
| `<fixture>.png` | The captured frame | Scaffolding: cleared at sign-off |
| `frames/<stem>.txt` | A fixture's golden frame as its character grid: a line a row, trailing blanks trimmed | **Kept**: regenerated, never hand-edited |
| `frames/<stem>.ansi` | The same frame in 24-bit colour, with no newline after the last row, so `cat` in a terminal of its size doesn't scroll it | **Kept**: regenerated, never hand-edited |
| `reference/*.png` | Committed design exports: the frames the code was built *against* | **Kept** |
| `LOCK-IN.md` | The light-tint lock-in record: pinned hexes, derivations, contrast ratios and the eyeball decision that settled them. A historical record; the captures it names were cleared long ago | **Kept** |
| `.gifcache/*.gif` | Transient `vhs` byproduct of a tape's `Output`; tapes write it into the hidden subdir so the listing stays clean | **git-ignored**, never committed |

**Why `reference/` is exempt.** A capture is a render *of the code*, which is why it rots and why
the rule above exists. A reference frame is the **design** the code was built against, exported and
committed *before* implementation so the implementer and the reviewer could self-check against it.
It doesn't go stale the way a capture does; it is a record of what was specified, and for some
screens it is the only reference that exists. Keep them, and note that no Go source cites them by
path, so their retention rests on this table rather than on an inbound reference.

**The fixtures and the harness are permanent.** The Go fixture definitions in `internal/capture`,
`cmd/capturetool` and the `vhs` route all stay. Only the images and tapes here are transient.
Deleting a fixture is a different act entirely, with a consequence: see Adding (or removing) a
fixture.

The README's art is made elsewhere, by the tapes in `demo/`, and kept in `art/`: see
[demo/README.md](../../demo/README.md).

## The frames

49 of them, each by its stem: the fixture's name and the terminal size it's drawn at, as in
`sessions-flat-120x40`. 42 are picker fixtures (23 `sessions-*`, 11 `theme-panel-*`, 3
`projects*`, 3 `preview-*` and 2 `loading-*`), one is the contrast swatch, and 6 are the resume
panel's standalone surfaces. All are drawn in `tokyo-night` (`capture.FramesThemeSlug`, pinned as a
literal so moving the shipped default rewrites nothing), in colour whatever `NO_COLOR` says: the
`sessions-pending-resume-colourless` fixture is colourless by its own declaration.

The size is the one the screen is designed for:

| Frames | Count | Size | Why |
|---|---|---|---|
| Every picker fixture and the contrast swatch, unless below | 39 | `120x40` | the harness size the package's tests have always drawn at |
| `theme-panel-narrow`, `theme-panel-confirm` | 2 | `63x40` | the panel's minimum-width terminal (`tui.ThemePanelMinWidthTerminal`); the narrow fixture declares it, the confirm wraps its message there |
| `theme-panel-min-height-message` | 1 | `63x10` | its declared size: the minimum width on the panel's height floor |
| `theme-panel-dir-unreadable` | 1 | `120x18` | short enough that the panel's list spans two pages, which is where the warning shows |
| `resume-panel-*` (card) | 4 | `80x24` | the surface's pinned size |
| `resume-panel-*-degraded` | 2 | `40x10` | the surface's pinned size, below the card |

A fixture whose frame is a geometry declares its render size on the fixture; one designed for a
size it doesn't pin sets its frame size there. Either way the stem names the size the frame is truly
drawn at.

## How the frames are held

`internal/capture`'s `frames_test.go` holds each fixture to its frame, with the cells
`cells_test.go` reads and compares:

- **The fixture is drawn** at its frame's size in `tokyo-night` and read into cells as a terminal
  reads it: a glyph a cell, each with its foreground, background, bold and OSC 8 hyperlink, the SGR
  state carried from cell to cell until reset. What it can't read as a terminal would is a failure,
  not a guess: an escape that is neither SGR nor a hyperlink, an attribute but bold, a colour not in
  24 bits, a control character, or a glyph not one cell wide.
- **Cell for cell,** it's compared with the frame's `.ansi`, passing nothing but what a terminal
  shows alike:
  - **the terminal's background:** a cell whose background isn't painted shows whatever the screen
    set the terminal's background to. The picker fixtures and the contrast swatch set their canvas
    with OSC 11, so there an unpainted cell counts as `tokyo-night`'s canvas. The resume surfaces
    are drawn into a session and set none, and a colourless fixture sets none, so there an
    unpainted cell matches only an unpainted cell: a surface losing its painted canvas, or a
    colourless screen gaining one, fails;
  - **a blank's foreground and bold,** which nothing shows. A link is compared on every cell, blank
    or not, since a linked space is clickable.

  A drawn row shorter than its frame's is read as blank and unpainted to the frame's width; a wider
  row, or a row more or fewer, fails. A failure names the stem and lists the first 20 differing
  cells, each with its row and column (counted from 1) and the frame's and the fixture's cell, then
  how many more.
- **Every frame is a pair**, every frame has a fixture drawing it at its stem's size, every fixture
  keeps a frame, and each `.ansi` reads to its `.txt` at the stem's size. A stored `.ansi` has no
  newline after its last row, so it is read back at its stem's height: a frame whose last row is
  unwritten still holds.
- **Each fixture draws the same twice,** from two separately built worlds, which catches state one
  draw leaves for the next and map-order output. Run in one process, it can't see a dependence on
  the environment. Two such dependences are known and closed:
  - **the home directory:** the fixtures pass theirs as `tui.Deps.Home`, and every frame is held
    again under `HOME` set to that home and to `/`;
  - **East Asian ambiguous widths:** `RUNEWIDTH_EASTASIAN` or a CJK locale would draw `▌` and the
    box glyphs two cells wide. The width libraries read them at init, so the capture tests'
    `TestMain` (`capture.RunNarrow`) refuses to run where they would, naming
    `RUNEWIDTH_EASTASIAN=0` as the fix. Write frames with `--frames` from an environment without
    either.

  No fixture renders the time, so the clock isn't pinned.
- **A frame written is a frame held:** a test writes frames with the capture tool's writer, reads
  them back and holds each fixture to its own.

## The size sweeps

The frames hold each fixture at one size. The sweeps, in `internal/capture`'s `sweep_test.go` and
`sweep_breaks_test.go`, draw every fixture but the contrast swatch at many more, and hold each draw
to rules rather than to a frame. A fixture is drawn as captured, then its terminal resized to each
size, past any size it declares, so a geometry fixture is swept like any other. A resume surface is
drawn by its renderer at each size, past its pin.

**The grid.** Every drawing is swept at the owner's eight terminals: 73x61 and 73x36 (a phone
upright), 138x30 and 138x15 (a phone on its side), 108x44 and 108x24 (an iPad), 173x53 (a desktop)
and 109x53 (a MacBook's side screen). One drawing for each kind of page, and each resume renderer,
is swept across the whole grid as well: the flat, by-project, by-tag and paged lists, a session
search, Projects, the preview, the loading page, the theme panel, the resume panel and its discard
confirmation (`sweptFully`). The grid is every pairing of these:

| | |
|---|---|
| Widths | 20, 40, 52, 63, 73, 80, 100, 108, 120, 138, 160, 173, 230 |
| Heights | 8, 10, 12, 15, 18, 24, 30, 36, 40, 44, 53, 61 |

**The rules.** At every size it's swept at, each drawing:

- **R1, fitting its terminal:** draws exactly as many rows as the terminal has, each exactly as
  wide as the terminal, measured as a terminal measures cells.
- **R2, keeping its chrome:** from 12 rows up (`chromeFloor`), a picker page draws its header's
  first row and its footer row as it draws them at 60 rows, so a short terminal never draws over
  its chrome.
- **R3, showing the cursor's session:** a picker page with a session or project under its cursor at
  60 rows shows that row.
- **R4, no name cut with room:** no name the fixture's world gives (a session's name, label or
  directory, a project's name, path or tag) is cut with `…` on a row whose blank cells at its end
  outnumber what the cut took.
- **R5, no panic:** each drawing swept fully is resized through every width from 1 to 240 at its
  frame's height, then takes a key at every fourth width (↑ ↓ ^↑ ^↓ ⏎ / ␣ s n r k q x t m ? e d l y
  esc); every other drawing takes each of those keys at its frame's size. None may panic.

**The known breaks.** A break the sweep finds is fixed, or listed in `knownBreaks` with why, by
class:

- **owed a fix:** a break at one of the owner's sizes or at an ordinary size, 80x24 or larger,
  listed only until the fix lands. The list holds none today.
- **by design:** what the design draws that a rule reads as a break, such as the loading page's
  centred block reaching the header's row on a short terminal, the Projects page below 13 rows
  (a project row is two lines, so it can't hold one row, its pagination dots and its footer), or a
  name cut where the row keeps its indicator slot so the indicator column holds still. A designed
  cut names the cuts it passes over, as `loom-p8…`, and passes over no others.
- **extreme:** a break only at a size under 52 wide or under 12 tall.

`TestEveryKnownBreakSaysWhy` holds each entry to having a reason and the class its sizes allow: an
extreme break falls only at extreme sizes, a break owed a fix falls at a size that isn't, and a
designed cut is of the rule of a cut name with room. Every entry names a drawing and sizes the sweep
draws.

**A cleared break fails.** The list holds only what's drawn. A listed break that no longer breaks
its rule where it's named fails with `<fixture> at <size> no longer breaks the rule of <rule>:
delete it from knownBreaks`, and a designed cut no longer drawn with room fails with `<fixture> at
<size> no longer draws "<cut>" with room: delete it from knownBreaks`. A fix that clears a break
deletes its entry in the same change. A break the list doesn't name fails as
`<fixture> at <size> breaks the rule of <rule>`, with what shows it.

## Regenerating the frames

```bash
go run ./cmd/capturetool --frames testdata/vhs/frames                          # every fixture
go run ./cmd/capturetool --frames testdata/vhs/frames sessions-flat projects   # those named
```

`--frames` writes each named fixture's `.txt` and `.ansi`, at its own size, in `tokyo-night`: every
fixture where none is named. It takes no other flag, writes nothing if a name has no fixture, and
says each stem it writes. A fixture whose size changes writes a new stem; delete the old pair (the
tests name it).

**Regenerate in the same change that alters a screen,** once the new drawing is right, checking each
altered frame with `--print`. **Never edit a frame by hand.** **An unintended frame change is a
regression** to fix in the code, not a frame to rewrite.

## Setting up

`vhs` needs `ttyd` and `ffmpeg`, which Homebrew installs with it, and the tapes need JetBrains Mono.

```bash
brew install vhs
vhs --version       # vhs 0.12 or later
ttyd --version
ffmpeg -version
ls ~/Library/Fonts /Library/Fonts | grep JetBrainsMono-Regular
```

Without Homebrew, install `ttyd` and `ffmpeg` from your distro's package manager, then
`go install github.com/charmbracelet/vhs@latest`.

## Looking at a fixture

No tape needed to look at something:

```bash
go run ./cmd/capturetool --fixture sessions-flat
go run ./cmd/capturetool --fixture theme-panel-confirm --theme nord
```

The fixture runs full screen, at the size the terminal gives, until you quit it. An empty or
unknown `--fixture` is an error that lists them all; `capture.FixtureNames()` is the same list.
**The tool replays no keys of its own**, so a screen reached by a keystroke, such as the theme
slide-over, the Projects page or the preview, is reached by pressing that key yourself. A tape types
it for you. A fixture declares that sequence in `captureKeys`, which is what the offline driver,
`--print` and `--frames` replay.

### `--theme`, and `NO_COLOR`

```bash
--theme nord                      # a built-in slug (all six are listed in docs/theming.md)
--theme ./mytheme.theme           # a file: the only way to eyeball a drop-in
--theme ~/themes/mytheme.txt      # any extension; a separator makes it a path
NO_COLOR=1 go run ./cmd/capturetool --fixture sessions-flat
```

`--theme` defaults to `tokyo-night` and takes either a built-in slug or a path to a `.theme` file.
Slug versus path is decided by a path separator **or** the `.theme` suffix, so `nord` is a slug and
`nord.theme`, `./nord.theme` and `/abs/anything.txt` are all paths. A path is an input, not config
discovery: nothing here reads prefs or resolves the themes directory, which is what keeps the
harness's no-real-config guarantee intact.

Invalid input is a hard error with a non-zero exit, never a fallback: silently rendering the wrong
theme at a visual gate is the failure this tool exists to prevent. A file whose *name* the themes
directory would reject (a bad name, or a built-in's slug) warns on stderr and renders anyway, so an
author can see their theme before renaming the file.

There is no `--appearance` flag: a theme is light or dark, so there is no mode left to pin. With
`NO_COLOR` set, the fixture renders the colourless path on the terminal's native background,
whatever `--theme` named, live or printed.

### `--print`

```bash
go run ./cmd/capturetool --fixture sessions-flat --print                         # text, at its frame size
go run ./cmd/capturetool --fixture sessions-flat --print --ansi --size 100x30    # in colour, at a size
```

`--print` draws the fixture once, with no terminal, after its keys are pressed: at its frame's size,
or at `--size WxH`. It prints text alone, which diffs against a frame's `.txt`; with `--ansi`, its
colours too, which `cat` shows beside the frame's `.ansi`. `--size` and `--ansi` take `--print`:
live, the terminal gives the size and the frame is drawn in colour. A fixture or surface that pins a
dimension keeps it whatever `--size` asks.

```bash
diff <(go run ./cmd/capturetool --fixture sessions-flat --print) testdata/vhs/frames/sessions-flat-120x40.txt
```

## Running a tape

From the repository's root, as the tapes' paths are from there:

```bash
vhs testdata/vhs/<name>.tape
```

It writes the PNG its `Screenshot` names.

### Gotcha 1: the sandbox

`vhs` drives a headless browser that connects to `ttyd` over loopback, which an agent's sandbox
refuses:

```
could not open ttyd: ... ERR_CONNECTION_REFUSED
```

Run `vhs` with the sandbox off: in the agent harness, the Bash tool's
`dangerouslyDisableSandbox: true`. Everything else, `go build` and the tests included, runs
sandboxed.

### Gotcha 2: quoted paths

A path with a `/` in a tape must be quoted, or the tape doesn't parse:

```
Output "testdata/vhs/.gifcache/<name>.gif"   # parses
Screenshot "testdata/vhs/<name>.png"         # parses
Output testdata/vhs/.gifcache/<name>.gif     # a parser error
```

### Gotcha 3: `vhs` can fail silently

`vhs` can run a tape, report nothing wrong, and **not write the PNG**. You then judge a stale or
absent image, which reads either as "the change didn't render" or, worse, as a false pass against a
previous capture. A colour change is visible *only* in the image; no assertion anywhere would catch
a capture that never landed.

**Hash the target before and after, confirm the hash changed, and retry on failure**, before
trusting or reviewing any capture:

```bash
shasum -a 256 testdata/vhs/<name>.png     # before (may not exist yet)
vhs testdata/vhs/<name>.tape
shasum -a 256 testdata/vhs/<name>.png     # must differ
```

### Gotcha 4: settings come first

A `Set` after any other command is ignored, and `LetterSpacing` takes no negative value. A tape that
shows no frame fails with `no frames`, so `Show` before the `Screenshot`, and sleep a little either
side of it.

## The calibrated settings

Every tape uses these, the demo tapes' header, for a terminal of C columns and R rows:

```
Set Shell "bash"
Set FontFamily "JetBrains Mono"
Set FontSize 17
Set LetterSpacing 0
Set LineHeight 1
Set Width <C × 10 + 60>
Set Height <R × 22 + 44>
Set Padding 16
Set CursorBlink false
Set Framerate 30
Set Theme { "name": "tokyo-night", "background": "#0B0C14", ... }
```

JetBrains Mono at 17px makes a cell 10 × 22 px. So the picker's 120 × 40 is `Width 1260`,
`Height 924`, and the resume panel's 80 × 24 is `Width 860`, `Height 572`. The demo tapes' test
reads a tape's grid back as ⌊(Width − 57) / 10⌋ × ⌊(Height − 42) / 22⌋ and holds it to the size its
fixture or scenario is drawn at.

- **`LetterSpacing 0`**: `vhs` defaults to 1, a pixel more for every cell.
- **The `Theme`**: copy it from a tape in `demo/`. Its background and cursor are the theme's canvas,
  its foreground the theme's primary text, its selection the theme's `bg.selection`, and it sets the
  scheme's sixteen colours, so a cell the picker leaves unpainted shows the canvas, and the preview's
  palette colours show as the theme has them.
- **The launch**: a tape types `go run ./cmd/capturetool --fixture <name> --theme <slug>` while
  hidden, waits for the first screen with `Wait+Screen` rather than sleeping a guess (the first run
  waits on a build), types the fixture's `captureKeys`, then shows the screen and screenshots it.

## Determinism

The same tape gives the same PNG, run after run. The fixture data is injected in memory: the harness
reads no real config and reaches no zmx session. `--theme` pins a single palette with no light or
dark detection and no first-paint wait, so there is no gate to race.

## The capture tool and its fixtures

```
cmd/capturetool/main.go     # the separate harness binary (package main; NOT a portal subcommand)
internal/capture/           # fakes, fixtures and scenarios (test-only: the portal binary never links it)
  fakes.go                  #   every seam, faked: read seams return canned data; mutators are no-ops
  fixtures.go               #   FixtureByName / FixtureNames + the deterministic fixture data
  frames.go                 #   Drawing (a fixture or surface at its frame size), WriteFrame, Plain
  theme_fake.go             #   the faked theme enumeration a panel fixture declares its rows through
  harness.go                #   builds a fixture's model and replays its captureKeys
  swatch.go                 #   the contrast-validation swatch (a standalone tea.Model; NOT tui.Build)
  resume_surfaces.go        #   the resume panel's standalone surfaces + StandaloneNames (NOT tui.Build)
  scenario.go, scenarios.go #   the README's demo scenarios: portal-cold, portal-tour and themes
  live.go                   #   a scenario played in real time, its keys drawn as keycaps
```

`cmd/capturetool` takes `--fixture`, `--scenario`, `--theme`, `--print`, `--size`, `--ansi` and
`--frames`. Its `run(args, stdout, stderr, getenv)` is the whole command, so its tests drive it as
a user would. Live, `--fixture <name>` runs the production model, `tui.Build(fixture.Deps(theme))`,
on the alt screen: exactly the model and launch shape `cmd/open.go` uses, so the captured frame is
the real TUI.

`--scenario` plays a scenario through the same model in real time, its clock running and its timers
firing, for the README's demos: [demo/README.md](../../demo/README.md) says how. It refuses
`--fixture`, `--print`, `--size`, `--ansi` and `--frames`, and needs a terminal.

Fixtures are deliberately shallow: they do just enough to visualise what is meant to be visualised.
**They are about look, not behaviour**, and need not be functionally complete.

**The deliberate exceptions are the standalone surfaces**, which don't route through `tui.Build`.
`capture.StandaloneNames()` is the one list of them, and the guards that enumerate fixtures consult
it by name, so a new surface joins them all with one edit there:

- the `contrast-validation` swatch: a labelled set of tint bands on the theme's own canvas. It is
  how a new light theme's pinned surface tints get settled by eye.
- the resume panel's two screens, drawn through the same exported renderers a waiting session draws
  with: `resume-panel-waiting` and `resume-panel-discard`, each with a `-report` variant (the card
  carrying its report row) and a `-degraded` variant pinned below the card's size, where the frame
  gives way to a plain stack. Each is pinned to its own size, so a live terminal's window can't
  decide which form is captured, and none sets OSC 11: a session's draw never does.

`FixtureNames()` lists them alongside the picker fixtures; `FixtureByName` doesn't resolve them. All
of them take a `--theme` like everything else.

### Adding (or removing) a fixture

1. **Add the fixture** in `internal/capture/fixtures.go`: write a `*Fixture`-returning builder with
   the canned seam data (sessions, projects, theme rows, cursor position, …), the builder naming its
   own fixture, and add it to the `fixtureBuilders()` list. Both `FixtureByName` and `FixtureNames`
   derive from that one list, so there is no second registry to keep in step. Keep the data fixed and
   invented: determinism is the gate, and nothing personal or naming a tool belongs in it. If the
   frame is reached by a keystroke, declare it in `captureKeys` rather than only in a tape, so the
   tape and the offline driver can't drift. If the frame *is* a geometry, declare the terminal size
   on the fixture.
2. **Write its frame** with `go run ./cmd/capturetool --frames testdata/vhs/frames <name>`, once
   `--print` shows it right, and commit it with the fixture; the tests fail until every fixture keeps
   one. Set its frame size on the fixture if it's designed for one other than `120x40`. The sweeps
   pick it up at the owner's sizes with no edit; a break they find is fixed or listed in
   `knownBreaks` with why.
3. **To judge it by eye,** add a tape `testdata/vhs/<name>.tape` with the calibrated settings, its
   `Width` and `Height` from the fixture's size, modelled on a still's tape in `demo/` but ending in
   a `Screenshot "testdata/vhs/<name>.png"` with its `Output` in `.gifcache/`.
4. **Clear the tape and the PNG at sign-off.** Leave the fixture and its frame.

**Don't delete a fixture to tidy up.** The swap-and-diff completeness guard renders *every* fixture
the harness declares and never names one, so the fixture list **is** the coverage list: removing a
fixture silently shrinks the guard rather than failing it, and the screen it covered goes quietly
unchecked.

## The design reference

`reference/` holds committed PNG exports of the named design frames, kept in-repo so neither
implementation nor review needs a live design-tool connection. Export and commit the frame
**before** implementing against it. Sub-agents have no design-tool access, so re-exporting a changed
frame is the orchestrator's job; there is no agent-runnable command for it.

A capture is compared to its reference for **layout, structure and colour-role match**, judged by
eye, **not a pixel-diff gate**: the golden frames are the cell-for-cell gate, and the reference is
the design they began from. The design export is an HTML approximation (the real terminal uses the
user's font and the theme's own hexes), so an exact pixel diff would always fail. The implementer
self-checks, the reviewer gates, and the human opens both images side by side. Don't read colour
values off a reference frame: the tokens are the contract, the frame is an illustration of them.
