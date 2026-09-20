# Review Tracking: Lazy Resume On Attach - Integrity

## Findings

### 1. Every waiting pane paints dark whatever the terminal is, and the terminal's own reply backs the discard confirmation out

**Severity**: Critical
**Plan Reference**: Phase 3, task `lazy-resume-on-attach-3-5` (The theme the panel draws in); consumed by Phase 4 task `lazy-resume-on-attach-4-1` and Phase 5 task `lazy-resume-on-attach-5-2`
**Category**: Acceptance Criteria Quality / Task Self-Containment
**Move**: settled
**Change Type**: update-task

**Problem**:
A user on a light/dark pair gets the dark palette in every waiting pane, on a light terminal, forever — the panel never once detects the terminal it is painted on. The probe is wired to read the terminal's reply from `os.Stdin`, and `os.Stdin` cannot be given a read deadline when it is a terminal: measured on this machine (darwin, Go 1.27.1) against a real pty slave, `os.NewFile` over a blocking tty descriptor — which is exactly how the runtime builds `os.Stdin` — answers `SetReadDeadline` with `file type does not support deadline`, while the same tty opened through `os.OpenFile` takes the deadline and times the read out at the duration set. The task's own fallback then fires on every production draw: "if setting it is unsupported, return dark without reading rather than blocking".

The second half is worse than a wrong colour. The query has already been written by then, so the reply is never consumed and sits in the pane's input queue, where the waiter reads it as keystrokes. On the waiting panel the reply's bytes are swallowed; on the discard confirmation the reply's leading `ESC` is the cancel key, so a confirmation the user just opened backs itself out — and the input drop that exists to keep stale bytes off that screen runs *before* the probe writes, so it cannot catch them.

None of this is visible from the plan's tests. Every probe test drives the seams "over an `os.Pipe` pair", and a pipe takes a deadline whatever reader shape is bound — so all fourteen tests pass over the one reader shape that can never fail, while the shape production actually binds is never exercised.

**Proposal**:
Bind the production reader to a fresh `/dev/tty` open rather than `os.Stdin`, and pin the deadline against a real terminal instead of a pipe. The measurement determines it: of the two reader shapes available, only the `os.OpenFile` one takes a deadline on a tty, and for the process drawing into a pane `/dev/tty` *is* that pane's pty. It keeps the mechanism the task already commits to — a deadline-bounded read, with the swallow window bounded by the detect timeout — rather than trading it for a goroutine race whose abandoned reader would widen that window past the timeout the task states. The deadline-unsupported branch stays as a fallback but stops being the production path, and one pty-backed criterion holds it there, because a pipe cannot.

**Current**:
```markdown
- Add the probe behind a struct of seams — `out io.Writer`, a reader exposing `Read` plus `SetReadDeadline`, `isTerminal func() bool`, `makeRaw func() (restore func(), err error)`, and `timeout time.Duration` defaulted from `appearanceDetectTimeout` — with the production constructor binding `os.Stdout`, `os.Stdin`, `term.IsTerminal(os.Stdin.Fd())` and `term.MakeRaw`/`term.Restore`, over `github.com/charmbracelet/x/term`. That package is already in the module graph as an indirect requirement of the Bubble Tea stack and carries the pointer-sized descriptor shape these calls are written against, so this edit promotes it to a direct requirement rather than adding a module; it is also where the chain's later terminal calls come from — the waiter's raw-mode entry and the draw's size read alike — so one package serves all of them.
- Probe body: return dark without writing when `isTerminal` is false or `makeRaw` fails; `defer` the restore so it runs on every path; write `ansi.RequestBackgroundColor`; set the read deadline and, if setting it is unsupported, return dark without reading rather than blocking; read until a `BEL` or `ST` terminator, a bounded byte cap, the deadline or an error.
```
```markdown
- Cover the probe in `internal/tui/pane_appearance_test.go` driving the seams over an `os.Pipe` pair: a scripted dark reply, a light reply, no reply at all, a truncated reply, an unparseable payload, a read error, a non-terminal, a failed `makeRaw`, and a reply arriving after the deadline — each asserting the answer, whether anything was written, and that the restore ran.
```
```markdown
- [ ] The restore closure runs on every path that reached raw mode, including the timeout, the read error and the successful read.
```
```markdown
- `"it resolves dark and writes nothing when raw mode cannot be entered"`
```
```markdown
- A constant nomination writes nothing to the terminal and paints from the first frame — the gate is never consulted, exactly as the picker's `newNominationGate` pins a constant resolved.
```
Phase 3 task table, row `lazy-resume-on-attach-3-5`:
```markdown
| lazy-resume-on-attach-3-5 | The theme the panel draws in | a constant nomination writes nothing to the terminal and paints from the first frame, the pair's query races the picker's own timeout constant rather than a second copy, the first to resolve wins and a late reply never flips a resolved answer, no answer an unparseable reply or a stdout that is not a terminal all resolve dark by the same route, a pane drawn with no client attached resolves dark with no second rule, NO_COLOR runs no detection at all and writes no query, the terminal's mode is restored on every path including the timeout and a read failure, no OSC 11 set is ever written from a pane draw |
```

