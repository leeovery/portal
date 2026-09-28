TASK: lazy-resume-on-attach-4-2 (tick-bafa83) — The Waiting Process: Two Keys, Everything Else Swallowed (`portal state resume-wait`)

ACCEPTANCE CRITERIA:
1. `\r` and `\n` each produce exactly one hand-off; `d` produces exactly one hand-off; each is preceded by one `exec` INFO carrying `target` and `args`.
2. `0x03`, `0x04`, `0x1a`, `0x1b`, `D`, `y`, `q` and a run of ordinary printable text each produce no hand-off, no write to stdout, and leave the loop running.
3. A three-byte escape sequence (`\x1b[A`) produces no hand-off; no sequence assembles into an action.
4. A burst delivering `dy` in one write produces exactly one hand-off (the `d`) and leaves the `y` unread.
5. The terminal restore runs on every path out: an answered key, a read error, an EOF, and a `MakeRaw` that succeeded followed by any later failure.
6. A stdin that is not a terminal, and a `MakeRaw` that fails, each return an error without reading, writing or exec'ing.
7. The restore runs before the hand-off exec.
8. No handler for SIGHUP, SIGTERM or SIGINT, asserted by a source guard over the wait path's `signal.Notify` calls that admits only `syscall.SIGWINCH`.
9. The wait path arms no timer: a waiter holding a `--report` keeps reading, keeps the report and keeps dispatching with no input.
10. The wait path constructs no theme, calls no renderer and writes nothing to stdout while waiting.
11. The dispatch is size-independent, including sizes below the card's and non-positive ones.

STATUS: issues_found

SPEC CONTEXT: The waiter is the pane's resting process after the draw hands off. It must sit at the runtime floor (no theme, no rendering), and it swallows every byte except the acting keys, so a paste, a send-keys or Ctrl-C/Ctrl-\/Ctrl-Z/Ctrl-D can never answer the panel. It must not decline the hangup: tearing the pane down has to end the waiter, or culled sessions leave resident Portal processes behind. Escape is inert on the waiting panel. Both acting keys hand the pane to a fresh process image. The spec's §4.2, §4.3, §5.2 and §6 govern this task.

IMPLEMENTATION:
- Status: Implemented. Later tasks legitimately re-pointed the destinations and extended the loop.
- Location:
  - cmd/state_resume_wait.go:106-122 is `runResumeWait`. It checks for a terminal, enters raw mode, and wraps the restore in `sync.OnceFunc` behind a `defer`.
  - cmd/state_resume_wait.go:135-147 is `resumeKeysFor`, the per-screen key map. On the panel it holds CR, LF and `d` only.
  - cmd/state_resume_wait.go:151-198 is the loop.
  - cmd/state_resume_wait.go:301-311 is the one-byte, one-request-at-a-time reader.
  - cmd/state_resume_wait.go:397-400 is `resumeRedraw`, which restores and then calls `resumeHandOff`. `resumeHandOff` (cmd/state_resume_chain.go:117-127) resolves the exe, emits the `exec` INFO and execs.
  - cmd/state_resume_wait.go:425-429 is `winchSignals`, which watches SIGWINCH only.
  - cmd/state_resume_wait.go:444-504 holds the seam and the hidden `resume-wait` command on `stateCmd`.
- Notes:
  - Enter now resumes the hook or shell (task 4.3). Panel `d` now opens the confirmation (Phase 5). Both still end in exactly one exec. Each is preceded by one `exec` INFO carrying `target` and `args`: `resumeHandOff` for redraws, `execHandOff` (cmd/state_resume_chain.go:132-135) for hook/shell hand-offs.
  - Escape handling has moved on from "each byte discarded on its own". An ESC now opens a 50ms follow window, and a CSI, SS3 or OSC is consumed whole up to a cap (cmd/state_resume_wait.go:204-263). This divergence is sound and better than the plan's wording. With per-byte discarding, a sequence ending in an acting key (`\x1b[5d`, `\x1bOd`, an OSC 11 reply carrying a hex `d`) would have acted. Criterion 3's substance holds: no sequence produces an action.
  - The restore runs exactly once on every path out, and explicitly before every exec.
  - A failing `resumeChainExe()` is returned as the wait's ending condition, with the deferred restore running.
  - No SIGHUP/SIGTERM/SIGINT handler exists. `main.go` and `cmd/root.go` install none either.
  - Idle, no timer is armed: `settled` stays nil until SIGWINCH, and the ESC window is armed only after an ESC byte.
  - `state_resume_wait.go` imports no theme/tui/prefs package and calls no theme or render helper.
  - Dispatch never consults Width/Height.

