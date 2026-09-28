TASK: The Theme the Panel Draws In (lazy-resume-on-attach-3-5, tick-ce5f67) — a non-Bubble-Tea OSC 11 probe in internal/tui that resolves which palette a pane draw paints in.

ACCEPTANCE CRITERIA:
- A constant nomination returns its palette with nothing written to the writer and no reader touched; a zero nomination returns the zero theme the same way.
- `colourless` runs no detection and writes nothing, whatever the nomination's shape.
- An adaptive pair writes exactly `ansi.RequestBackgroundColor` — once — and returns the light member for a light reply and the dark member for a dark one.
- The timeout is read from `appearanceDetectTimeout`, the same constant the picker's gate uses, rather than a second copy; a probe whose reply never arrives returns dark once that duration has elapsed.
- A reply that arrives after the deadline never changes the answer already returned.
- No answer, a truncated reply, an unparseable payload, a read error, a stdin that is not a terminal, and a `makeRaw` failure all return the dark member — and the two that happen before the write make no write.
- The restore closure runs on every path that reached raw mode, including the timeout, the read error and the successful read, and it closes the reader it opened.
- The production reader takes a read deadline against a real terminal and times a read out at the duration set — asserted over a pty slave rather than a pipe.
- A reader whose `SetReadDeadline` is unsupported returns dark without reading and without blocking, and that branch is reachable only through a seam a test binds — no production path takes it.
- An `openReader` that fails returns the dark member and writes nothing, exactly as a non-terminal does.
- `internal/tui` calls `ansi.SetBackgroundColor` from exactly one non-test file, `restore.go`, with the guard failing on a second call site.

STATUS: issues_found

SPEC CONTEXT: The panel paints the active theme. A named theme paints from frame one with no gate; a light/dark pair runs the picker's detect-or-timeout gate (same `appearanceDetectTimeout`) in the drawing process, resolving dark on no answer — the common case, since most panes are drawn at restore with nobody attached. NO_COLOR runs no detection at all. Because the pane has no program loop, the reply is read under a deadline and abandoned; a late reply stays in the pane's input queue, so the input drop must run after the query, never before (this is why the function later grew its `dropInput` parameter). A pane must never write an OSC 11 set — only the picker's exit restore does.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/pane_appearance.go:18 (`paneTTYPath = "/dev/tty"`), :28-32 (`paneReader` seam), :37-43 (probe seams struct), :45-53 (production constructor: os.Stdout, `/dev/tty` via `openTTYPath`, `term.IsTerminal(os.Stdin.Fd())`, `makeStdinRaw`, `appearanceDetectTimeout`)
  - internal/tui/pane_appearance.go:65-85 (`ResolvePaneTheme` → `resolvePaneTheme` → `selectPaneTheme`: constant first, zero/colourless → `n.Select(MemberDark)`, which yields the constant for a constant and `Theme{}` for a zero nomination, else `n.Select(p.detect())`)
  - internal/tui/pane_appearance.go:87-116 (`detect`: non-terminal → dark; open failure → dark; reader close deferred before `makeRaw`; `makeRaw` failure → dark; restore deferred; write query; set deadline, dark on failure; read; classify via `ansi.XParseColor` + `terminalReplyFrom`)
  - internal/tui/pane_appearance.go:118-153 (bounded read to `paneReplyCap`, terminator on BEL, C1 ST, or ESC-backslash; a split ESC keeps reading rather than failing)
  - go.mod:11 (`github.com/charmbracelet/x/term v0.2.2` now a direct requirement)
- Notes:
  - The signature is now `ResolvePaneTheme(n, colourless, dropInput func() error) (theme.Theme, error)` (pane_appearance.go:65), not the plan's two-argument form. A later task extended it so the input drop runs strictly after the appearance query resolves. The change is sound: it enforces the spec's ordering rule, and the palette is returned alongside the drop's error. The only production caller is cmd/state_resume_draw.go:88.
  - The reader close sits in its own `defer` (pane_appearance.go:95) instead of inside the restore closure. It is registered before `makeRaw`, so the reader is closed on the `makeRaw`-failure path as well. That is an equivalent or better arrangement.
  - The nomination construction guard exempts this file's single `os.OpenFile` (internal/tui/nomination_test.go:229-231). internal/tui/pane_appearance_guard_test.go:113-143 narrows that exemption to the opener's own parameter, with `openTTYPath` pinned to `paneTTYPath`. The exemption cannot be widened without a guard noticing.
  - The classification reuses the picker's `terminalReplyFrom` (internal/tui/theme_state.go:19). `XParseColor` returns nil for an unparseable payload, and `isDarkColor` treats nil as dark, so there is one fallback route as planned.

