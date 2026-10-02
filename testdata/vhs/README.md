# Portal TUI — visual-capture harness (`vhs`)

This directory is where the TUI gets **looked at**. A task that changes what the
picker renders writes a `vhs` tape here, screenshots the live TUI through it, and
judges the frame — against a committed design reference where one exists. It is
what lets the implement → review loop terminate on something visual rather than on
an assertion about a string.

The capture mechanism is a **separate harness binary** (`cmd/capturetool`) that
imports Portal's real `internal/tui`, builds the production model via the shared
`tui.Build` constructor, and binds **every tmux seam to an in-memory fake**
(`internal/capture`). It therefore **never opens a tmux server**, never spawns a
daemon, never runs bootstrap, and never touches `~/.config/portal`. The shipped
`portal` binary is untouched: it has no new command and does not import the
harness fakes/fixtures (an import-guard test enforces this).

This matters more than it looks: **Portal cannot be run from a scratch build to
check a visual change** — a scratch build disturbs the running daemon and its
bootstrap touches real state. The harness is the only route to seeing a change
before release.

## What lives here, and for how long

**PNGs and the tapes that render them are scaffolding, not a durable asset.**
There is no visual-regression obligation — nothing diffs a capture against a
committed baseline — so a permanent image set would only be a directory of files
that rot the first time a colour token is renamed. The rule:

> A capture and its tape are **created as work proceeds**, **committed while the
> work is being collaborated on** (so the reviewer and the human open the same
> frame the implementer did), and **cleared out at sign-off**.

Sign-off means the *feature's* sign-off, not a phase's — work that re-captures a
screen in a later phase owns clearing what it re-captured.

So an empty-looking directory is the normal resting state. If you are here to
change a screen, you are expected to add a tape, capture, and take it away again
when the work lands.

| Path | What | Lifetime |
|---|---|---|
| `<fixture>.tape` | The `vhs` tape that drives one fixture and screenshots it | Scaffolding — cleared at sign-off |
| `<fixture>.png` | The captured frame | Scaffolding — cleared at sign-off |
| `reference/*.png` | Committed design exports — the frames the code was built *against* | **Kept** |
| `LOCK-IN.md` | The light-tint lock-in record — pinned hexes, derivations, contrast ratios and the eyeball decision that settled them. A historical record; the captures it names were cleared long ago | **Kept** |
| `.gifcache/*.gif` | Transient `vhs` byproduct (`vhs` requires an `Output`); tapes write it into the hidden subdir so the listing stays clean | **git-ignored**, never committed |

**Why `reference/` is exempt.** A capture is a render *of the code*, which is why
it rots and why the rule above exists. A reference frame is the **design** the code
was built against — exported and committed *before* implementation so the
implementer and the reviewer could self-check against it. It does not go stale the
way a capture does; it is a record of what was specified, and for some screens it is
the only reference that exists. Keep them — and note that no Go source cites them
by path, so their retention rests on this table rather than on an inbound
reference.

**The fixtures and the harness are permanent.** The Go fixture definitions in
`internal/capture`, `cmd/capturetool` and the `vhs` route all stay. Only the images
and tapes are transient. Deleting a fixture is a different act entirely, with a
consequence — see "Adding (or removing) a fixture" below.

## One-time setup: install + verify `vhs`

`vhs` needs two companion tools, `ttyd` (headless terminal) and `ffmpeg` (frame
encoding), plus a headless Chrome (its rod-based renderer) on `PATH`.

**Homebrew (macOS / Linuxbrew)** — pulls `ttyd` + `ffmpeg` as dependencies:

```bash
brew install vhs
```

**Non-Homebrew** — install each tool separately, then `vhs`:

```bash
# ttyd + ffmpeg via your distro package manager, e.g.:
#   apt install ttyd ffmpeg     (Debian/Ubuntu)
#   dnf install ttyd ffmpeg     (Fedora)
go install github.com/charmbracelet/vhs@latest
```

**Verify** (all three must resolve):

```bash
vhs --version      # e.g. vhs version 0.11.0
ttyd --version
ffmpeg -version
```

## Viewing a screen live

No tape needed to just *look* at something:

```bash
go run ./cmd/capturetool --fixture sessions-flat
go run ./cmd/capturetool --fixture theme-panel-confirm --theme nord
```

