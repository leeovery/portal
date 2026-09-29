TASK: Pane Appearance Probe Reads the Terminal's Reply Through a Bounded Read macOS Honours (lazy-resume-on-attach-9-4, tick-f260b4)

ACCEPTANCE CRITERIA:
- From a process whose controlling terminal is a pty, the production probe under an adaptive pair writes the background-colour query once. Answered with a light background it resolves the light member; answered with a dark background, the dark member.
- A reply that arrives within the timeout is consumed by the probe: nothing of it is echoed back to the terminal.
- A terminal that never answers resolves dark no sooner than the probe's timeout, and the bound the read was armed with, recorded when it was armed, is no more than that timeout: a probe arming a bound ten times the timeout fails.
- A path that cannot arm the bounded read writes nothing to the terminal and resolves dark without blocking.
- The existing outcomes hold. A constant or zero nomination and a `NO_COLOR` draw write and read nothing. A stdin that is not a terminal or refuses raw mode resolves dark with nothing written. A read failure, a truncated or unparseable reply, and a reply after the deadline all resolve dark. Raw mode is restored on every path that entered it, and the input drop runs once, after the question has resolved.
- `internal/tui` opens no file: no `os.OpenFile` call remains, `TestConstruction_ReadsNoThemesDirectory` holds with no exemption, and the package makes no `os.Stat` or `os.Getenv` call.
- `newPaneAppearanceProbe` has exactly one caller, and the probe's timeout is `appearanceDetectTimeout` with no duration of its own (`TestPaneAppearance_ReachesTheProbeFromOnePlace`, `TestPaneAppearance_TakesThePickerTimeout`).
- The real-terminal test runs in the unit lane and builds no portal binary.

STATUS: issues_found

SPEC CONTEXT: The spec's section on the panel's theme says a light/dark pair runs the same detect-or-timeout gate as the picker, in the drawing process, racing `appearanceDetectTimeout` and resolving dark when nothing answers. A redraw with a client present resolves against the terminal in front of it. The 2026-09-21 corrigendum fixes the order: probe first, input drop after, because the query's own reply is input that a drop taken first cannot clear. `NO_COLOR` runs no detection. The task fixes the macOS defect where a fresh `/dev/tty` open refused a read deadline. On that path the probe wrote the query, then resolved dark on every draw, and the terminal's reply was echoed across the card.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/pane_appearance.go:38-46: the production constructor binds `armRead: armStdinRead` and `timeout: appearanceDetectTimeout`. It has one non-test caller (`ResolvePaneTheme`, :59).
  - internal/tui/pane_appearance.go:80-104: `detect()` runs isTerminal, then makeRaw with a deferred restore, then `armRead(p.timeout)`, then writes the query. An arm failure (:92-95) returns dark before any byte is written.
  - internal/tui/pane_appearance.go:146-152: `armStdinRead` rejects an fd outside the select set and records the deadline as `time.Now().Add(bound)` at arm time.
  - internal/tui/pane_appearance.go:154-193: `selectBoundedReader` runs select on stdin's fd with the remaining time, recomputed on every loop (so EINTR is safe), then does a plain `unix.Read`. A timeout returns `os.ErrDeadlineExceeded` and a 0-byte read returns `io.EOF`.
  - The `paneTTYPath`/`openTTYPath`/`paneReader` machinery is gone. No `os.OpenFile`, `os.Stat` or `os.Getenv` call remains in the package's non-test files. `x/sys/unix` v0.45.0 declares `Select (n int, err error)`, `FdSet.Set`, `FD_SETSIZE` and `NsecToTimeval` on both darwin and linux, so the one path compiles on both release platforms.
- Notes: The implementation matches the chosen form in the task's Solution: a select-bounded read on stdin, nothing opened, and the query written only once the read is armed. The probe-then-drop order is unchanged (`resolvePaneTheme`, :62-68). Raw mode is now entered before the read is armed, so the arm-failure path runs the restore. The restore-on-every-path test covers this. The one inaccurate item is the justifying comment on `armStdinRead` (see FINDINGS).