TESTS:
- Status: Adequate
- Coverage: Every test named in the plan is present:
  - The constant, zero and NO_COLOR table, with no opens and no writes (pane_appearance_test.go:185-245).
  - Pair member selection through `resolvePaneTheme` (:247-273).
  - The query written exactly once, compared against a spelled-out literal so an OSC 11 set could not pass (:18, :393-400).
  - Light and dark replies, with BEL and ST terminators (:402-423).
  - No answer (:425-444) and a truncated/unparseable/garbage table (:446-463).
  - Read failure (:465-473), non-terminal (:475-487), raw mode refused with the reader still closed (:489-501), open failure with no raw mode entered (:503-514).
  - Refused deadline, run off-goroutine with `reads == 0` (:516-538), and a late reply (:540-555).
  - A restore-and-close table across seven outcomes (:557-585).
  - Production wiring: timeout equals `appearanceDetectTimeout`, and every seam is set (:588-609).
  - The darwin pty-slave deadline test (pane_appearance_realtty_test.go:21-44) and the SetBackgroundColor guard with a tripwire that requires restore.go to still call it (pane_appearance_guard_test.go:27-54).
  - Tests added by later tasks cover drop ordering and error return (pane_appearance_test.go:275-389).
- Notes: The no-answer test's upper-bound assertion on the deadline can never fail (see FINDINGS). No over-testing worth reporting.

CODE QUALITY:
- Project conventions: Followed. Seams are small function fields, no `t.Parallel`, source guards go through `sourceguardtest`, and there are no raw hex values in production code.
- SOLID principles: Good. Nomination selection, probe I/O and reply parsing are separate functions, and the terminal dependencies are injected.
- Complexity: Low
- Modern idioms: Yes (`bytes.Cut`, `slices`, `errors.Is`)
- Readability: Good. The comments state reasons, and each one I checked is true of the code.
- Issues: None beyond the test finding below.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] internal/tui/pane_appearance_test.go:441 — The deadline-bound assertion `time.Until(h.reader.deadlines[0]) > testProbeTimeout` runs after `detect()` has already returned. In the no-reply case `detect()` only returns once the deadline has passed, so `remaining` is always zero or negative and the check can never fail, whatever deadline the probe set. The fix is to measure the offset when the deadline is set: have `fakePaneReader.SetReadDeadline` record `time.Until(t)` alongside `t`, and assert that recorded offset is `<= testProbeTimeout`. Alternatively, add an upper bound on `elapsed` with scheduling slack. — FAILS: a probe that sets its read deadline further out than `p.timeout` (e.g. `p.timeout*10`) passes this test, and the production-wiring test and the duration source guard both pass too. The panel would then wait on a silent terminal far longer than the picker's timeout, which is the drift the criterion exists to prevent, and the test's own error message claims a bound it does not enforce.

UNSETTLED:
- "The production reader takes a read deadline against a real terminal and times a read out at the duration set — asserted over a pty slave rather than a pipe, because a pipe takes a deadline whatever reader shape is bound." — Run `go test ./internal/tui -run TestProductionReader_BoundsAReadAgainstARealTerminal` on darwin to confirm it passes. Separately, the test opens the pty slave's own device node (e.g. /dev/ttysNNN), while production opens the literal `/dev/tty`. On darwin that is the controlling-tty indirection device, and kqueue behaviour differs there: muesli/cancelreader v0.2.2 `cancelreader_bsd.go` special-cases it with "kqueue returns instantly when polling /dev/tty". To settle the criterion for the production path, observe from a process whose controlling terminal is a tmux pane on darwin that `os.OpenFile("/dev/tty", O_RDONLY, 0)` accepts `SetReadDeadline` and delivers an OSC 11 reply that arrives before the deadline. For example: a `portal state resume-draw` under an adaptive pair on a light terminal paints the light member and leaves no reply bytes in the pane's input queue.
- "A reader whose `SetReadDeadline` is unsupported returns dark without reading and without blocking, and that branch is reachable only through a seam a test binds — no production path takes it." — The first half is settled by reading (pane_appearance.go:108-110, test :516-538). "No production path takes it" rests on the same observation as above: if kqueue registration of `/dev/tty` fails on darwin, Go falls back to a blocking descriptor, `SetReadDeadline` returns "file type does not support deadline", and every production draw would take this branch.
