# Portal TUI — visual-capture harness (`vhs`)

This directory is Portal's **permanent visual-test rig**. Every TUI-changing task
screenshots the live TUI through here and compares the capture to a committed
Paper design reference (spec § 15). It is the objective visual gate that lets the
per-task implement → review loop terminate.

The capture mechanism is a **separate harness binary** (`cmd/capturetool`) that
imports Portal's real `internal/tui`, builds the production model via the shared
`tui.Build` constructor, and binds **every tmux seam to an in-memory fake**
(`internal/capture`). It therefore **never opens a tmux server**, never spawns a
daemon, never runs bootstrap, and never touches `~/.config/portal`. The shipped
`portal` binary is untouched: it has no new command and does not import the
harness fakes/fixtures (an import-guard test enforces this).

## Contents

| Path | What |
|---|---|
| `sessions-flat.tape` | vhs tape that drives the `sessions-flat` fixture and screenshots it |
| `sessions-flat.png` | The captured frame (current design) — committed, overwritten in place so "latest" is always current |
| `contrast-validation-{dark,light}.tape` | vhs tapes that drive the `contrast-validation` swatch (the §16.5 lock-in/bail gate, task 1-9) in each owned-canvas mode |
| `contrast-validation-{dark,light}.png` | The captured swatch frames — labelled tint bands the human eyeballs against `#0b0c14` / `#e1e2e7` |
| `LOCK-IN.md` | The committed lock-in/bail record (pinned hexes + derivations + ratios + a PENDING human-eyeball decision section) |
| `trail/<screen>/<phase>-<task>.png` | **Per-task snapshots** of each screen — a committed, browsable visual history (see "Capture trail" below) |
| `reference/sessions-modern-vivid-v2.png` | The committed Paper export of frame **Sessions — Modern Vivid v2** (the build target) — the comparison reference, kept in-repo so no live MCP is needed |
| `.gifcache/*.gif` | Transient vhs byproduct (vhs requires an `Output`); tapes write it into the hidden `.gifcache/` subdir so the dir listing stays clean. **git-ignored**, not committed |

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

## Running a tape

From the **project root** (paths in the tape are repo-root-relative):

```bash
vhs testdata/vhs/sessions-flat.tape
```

This writes `testdata/vhs/sessions-flat.png` (overwriting the committed one).

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
Output "testdata/vhs/.gifcache/sessions-flat.gif" # ✅ quoted
Screenshot "testdata/vhs/sessions-flat.png"       # ✅ quoted
Output testdata/vhs/.gifcache/sessions-flat.gif   # ❌ parser error
```

### Determinism (the acceptance gate)

The gate for this harness is **determinism, not a frame match**: two runs of the
same tape from a clean checkout must produce **byte-comparable** PNGs.

```bash
vhs testdata/vhs/sessions-flat.tape && shasum -a 256 testdata/vhs/sessions-flat.png
vhs testdata/vhs/sessions-flat.tape && shasum -a 256 testdata/vhs/sessions-flat.png
# the two hashes must match (or: cmp the two PNGs)
```

Determinism is load-bearing because the fixture data is injected **in-memory** —
the harness reads no real config and contacts no tmux server, so the only inputs
are the fixed fixture + the fixed font/size/dimensions pinned in the tape.

## Capture trail (per-task history)

The canonical `testdata/vhs/<screen>.png` is the **latest** capture — overwritten
in place so the reviewer/human always open the current frame. But a single
overwritten file destroys the screen's *history* once reskin tasks begin. So each
task that (re)captures a screen also commits a permanent, task-stamped snapshot:

```
testdata/vhs/trail/<screen>/<phase>-<task>.png    # e.g. trail/sessions-flat/1-1.png
```

This gives a committed, browsable evolution of every screen — `1-1` (baseline),
`1-3`, `1-5`, … side by side — without `git` archaeology. The orchestrator copies
the fresh capture into the trail when it commits the task. Parity-only tasks whose
capture is byte-identical to the prior frame reuse it (no new trail entry — the gap
in numbering means "unchanged here"). Open the trail files in any image viewer
(images do not render in the Claude Code terminal).

## The capture tool + fixture design

```
cmd/capturetool/main.go     # the separate harness binary (package main; NOT a portal subcommand)
internal/capture/           # in-memory fakes + named fixtures (imported ONLY by the capture tool)
  fakes.go                  #   every tmux seam, faked: read seams return canned data; mutators are no-ops
  fixtures.go               #   FixtureByName / FixtureNames + the deterministic session sets
  swatch.go                 #   the contrast-validation swatch (a standalone tea.Model; NOT tui.Build)
