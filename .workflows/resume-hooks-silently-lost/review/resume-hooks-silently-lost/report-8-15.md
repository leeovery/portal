TASK: resume-hooks-silently-lost-8-15 — "Fifteen Fixed-Budget PollUntil Sites Are The Flake Class Already Fixed For Seven"
(tick-8d54f3, phase 8, done). Convert fifteen fixed-wall-clock daemon-lifecycle waits to the progress-based
`AwaitProgress`, move `AwaitProgress`/`PollUntil` out of `internal/tmuxtest` into the neutral leaf task 24 created,
and loosen the unit-lane elapsed assertion to a `2*Ceiling` tolerance.

ACCEPTANCE CRITERIA:
- [x] None of the fifteen sites polls a monotonic observable against a fixed wall-clock budget.
- [x] `AwaitProgress` and `PollUntil` are declared once, in the neutral leaf; `internal/tmuxtest` declares neither.
- [x] Each converted failure message quotes the last observation.
- [x] The elapsed assertion tolerates `2*Ceiling` and still fails a mis-derived ceiling.
- [~] Both lanes pass, the integration lane serially (`-p 1`). — not executable by this reviewer (no test
      execution); judged by reading, with no compile or logic breakage found.

STATUS: complete

SPEC CONTEXT: The specification (`Resume Hooks Silently Lost`) governs the positional-hook-key defect and its four
repairs (A durable pane token, B loud registration, C shape-aware sweep, D locked hooks.json). This is a phase-8
implementation-analysis task: per the shared verifier context, its authority is its own body, not the spec. The spec
is relevant only as the reason these daemon-lifecycle integration suites exist — they are the suites the sweep,
lock and restore work is pinned by, so a wall-clock flake in them manufactures false verdicts about the repair.

IMPLEMENTATION:
- Status: Implemented
- Location: commit `45e00708`.
  - Move (verbatim, only the `package` line changed): `internal/harnesstest/progress.go:1-97`,
    `internal/harnesstest/poll.go:1-19`; `internal/tmuxtest` now holds `baseindex.go`, `skip.go`, `socket.go`,
    `stamp.go` and one real-tmux test only.
  - New shared observation type: `internal/portaltest/daemon_pid_observation.go:15-37`
    (`DaemonPIDObservation` + `ObserveDaemonPID`), consumed by six of the converted waits.
  - The fifteen converted sites:
    `cmd/bootstrap/orphan_sweep_integration_test.go:228`;
    `cmd/bootstrap/upgrade_path_integration_test.go:190` and `:202`;
    `cmd/state_daemon_integration_test.go:283`;
    `cmd/state_daemon_hook_cleanup_integration_test.go:142` and `:200`;
    `internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go:105` and `:156`;
    `internal/tmux/portal_saver_endstate_integration_test.go:67` and `:117`;
    `internal/tmux/portal_saver_integration_test.go:226`, `:244`, `:266`, `:338`, `:349`.
    I enumerated the pre-change set at `45e00708^` and counted exactly fifteen `tmuxtest.PollUntil` calls across
    those files; all fifteen are gone and all fifteen replacements are present. (The task body's prose says "eight
    across" the three `internal/tmux` files; the true count there is nine — 5 + 2 + 2 — which is why the totals
    reconcile to fifteen. The criterion is met either way.)
  - `internal/portaltest/tmux_server_wait.go:30` and `internal/state/capture_colon_session_realtmux_test.go:79`
    re-pointed to `harnesstest.PollUntil` (the only two `PollUntil` consumers outside the moved package at the time).
  - CLAUDE.md rows for `harnesstest` and `portaltest` updated to record the new homes and the new export; the
    `tmuxtest` row already read "real-tmux socket fixtures" and needed no change.
- Notes:
  - No dead constants left behind: `saverPanePIDTimeout`, `upgradePathPIDFileTimeout`, `daemonAliveTimeout`,
    `daemonReadyBudget`, `hookCleanupObservationBudget`, `endStateReadyTimeout`, `scrollbackEmergenceTimeout` and
    `singletonRecycleTimeout` are all gone from the tree; every surviving tick constant is referenced by a
    `ProgressWait.Tick`.
  - No stale references remain: nothing in the tree names `tmuxtest.AwaitProgress`, `tmuxtest.PollUntil`,
    `tmuxtest.ProgressWait` or `tmuxtest.ProgressResult`.
  - Semantics preserved at each converted site. The two that changed their read are equivalent by construction:
    `waitForDaemonNotAlive` moved from `state.DaemonAlive(dir)` to `!ObserveDaemonPID(dir).Alive`, and
    `waitForDaemonAlive` likewise — `DaemonAlive` (`internal/state/daemon_state.go:65-71`) is exactly
    `ReadPIDFile` + `IsProcessAlive`, which is what `ObserveDaemonPID` composes.
  - Genuine de-duplication landed alongside the conversion rather than a second copy:
    `cmd/bootstrap/orphan_sweep_integration_test.go:278` (`readSaverPanePID`) now reuses `observeSaverPanePID`
    instead of restating the list-panes read.
  - Every new observation type is `comparable` (required by `AwaitProgress[T comparable]`) and carries a
    `String()`, so `ProgressResult.String()`'s `last=%v` renders the reading rather than a struct dump.

