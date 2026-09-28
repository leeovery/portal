TASK: 5-4 (tick-02e519) — The Confirmation Refuses Input That Was Already in Flight

ACCEPTANCE CRITERIA:
- `d` on the waiting panel composes a `resume-draw` argv carrying `--screen discard --drop-input`; no other hand-off in the chain composes an argv carrying `--drop-input`.
- A draw carrying `--drop-input` calls the drop seam exactly once, after the appearance query has resolved and before the first byte is written to stdout.
- A draw not carrying it never calls the drop seam (waiting-panel draw, resize redraw, report redraw), and `ResolvePaneTheme` is handed a nil drop on those paths.
- The `resume-wait` argv the draw execs never carries `--drop-input`.
- A drop seam returning an error paints the waiting panel carrying that error's text on its report row, execs a waiter on the panel screen, and paints no confirmation.
- A drop seam returning nil paints the confirmation exactly as task 5.2 paints it, with no report.
- The drop discards the queue rather than reading it; the only stdin read near it is the appearance probe, which runs before the drop.
- `flushTTYInput` errors for a non-tty file descriptor.
- On a real pty: bytes written to the master before the flush are not readable from the slave after it, and bytes written after the flush are.
- `flushTTYInput` has exactly one call site in the tree outside its declarations and its own tests.
- Task 4.2's inherited-bytes guarantee is unchanged on the waiting panel.

STATUS: complete

SPEC CONTEXT: §4.3 states that a burst of input cannot carry the discard through both screens. Input already in flight when the confirmation opens is dropped, not read as agreement, so only a keystroke arriving after the confirmation is on screen can confirm it. §5.2's corrigendum of 2026-09-21 fixes the order: the drop runs after the appearance query and never before it. The reason is that the query's own late reply lands in the pane's input queue, opens with ESC, and can carry a hex `d`. The keys themselves are unchanged (§6.2).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/tty_flush_darwin.go:7-13 — `freadOnly = 0x1` with a one-line comment, and `IoctlSetPointerInt(fd, TIOCFLUSH, freadOnly)`. Confirmed: x/sys v0.45.0 exports `TIOCFLUSH` but not `FREAD` on darwin, so the comment holds.
  - cmd/tty_flush_linux.go:8-10 — `IoctlSetInt(fd, TCFLSH, TCIFLUSH)`.
  - cmd/state_resume_chain.go:26 — the `drop-input` flag constant. cmd/state_resume_chain.go:56-59 — the `DropInput` field. cmd/state_resume_chain.go:86-88 — the flag is emitted only when `DropInput` is set.
  - cmd/state_resume_draw.go:45-49 — the seam is handed to `ResolveTheme` only when `DropInput` is set, otherwise nil. cmd/state_resume_draw.go:51-58 — `DropInput` is cleared on the hand-off payload. On a drop error the screen switches to the panel and the error text goes on the report. cmd/state_resume_draw.go:86-89 — `paneDrawTheme` threads the drop into `tui.ResolvePaneTheme`. cmd/state_resume_draw.go:119-124 — `dropStdinInputQueue` is the single `flushTTYInput` call site. cmd/state_resume_draw.go:183 — the flag is registered on the draw only.
  - cmd/state_resume_wait.go:330-333 — `resumeOpenDiscardConfirm` sets `DropInput`. cmd/state_resume_wait.go:379 — `resumeReport` clears it. The waiter does not register the flag (cmd/state_resume_wait.go:492-501), so a stray `--drop-input` fails its parse.
  - internal/tui/pane_appearance.go:69-75 — `resolvePaneTheme` runs the drop after `selectPaneTheme` returns. That code is task 3.10's; this task consumes it.
- Notes:
  - No hand-off other than `resumeOpenDiscardConfirm` sets the flag. The helper's first draw (cmd/state_hydrate.go:263-268), Enter, the discard, the cancel, the settle redraw and the report redraw all compose the payload with it unset.
  - No other flushing ioctl exists in the tree (`TCSETSF`, `TIOCSETAF`, `TCIOFLUSH` all absent), so the Enter path's inherited-bytes behaviour is untouched.
  - On an adaptive-pair nomination, the probe writes the OSC 11 query to stdout before the drop. "Before the first byte is written to stdout" therefore reads, as the task's own Do bullet frames it, as "before any paint byte". The query-first ordering the criterion also requires makes this the only consistent reading. Not a defect.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/state_resume_drop_input_test.go covers all ten planned tests:
    - Opening from `d` carries `--screen discard --drop-input`.
    - A table asserts no other waiter hand-off carries the flag: y, Escape, a refused-clear Enter, and a settle redraw of the confirmation.
    - An AST guard allows only `resumeOpenDiscardConfirm` to set `DropInput` true.
    - A shared call-order recorder shows appearance-query, then drop, then writes, then exec.
    - A table covers panel, resize and report draws, each handed a nil drop.
    - The flag is cleared on the exec'd `resume-wait` argv, on both success and failure.
    - On a failed drop, the panel's exact bytes carry the report and the exec is a panel waiter. The confirmation is absent.
    - On a successful drop, the confirmation's bytes are unchanged.
    - Cobra parsing: the draw accepts the flag and wires the seam; the waiter refuses it.
    - A repo-wide source guard pins exactly one `flushTTYInput` reference, with the `TIOCFLUSH`/`TCFLSH` ioctls confined to `cmd/tty_flush_*`.
  - `TestPaneDrawTheme_HandsTheDropThrough` covers the production resolver passing the drop's error through.
  - cmd/tty_flush_test.go asserts an error on a pipe descriptor.
  - cmd/tty_flush_pty_test.go (darwin) opens a pty through `/dev/ptmx` with the grant/unlock/name ioctls. It writes `cd ~/dev && yarn`, flushes the slave, and asserts nothing is readable within a 200ms deadline. It then asserts that a later `y` is readable.
  - The real ordering inside `ResolvePaneTheme` is pinned by task 3.10's tests (internal/tui/pane_appearance_test.go:275).
- Notes: "it consumes no byte on the drop path" asserts the same ordering as the call-order test. The no-byte property itself holds by structure: the seam's signature is `func() error` and the source guard pins the flush to the ioctl. This matches the plan's description of that test.

CODE QUALITY:
- Project conventions: Followed. Seams on the config struct, the cobra RunE wiring through `resumeDrawRunFunc`, and tests staged via `withFuncSeam`. Platform files match the release targets (darwin and linux only, per .goreleaser.yaml).
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "On a real pty: bytes written to the master before the flush are not readable from the slave after it, and bytes written after the flush are." — requires running `TestFlushTTYInput_RealPTY` in cmd/tty_flush_pty_test.go on darwin. Two runtime facts need observing: that the pty slave accepts a read deadline, and that the master write has reached the slave's input queue before the flush. Reading alone cannot confirm either.