```

The tool takes `--fixture <name>`, resolves it via `resolveProgram`, and runs the
resulting Bubble Tea model on the alt screen. Most fixtures resolve to the
production model with `tui.Build(fixture.Deps())` — exactly the model and launch
shape `cmd/open.go` uses, so the captured frame is the **real** TUI.

**One deliberate exception:** the `contrast-validation` swatch (the §16.5
lock-in/bail gate, task 1-9) is a standalone validation surface — a labelled set
of tint bands on the owned canvas — that does **not** route through `tui.Build`.
The four light-tint *surfaces* it validates (selection row, separator/footer
borders, warning band, loading track) are built in later phases; the swatch
validates the colour TOKENS before those phases invest in the surfaces
(anti-sunk-cost). It is driven by `--appearance dark|light` to pin the owned
canvas, identically to the other fixtures.

### Adding a new fixture / screen

1. **Add the fixture** in `internal/capture/fixtures.go`: add a `case "<name>"`
   to `FixtureByName`, add `"<name>"` to `FixtureNames`, and write a
   `*Fixture`-returning builder with the canned seam data (sessions, projects,
   etc.). Keep the data fixed — determinism is the gate.
2. **Add a tape** `testdata/vhs/<name>.tape` modelled on `sessions-flat.tape`:
   set the same `FontFamily "JetBrains Mono"` + fixed `Width`/`Height`, launch
   `go run ./cmd/capturetool --fixture <name>`, `Sleep` for first paint (and send
   any keys needed to reach the target screen), then `Screenshot "<...>.png"`.
3. **Commit the reference** under `reference/<frame>.png` (see below) and the
   captured `<name>.png`.

The fixture set lives entirely in `internal/capture`, which the `portal` binary
must never import — keep it that way (the import-guard test in
`cmd/capturetool/import_guard_test.go` will fail if production grows a dependency
on it).

## The Paper design reference

The comparison reference is a **committed PNG export of the task's named Paper
frame** (spec § 15.1 / § 15.5) — kept in-repo (`reference/`) so neither
implementation nor CI needs a live `paper` MCP.

`reference/sessions-modern-vivid-v2.png` is the **Sessions — Modern Vivid v2**
frame (860×680 @2x). **This task ships the pre-reskin capture against it** —
`sessions-flat.png` is the current, un-reskinned baseline and is NOT expected to
match the Paper frame yet; later reskin tasks converge it.

### Refreshing the reference (orchestrator-run)

Sub-agents have **no `paper` MCP access**. When a frame changes in Paper, the
**orchestrator** re-exports and re-commits it via the `paper` MCP — exporting the
frame by its node-id (`get_screenshot` / export) and overwriting
`reference/<frame>.png`. There is no agent-runnable command for this step.

## How the comparison is judged

The capture is compared to the Paper reference for **layout, structure, and
colour-role match** — **agent/user-judged, NOT a pixel-diff CI gate**. Paper is an
HTML approximation (the real terminal uses the user's font + the §2.9 token
hexes), so an exact pixel diff would always fail. The implementer self-checks, the
reviewer gates, and the human opens both images side by side (spec § 15.4 / §
15.5). The only automated gate here is the **determinism** check above.