TESTS:
- Status: Adequate
- Coverage: `internal/harnesstest/progress_test.go` and `poll_test.go` moved with their subjects and were
  restructured from five top-level `TestAwaitProgress_*` functions in `package tmuxtest` into one
  `TestAwaitProgress` with "it …" subtests in the external `package harnesstest_test` — a tightening, since the
  suite now drives only the exported surface. All three tests the plan named are present and are what they say:
  "it reaches the target once the observation converges" (`:12`), "it reports the last observation when the wait
  stalls" (`:93`), "it tolerates scheduler noise up to twice the ceiling" (`:35`). The two carried-over subtests
  ("it gives up inside the ceiling when the observation stops changing" `:69`, "it applies defaults to a
  zero-value wait" `:116`) each pin a distinct behaviour — Stall, and `withDefaults` — so the file is not
  over-tested. The fifteen converted suites keep their existing assertions; only the wait changed, which is what
  the plan directed.
- Notes:
  - Criterion 4 verified against the old line: `45e00708^:internal/tmuxtest/progress_test.go:55` was
    `got.Elapsed > wait.Ceiling+300*time.Millisecond`; it is now `got.Elapsed > 2*wait.Ceiling`
    (`internal/harnesstest/progress_test.go:63`), with the reasoning stated in-source at `:58-62`. It still fails a
    mis-derived ceiling: with `Ceiling: 900ms` a ceiling taken from the wrong budget is off by orders of magnitude
    in either direction, and the low side is separately caught by the `Elapsed >= 3*Stall` assertion above it.
  - `internal/harnesstest/leaf_guard_test.go:16-20` pins the package to an empty dependency allowlist across both
    lanes, so the two moved files cannot quietly drag a dependency into a package every test package reaches for.
    `progress.go` and `poll.go` import `fmt` and `time` only, so the guard holds.
  - Not executed (this review reads tests, it does not run them). Every changed file's import block is complete and
    fully used, and no identifier collides inside the shared `package tmux_test` / `package bootstrap_test` /
    `package cmd_test` namespaces.

CODE QUALITY:
- Project conventions: Followed. Lane rule intact — every converted suite is already `//go:build integration` and
  stays there; `internal/harnesstest` remains untagged and stdlib-only, so its own suite runs in the fast lane.
  The test-isolation invariants are untouched: no wait enumerates or signals a process the test did not spawn, and
  `state.RegisterSandboxDaemon` still runs on every PID the saver-pane read yields
  (`cmd/bootstrap/orphan_sweep_integration_test.go:235`, `:279`). No `t.Parallel()` introduced.
- SOLID principles: Good. `ObserveDaemonPID` is one reading with one reason to change, and hoisting it into
  `portaltest` removes what would otherwise have been six copies across three packages.
- Complexity: Low. Each conversion is observe-closure + reached-predicate + a quoted failure; the branching that
  used to live inside the poll body (`if err != nil { return false }`) became a field on a comparable struct, which
  is what lets the wait distinguish "still coming" from "stopped moving".
- Modern idioms: Yes. Generic `ProgressResult[T comparable]`, value-receiver `String()` for `%v`, external test
  package.
- Readability: Good. Every `ProgressWait` var carries a comment justifying its Stall against the real convergence
  it bounds — the sharpest being `cmd/state_daemon_hook_cleanup_integration_test.go:52-56`, which explains why
  Stall must outlast the ~10s reap throttle (25s) or the wait would give up on a daemon merely waiting its turn.
- Issues: None that clear the bar. Checked and dismissed as preference, not defect:
  (a) `internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go:156` observes a bare `bool`, so
      only Stall (3s, unchanged from the old fixed budget) can fire — but the orphan is already reaped by the
      `select` at `:149-155`, the observable is genuinely binary, and there is no intermediate state to progress on.
  (b) `endStateReadyWait`, `singletonRecycleWait` and `upgradePathPIDFileWait` each serve two waits while their
      comments describe one; incomplete rather than false, and the budgets fit both consumers.
  (c) `internal/tmux/portal_saver_integration_test.go:54`/`:67` pass `singletonRecycleWait.Ceiling` to
      `tmuxtest.WaitForSession`, widening that (pre-existing, out-of-scope) fixed poll from 5s to 45s — a slower
      red run, never a wrong verdict.
  (d) Do-item 4's "drop the part of it the two assertions above it already make" left no assertion removed. The
      governing criterion ("tolerates 2*Ceiling and still fails a mis-derived ceiling") is met, the surviving
      `Elapsed >= 3*Stall` cannot false-fail (with `!Reached && !Stalled` the wait exits at the ceiling, nine
      stall budgets in), and nothing breaks either way.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