**Proposed Text**:
```markdown
- Add the probe behind a struct of seams — `out io.Writer`, `openReader func() (paneReader, error)` where `paneReader` exposes `Read`, `SetReadDeadline` and `Close`, `isTerminal func() bool`, `makeRaw func() (restore func(), err error)`, and `timeout time.Duration` defaulted from `appearanceDetectTimeout` — with the production constructor binding `os.Stdout`, `term.IsTerminal(os.Stdin.Fd())` and `term.MakeRaw`/`term.Restore`, over `github.com/charmbracelet/x/term`. That package is already in the module graph as an indirect requirement of the Bubble Tea stack and carries the pointer-sized descriptor shape these calls are written against, so this edit promotes it to a direct requirement rather than adding a module; it is also where the chain's later terminal calls come from — the waiter's raw-mode entry and the draw's size read alike — so one package serves all of them.
- The production `openReader` is `os.OpenFile("/dev/tty", os.O_RDONLY, 0)` and **not** `os.Stdin`. Measured on darwin against a real pty slave, Go 1.27.1: `os.NewFile` over a blocking tty descriptor — which is how the runtime builds `os.Stdin` — refuses `SetReadDeadline` with `file type does not support deadline`, while the same tty opened through `os.OpenFile` takes the deadline and times the read out at the duration set. Binding `os.Stdin` would send every production probe down the deadline-unsupported branch, resolving dark on every draw whatever the terminal answered and leaving the reply in the pane's input queue for the waiter to read as keystrokes. For the process drawing into a pane, `/dev/tty` is that pane's own pty; an open that fails resolves dark without writing, by the same route a non-terminal does. Raw mode stays on stdin's descriptor: the mode is a property of the terminal rather than of a descriptor onto it, so the reader sees it either way and nothing needs a second `MakeRaw`.
- Probe body: return dark without writing when `isTerminal` is false, when `openReader` fails, or when `makeRaw` fails; `defer` the restore — which closes the reader as well as restoring the mode — so it runs on every path; write `ansi.RequestBackgroundColor`; set the read deadline and, if setting it is unsupported, return dark without reading rather than blocking; read until a `BEL` or `ST` terminator, a bounded byte cap, the deadline or an error.
```
```markdown
- Cover the probe in `internal/tui/pane_appearance_test.go` driving the seams over an `os.Pipe` pair: a scripted dark reply, a light reply, no reply at all, a truncated reply, an unparseable payload, a read error, a non-terminal, a failed `openReader`, a failed `makeRaw`, a reader that refuses a deadline, and a reply arriving after the deadline — each asserting the answer, whether anything was written, and that the restore ran.
- Add `internal/tui/pane_appearance_realtty_test.go` (`//go:build darwin`) covering the production `openReader` alone against a real terminal: open a pty through `/dev/ptmx` plus the host's grant/unlock/name ioctls, open its slave the way the production constructor opens `/dev/tty`, and assert `SetReadDeadline` returns nil and a read of it returns a timeout at the duration set rather than blocking. A pipe takes a deadline whatever reader shape is bound, so it cannot fail this; the linux arm is the same `os.OpenFile` call and is covered through the seam.
```
```markdown
- [ ] The restore closure runs on every path that reached raw mode, including the timeout, the read error and the successful read, and it closes the reader it opened.
- [ ] The production reader takes a read deadline against a real terminal and times a read out at the duration set — asserted over a pty slave rather than a pipe, because a pipe takes a deadline whatever reader shape is bound.
- [ ] A reader whose `SetReadDeadline` is unsupported returns dark without reading and without blocking, and that branch is reachable only through a seam a test binds — no production path takes it.
- [ ] An `openReader` that fails returns the dark member and writes nothing, exactly as a non-terminal does.
```
```markdown
- `"it resolves dark and writes nothing when raw mode cannot be entered"`
- `"it resolves dark and writes nothing when the terminal cannot be opened"`
- `"it resolves dark without blocking when the reader refuses a deadline"`
- `"it bounds a read against a real terminal"` (pty slave, darwin-tagged)
```
```markdown
- A constant nomination writes nothing to the terminal and paints from the first frame — the gate is never consulted, exactly as the picker's `newNominationGate` pins a constant resolved.
- The reply is read from a fresh `/dev/tty` open rather than from `os.Stdin`, because `os.Stdin` refuses a read deadline on a terminal — measured. A probe bound to it would answer dark on every draw whatever the terminal said, and would leave the terminal's reply in the pane's input queue, where the waiter reads it as keystrokes; on the discard confirmation the reply's leading `ESC` is the cancel key, so the confirmation would back itself out, and the input drop that guards that screen runs before the probe writes and so cannot catch it. The two reader shapes are indistinguishable over a pipe, which is why the deadline is pinned against a real terminal.
```
Phase 3 task table, row `lazy-resume-on-attach-3-5`:
```markdown
| lazy-resume-on-attach-3-5 | The theme the panel draws in | a constant nomination writes nothing to the terminal and paints from the first frame, the reply is read from a fresh `/dev/tty` open because `os.Stdin` refuses a read deadline on a terminal, the pair's query races the picker's own timeout constant rather than a second copy, the first to resolve wins and a late reply never flips a resolved answer, no answer an unparseable reply or a stdout that is not a terminal all resolve dark by the same route, a pane drawn with no client attached resolves dark with no second rule, NO_COLOR runs no detection at all and writes no query, the terminal's mode is restored on every path including the timeout and a read failure, no OSC 11 set is ever written from a pane draw |
```

**Resolution**: Fixed — task 3-5's probe gains an `openReader` seam whose production binding is `os.OpenFile("/dev/tty", os.O_RDONLY, 0)` rather than `os.Stdin`, with the measurement stated in line; the body returns dark without writing on a failed open and the restore closes the reader; a darwin-tagged `pane_appearance_realtty_test.go` pins the deadline against a real pty; three criteria, four test names and one edge case follow. Phase 3's task-table row carries the same clause. Tick body re-synced and byte-verified.
**Notes**: Reproduced the measurement independently before applying, on a real pty slave on this machine (darwin, Go 1.27.1): `os.NewFile(tty.Fd())` answers `SetReadDeadline` with `file type does not support deadline`, while `os.OpenFile(tty.Name(), os.O_RDONLY, 0)` accepts it and the subsequent read returns `i/o timeout` at 52ms against a 50ms budget. The pty was opened and closed inside the probe program; no tmux server and nothing on the developer's machine was touched.

---

### 2. The burst rule the waiting panel states is the opposite of the one the discard guard is built on

**Severity**: Minor
**Plan Reference**: Phase 4, task `lazy-resume-on-attach-4-2` (The waiting process: two keys, everything else swallowed)
**Category**: Acceptance Criteria Quality
**Move**: settled
**Change Type**: update-task

**Problem**:
An acceptance criterion tells the implementer the waiter consumes the rest of a burst ("the `y` is discarded"), and the task's own edge case tells them the opposite ("Bytes left unread after an answer are inherited by whatever the handover execs"). Built to the criterion, the waiter drains the pane's input queue after every answer: a user who types ahead while pressing Enter loses those keystrokes into the resumed command, and the input drop Phase 5 exists to perform becomes a guard against a hazard that no longer occurs — so the one screen where a stale byte can destroy something is protected by a mechanism whose premise has been quietly removed. Neither the criterion's test nor any other test in the plan would notice: the test only counts hand-offs.

**Proposal**:
State the criterion as the behaviour the Do already pins ("one-byte buffer per iteration, never buffering ahead") and the edge case already declares: the `y` is left unread, not consumed. The test name follows it, and the test gains something it can actually check — that the remaining byte is still readable from the seam after the hand-off. Nothing else in the task moves; the edge case is already correct.

**Current**:
```markdown
- [ ] A burst delivering `dy` in one write produces exactly one hand-off (the `d`) and the `y` is discarded, because nothing on this screen acts on `y`.
```
```markdown
- `"it acts on the first acting key in a burst and swallows the rest"`
```

**Proposed Text**:
```markdown
- [ ] A burst delivering `dy` in one write produces exactly one hand-off (the `d`) and leaves the `y` unread — still readable from the reader after the hand-off — because the loop takes the byte it dispatched and no further, so whatever the hand-off execs inherits the rest.
```
```markdown
- `"it acts on the first acting key in a burst and leaves the rest unread"`
```

**Resolution**: Fixed — task 4-2's burst criterion now says the `y` is left unread and still readable from the reader after the hand-off, and the test name follows it. The edge case was already correct and is unchanged; phase 4's task table never carried the wrong wording. Tick body re-synced and byte-verified.
**Notes**: The criterion was the only place stating the opposite rule, so the fix is one criterion and one test name.

---

### 3. Phase 3's visual gate asks for a width the capture surfaces do not offer

**Severity**: Minor
**Plan Reference**: Phase 3, Acceptance (planning.md); delivered by task `lazy-resume-on-attach-3-6` (Both screens on demand for the visual gate)
**Category**: Phase Structure
**Move**: settled
**Change Type**: update-task

**Problem**:
The phase's last acceptance criterion promises the two screens can be rendered "at a chosen theme and width", and nothing in the phase builds a width control: `capturetool` takes `--fixture` and `--theme` and nothing else, and task 3-6 deliberately reaches the degraded form by naming a surface pinned below the card's size rather than by choosing a width — its own edge case says so, because a human dragging a window cannot reproduce a fallback point. So the gate either reads as unmet against work that is complete, or it invites a `--width` flag the task never asked for and whose interaction with `renderSizeFilter` nobody designed.

**Proposal**:
Restate the criterion in the terms the phase actually delivers — six named surfaces covering framed, reported and degraded for both screens, each at a size the surface pins, with the theme chosen by `--theme`. The wording is the planner's own rather than the specification's (the specification states nothing about how a screen is rendered for the gate), so nothing traceable moves.

**Current**:
```markdown
- [ ] Both screens can be rendered on demand at a chosen theme and width for visual check against the committed reference frames `resume-panel-waiting-nord.png` and `resume-panel-discard-confirm-nord.png`, and against Portal's existing modal grammar, which both screens are built from.
```

**Proposed Text**:
```markdown
- [ ] Both screens are reachable by name at a chosen theme — framed, carrying a report, and degraded — each at a size the surface pins rather than one reached by resizing a terminal, for visual check against the committed reference frames `resume-panel-waiting-nord.png` and `resume-panel-discard-confirm-nord.png`, and against Portal's existing modal grammar, which both screens are built from.
```

**Resolution**: Fixed — Phase 3's final acceptance criterion is restated as the screens being reachable by name at a chosen theme, framed / reported / degraded, each at a size its surface pins.
**Notes**: Verified before applying: `cmd/capturetool/main.go:36-37` declares `--fixture` and `--theme` and no other flag, so there is no width control for the old wording to have meant. planning.md only — no task body changed and no tick re-sync needed.