TESTS:
- Status: Adequate
- Coverage:
  - Seam suite (internal/tui/pane_appearance_test.go):
    - The fake `armRead` records the bound at arm time, before anything is read (:115-127).
    - `assertArmedWithin` (:172-181) replaces the check that could never fail. It is applied in the never-answers case, which also asserts elapsed >= timeout (:429-443).
    - It is shown to fail on a probe armed at ten times the timeout, through `harnesstest.Recorder` (:445-457).
    - The unarmable path writes nothing, reads nothing and returns within 4x the timeout, off the test goroutine (:512-534). The event order is [arm read, write query, drop] for answered and silent probes, and [arm read, drop] when the read cannot be armed (:281-305).
    - Constant, zero and `NO_COLOR` draws arm and read nothing (:196-251). Non-terminal stdin and refused raw mode are covered (:488-510), as are read failure, truncated or unparseable replies and a late reply (:459-486, :536-551). Restore runs once on seven paths, including arm failure (:553-578), and the drop runs once per nomination (:307-337).
  - Real-terminal test (internal/tui/pane_appearance_realtty_test.go:40-67, 91-125, `//go:build darwin`, no integration tag):
    - It re-execs the test binary itself as a `Setsid`/`Setctty` child whose stdin, stdout and stderr are a fresh pty slave, the same pattern as `foregroundOn`. The child returns its verdict on ExtraFiles fd 3, so nothing but the probe writes to the pty.
    - It covers a light answer, a dark answer and no answer. It asserts both the resolved member and that the terminal saw exactly one query and no echo.
    - Against the old `/dev/tty` reader it would have failed: an immediate dark result and an echoed reply.
    - It references no `portalbintest` build helper.
  - Guards:
    - `TestPaneAppearance_OpensNothing` (internal/tui/pane_appearance_guard_test.go:111-132) forbids `os`/`syscall`/`unix` `Open*`/`Create` in non-test files.
    - `TestConstruction_ReadsNoThemesDirectory` (internal/tui/nomination_test.go:173-197) has lost its `opensThePanesTerminal` exemption (:189) and still bans `os.Stat`/`os.Getenv`.
    - The one-caller and timeout guards (:58-74, :76-107) are unchanged.
- Notes: The production reader's own timing is exercised only by the real-terminal no-answer case, whose ceiling is the 10s child budget. The bound assertion sits at the fake seam, as the task prescribes. That real-terminal case drives the production 50ms window across a cross-process pty round trip, so whether it holds steady under a loaded unit lane is a run question (see UNSETTLED). No over-testing: each subtest pins a distinct path.

CODE QUALITY:
- Project conventions: Followed. Small function seams, no `t.Parallel`, source guards over sourceguardtest, no process-artifact references in comments, and the guard amendments made in the same change as required.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: The `armStdinRead` comment overstates the platform fact it rests on (see FINDINGS).

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] internal/tui/pane_appearance.go:143-145 — the comment says "macOS refuses to poll a terminal through kqueue, so neither takes a deadline there". That is true of `/dev/tty` only. The task itself measured that a pty's own device node accepts a read deadline, which in Go means the kqueue registration succeeded. The `muesli/cancelreader` module in `go.mod` also falls back to select only when the file is named `/dev/tty`, and uses kqueue for every other descriptor, stdin included (cancelreader_bsd.go:28-30). Replace lines 143-145 with: `// armStdinRead bounds reads of stdin with select rather than a read deadline: os.Stdin is a blocking descriptor outside the runtime poller, so it takes no deadline, and a fresh /dev/tty open takes none on macOS either, since kqueue cannot poll that device.` (wrapped across comment lines). — FAILS: the only in-code reason for the design states a false platform fact. A maintainer weighing the pane's-own-device alternative, which the task measured as working, would rule it out on that comment's word.

UNSETTLED:
- "From a process whose controlling terminal is a pty, the production probe under an adaptive pair writes the background-colour query once. Answered with a light background it resolves the light member; answered with a dark background, the dark member." — Reading shows the code and the test are correct. Whether macOS's select on a pty stdin reports the reply readable within the production 50ms window is a platform behaviour, and that is exactly what the previous form got wrong. To settle it, run `go test ./internal/tui -run TestProductionPaneProbe_RealTerminal -count=20` on darwin, once alone and once alongside a full `go test ./...`. All three subtests must pass on every run.
- "A reply that arrives within the timeout is consumed by the probe: nothing of it is echoed back to the terminal." — Settled by the same real-terminal run: the transcript equals exactly one query in the light and dark cases.