`capture.FixtureNames()` lists every fixture; an empty or unknown `--fixture` errors
with the list. **The tool replays no keys of its own**, so a screen reached by a
keystroke — the theme slide-over, the Projects page, the preview — is reached by
pressing that key yourself. The tape types it for you; live, you do. (A fixture
declares that sequence in `captureKeys`, which is what the offline driver replays.)

### `--theme <slug|path>` (default `tokyo-night`)

One flag pins the palette, and it takes **either** a built-in slug **or an explicit
path to a `.theme` file**:

```bash
--theme nord                      # a built-in: tokyo-night, tokyo-night-day, nord
--theme ./mytheme.theme           # a file — the only way to eyeball a drop-in
--theme ~/themes/mytheme.txt      # any extension; a separator makes it a path
```

Slug versus path is decided by a path separator **or** the `.theme` suffix, so
`nord` is a slug and `nord.theme`, `./nord.theme` and `/abs/anything.txt` are all
paths. A path is an **input, not config discovery** — nothing here reads prefs or
resolves the themes directory, which is what keeps the harness's no-real-config
guarantee intact.

**Invalid input is a hard error with a non-zero exit, never a fallback** — silently
rendering the wrong theme at a visual gate is the failure this tool exists to
prevent. A file whose *name* would be rejected by the themes directory (bad name, or
a built-in's slug) warns on stderr and renders anyway, so an author can see their
theme before renaming the file.

There is no `--appearance` flag: a theme **is** light or dark, so there is no mode
left to pin. Setting `NO_COLOR=1` in the environment renders the colourless path on
the terminal's native background, whatever `--theme` named.

## Running a tape

From the **project root** (paths in the tape are repo-root-relative):

```bash
vhs testdata/vhs/<name>.tape
```

This writes the `Screenshot` path the tape names.

### Gotcha 1 — sandbox / loopback networking

`vhs` drives a headless browser that connects to a local `ttyd` over **loopback
networking**. In a restricted sandbox (e.g. an agent's default sandbox) that
connection is refused:

```
could not open ttyd: ... ERR_CONNECTION_REFUSED
```

**Fix:** run `vhs` with the sandbox disabled. Inside the agent harness that means
the Bash tool's `dangerouslyDisableSandbox: true`. Ordinary `go build` / `go test`
run fine sandboxed — only the `vhs` invocation needs loopback access.

### Gotcha 2 — quoted slashed paths

vhs tape paths that contain a `/` **must be quoted**, or the tape parser errors:

```
Output "testdata/vhs/.gifcache/<name>.gif" # ✅ quoted
Screenshot "testdata/vhs/<name>.png"       # ✅ quoted
Output testdata/vhs/.gifcache/<name>.gif   # ❌ parser error
```

### Gotcha 3 — vhs fails silently on write

`vhs` will run the tape, report no error, and **not produce the PNG**. You then
pixel-check a stale or absent image, which reads either as "the change didn't
render" or — worse — as a false pass against a previous capture. A colour change is
visible *only* in the image; no assertion anywhere would catch a capture that never
landed.

**Hash the target before and after, confirm the hash changed, and retry on
failure**, before trusting or reviewing any capture:

```bash
shasum -a 256 testdata/vhs/<name>.png     # before (may not exist yet)
vhs testdata/vhs/<name>.tape
shasum -a 256 testdata/vhs/<name>.png     # must differ
```

### Determinism

Two runs of the same tape from a clean checkout must produce **byte-comparable**
PNGs. Determinism is load-bearing because the fixture data is injected
**in-memory** — the harness reads no real config and contacts no tmux server — and
because `--theme` pins a single palette with no light/dark detection and no
first-paint wait, so there is no gate to race.

## The capture tool + fixture design

```
cmd/capturetool/main.go     # the separate harness binary (package main; NOT a portal subcommand)
internal/capture/           # in-memory fakes + named fixtures (imported ONLY by the capture tool)
  fakes.go                  #   every tmux seam, faked: read seams return canned data; mutators are no-ops
  fixtures.go               #   FixtureByName / FixtureNames + the deterministic fixture data
  theme_fake.go             #   the faked theme enumeration a panel fixture declares its rows through
  harness.go                #   builds a fixture's model and replays its captureKeys
  swatch.go                 #   the contrast-validation swatch (a standalone tea.Model; NOT tui.Build)
  resume_surfaces.go        #   the resume panel's standalone surfaces + StandaloneNames (NOT tui.Build)
```

The tool takes `--fixture <name>`, resolves it via `resolveProgram`, and runs the
resulting Bubble Tea model on the alt screen. Most fixtures resolve to the
production model with `tui.Build(fixture.Deps(theme))` — exactly the model and
launch shape `cmd/open.go` uses, so the captured frame is the **real** TUI.

Fixtures are deliberately shallow: they do just enough to visualise what is meant to
be visualised. **They are about look, not behaviour**, and need not be functionally
complete.

**The deliberate exceptions are the standalone surfaces**, which do **not** route
through `tui.Build`. `capture.StandaloneNames()` is the one list of them, and the
guards that enumerate fixtures consult it by name — a new surface joins them all
with one edit there:

- the `contrast-validation` swatch — a labelled set of tint bands on the theme's own
  canvas. It is how a new light theme's pinned surface tints get settled by eye.
- the resume panel's two screens, drawn through the same exported renderers a
  waiting pane draws with: `resume-panel-waiting` and `resume-panel-discard`, each
  with a `-report` variant (the card carrying its report row) and a `-degraded`
  variant pinned below the card's size, where the frame gives way to a plain stack.
  Each is pinned to its own size, so a live terminal's window cannot decide which
  form is captured, and none sets OSC 11 — a pane draw never does.

`FixtureNames()` lists them alongside the picker fixtures; `FixtureByName` does not
resolve them. All of them take a `--theme` like everything else.

### Adding (or removing) a fixture

1. **Add the fixture** in `internal/capture/fixtures.go`: write a
   `*Fixture`-returning builder with the canned seam data (sessions, projects, theme
   rows, cursor position, …) — the builder names its own fixture — and add it to the
   `fixtureBuilders()` list. Both `FixtureByName` and `FixtureNames` derive from that
   one list, so there is no second registry to keep in step. Keep the data fixed —
   determinism is the gate. If the frame is reached by a keystroke, declare it in
   `captureKeys` rather than only in the tape, so the tape and the offline driver
   cannot drift.
2. **Add a tape** `testdata/vhs/<name>.tape` modelled on an existing one (see git
   history — the directory is normally empty). Set `FontFamily "JetBrains Mono"` +
   `FontSize 16`, fix `Width`/`Height` (vhs sizes in **pixels**, so record the
   resulting column/row count in a comment). If the frame *is* a geometry, declare
   the terminal size on the fixture rather than only in the tape, so the tape and
   the offline driver cannot drift — the tape then sizes its terminal to match the
   declared columns/rows. `Set Shell "bash"` for a deterministic
   prompt, launch `go run ./cmd/capturetool --fixture <name> --theme <slug>`, `Sleep`
   for the compile + first paint, type any keys, then `Screenshot "<...>.png"`.
3. **Clear the tape and the PNG at sign-off.** Leave the fixture.

**Do not delete a fixture to tidy up.** The swap-and-diff completeness guard renders
*every* fixture the harness declares and never names one, so the fixture list **is**
the coverage list — removing a fixture silently shrinks the guard rather than failing
it, and the screen it covered goes quietly unchecked.

The fixture set lives entirely in `internal/capture`, which the `portal` binary must
never import — keep it that way (the import-guard test in
`cmd/capturetool/import_guard_test.go` will fail if production grows a dependency on
it).

## The design reference

`reference/` holds committed PNG exports of the named design frames, kept in-repo so
neither implementation nor review needs a live design-tool connection. Export and
commit the frame **before** implementing against it.

Sub-agents have no design-tool access, so re-exporting a changed frame is the
orchestrator's job — there is no agent-runnable command for it.

## How the comparison is judged

The capture is compared to its reference for **layout, structure, and colour-role
match** — **agent/user-judged, NOT a pixel-diff gate**. The design export is an HTML
approximation (the real terminal uses the user's font and the theme's own hexes), so
an exact pixel diff would always fail. The implementer self-checks, the reviewer
gates, and the human opens both images side by side. Do not read colour values off a
reference frame: the tokens are the contract, the frame is an illustration of them.