TESTS:
- Status: Adequate. Both static guards have reach gaps (see FINDINGS).
- Coverage:
  - Enter CR/LF: cmd/state_resume_wait_test.go:190-207.
  - `d` hand-off with exec INFO target/args/level: cmd/state_resume_screens_test.go:278-286, through `assertHandOff`.
  - Every non-acting key on both screens: cmd/state_resume_screens_test.go:537-550. This covers \x03 \x04 \x1a \x1b D q a space and "cat README", plus y/Y on the panel. It asserts EOF reached, reads == len+1, no exec, no write and no state touched.
  - `\x1b[A`: cmd/state_resume_wait_test.go:210-219.
  - `dy` burst leaving `y` unread, with a reader that fails on any buffer larger than one byte: cmd/state_resume_wait_test.go:221-238.
  - Restore on answered/read-error/EOF: cmd/state_resume_wait_test.go:242-262.
  - Restore before exec: cmd/state_resume_wait_test.go:264-274 and cmd/state_resume_screens_test.go:588-618.
  - Non-tty and MakeRaw refusals, with zero reads, no raw call and no hand-off: cmd/state_resume_wait_test.go:276-312.
  - Read error: cmd/state_resume_wait_test.go:316-326.
  - No write and no theme while waiting: cmd/state_resume_wait_test.go:328-347.
  - Report held through an idle wait with no windows armed: cmd/state_resume_wait_test.go:349-430.
  - Signal guard: cmd/state_resume_wait_test.go:437-469.
  - Sizes 1x1, 20x6, 0x0 and -4x-2 on both keys: cmd/state_resume_screens_test.go:620-658.
  - Command argv parse and required `--command`: cmd/state_resume_wait_test.go:501-543.
- Notes: Not over-tested. The large escape/screen tables belong to later tasks' criteria.

CODE QUALITY:
- Project conventions: Followed. The seams are package-level, the tests stage them via `withFuncSeam`, there is no `t.Parallel`, and the logs use the hydrate component.
- SOLID principles: Good. The per-screen key map keeps the loop closed to new screens, and every OS touchpoint sits behind a config seam.
- Complexity: Acceptable. The escape resolution adds branching, but each piece is small and named.
- Modern idioms: Yes (`sync.OnceFunc`, `for range` over channels).
- Readability: Good. The comments state their reasons and hold true against the code.
- Issues: None in production code.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] cmd/state_resume_wait_test.go:448 — The signal guard checks `Notify`, `Ignore` and `Reset`. Every other callee falls through to `default`, so the guard never inspects `signal.NotifyContext`, the standard library's other handler-registering call. It takes the same argument shape (parent context first, then the signals), and naming no signal relays every signal. Fix: change `case "Notify":` to `case "Notify", "NotifyContext":`. The existing `len(call.Args) < 2` and `call.Args[1:]` checks then apply unchanged and correctly. — FAILS: someone could add `signal.NotifyContext(ctx, syscall.SIGTERM)` or `(ctx, syscall.SIGHUP)` to state_resume_wait.go, the idiomatic Go shape for graceful termination. The waiter would then decline termination or hangup and outlive its torn-down pane, and the guard written to forbid exactly this would stay green.
- [in-scope] [spreading] cmd/state_resume_wait_test.go:335 — The test "it writes nothing and resolves no theme while waiting" checks "resolves no theme" only by reading state_resume_wait.go's import list for internal/theme, internal/tui and internal/prefs. The draw's theme route lives in the same `cmd` package: `paneDrawTheme` (cmd/state_resume_draw.go:86) and `shippedPaneThemePair` (cmd/state_resume_draw.go:112). The wait file can call either without adding an import, so the check passes. The test's behavioural half only watches `cfg.Stdout`, and theme resolution never writes there. The guard has to see same-package references to the draw path, and the fix is not mechanical. The obvious rule would fail on any reference to a function declared in a cmd file that imports those packages. But that rule would flag `paneSizeFromStdin`, which the waiter legitimately wires (cmd/state_resume_wait.go:481) and which is declared in that same draw file (cmd/state_resume_draw.go:128). So either move that helper out of the draw file and adopt the rule, or name the draw path's theme and render entry points in a deny list. — FAILS: a waiter that resolves a theme on every launch passes the test named for forbidding it. The theme loader and prefs read would then stay resident for the whole wait, which is the resident cost the draw-then-hand-off split exists to remove.

UNSETTLED:
- None
