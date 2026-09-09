TASK: resume-hooks-silently-lost-7-2 — "The _portal-saver Pane Probe Uses The Helper The Package's Own Doc Calls Wrong, And Resolves A Prefix Sibling" (phase 7, implementation-analysis cycle; authority is the task body, not the specification)

ACCEPTANCE CRITERIA:
- Neither `saver_pane_pid.go` call site passes `exactTarget`.
- With `_portal-saver-old` live and `_portal-saver` absent, `SaverPanePIDOrAbsent` reports absence and never the sibling's pid.
- A live `_portal-saver` still resolves to its own pane pid.
- A genuinely missing session (no sibling present) still collapses to `(0, false, nil)`.
- A non-absence failure still returns `(0, false, err)`.
- The doc comment on `SaverPanePIDOrAbsent` describes the sentinel the route emits, with no unreachable claim left.

STATUS: complete

SPEC CONTEXT: This is a phase-7 analysis task, not specified work — the specification says nothing about the `_portal-saver` pane read, and its `list-panes` material concerns the hook-key enumeration (spec lines 150, 243). The task's own body carries the measurement it rests on, and the work unit's recorded analysis corroborates it: `.workflows/resume-hooks-silently-lost/implementation/resume-hooks-silently-lost/analysis-report-c2.md:14` records that on tmux 3.7c `list-panes -t =_portal-saver` returns `_portal-saver-old`'s pane pid at exit 0, and `analysis-tasks-c4.md:40` records `show-environment` answering a missed session with `no such session` — the two facts the fix and its comment are built on. The consumers at risk are the ones CLAUDE.md names for `SaverPanePIDOrAbsent`: the bootstrap orphan sweep, the daemon's Component D self-supervision probe, and the `_portal-saver` end-state assertions.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmux/saver_pane_pid.go:13` (`saverPanePID`) and `:37` (`SaverPaneID`) — both now compose `string(CoordTargetExact(sessionName))`, i.e. `=<name>:`. The helper was renamed from the task's `exactCoordTarget` to the exported `CoordTargetExact` by a later phase-9 task (`internal/tmux/tmux.go:467`); the substance is the one the task prescribed. Neither site passes `SessionTargetExact` / the old `exactTarget` — verified by reading both call sites and by a repo-wide search for a non-colon `=_portal-saver` target, whose only hit is a test's own direct `has-session` probe in `cmd/state_daemon_self_supervision_integration_test.go:378` (outside this change-set, and on an isolated socket that stages no sibling).
  - `internal/tmux/errors.go:29-37,52-56` — the absence signal was settled by the first of the task's two sanctioned options: `noSuchSessionStderrSubstrs` widened to `{"no such session", "can't find session"}` behind `reportsNoSuchSession`, keeping `ErrNoSuchSession` the single discriminator. `"can't find window"` is deliberately excluded and the comment says why (a missing window inside a live session is not a session absence) — an exclusion later confirmed by measurement in the task's own fix-tracking (`can't find window` is also what tmux emits for a missing *pane* index).
  - `internal/tmux/saver_pane_pid.go:48-52` — the doc now names both sentinels (`ErrNoSuchSession`, `ErrEmptyPaneList`), and with the widening the `ErrNoSuchSession` arm is genuinely reachable on this route rather than dead. No unreachable claim remains.
- Notes: The widening is a global behaviour change to `wrapNoSuchSession`, so I checked every classifier that consumes the sentinel. `internal/state/capture.go:71` classifies `ShowEnvironment`'s error, and `show-environment` emits `no such session` (measured in the work unit's own record), so capture's churn/anomaly split is untouched. `cmd/state_daemon.go:320-329` already matched `"can't find "` on its own path. `cmd/uninstall.go:95` becomes *more* correct — a `kill-session` that reports the saver already gone now reports a clean removal instead of an error, and `cmd/uninstall_test.go:342-355` pins exactly that. `cmd/state_daemon.go:81-90`'s membership probe reads through `HasSession` (also `CoordTargetExact`) and then `SaverPanePIDOrAbsent`, so both halves of the probe now pin the session exactly — which is the defect's actual blast radius closed.

TESTS:
- Status: Adequate
- Coverage: All six subtests the task named exist, and each maps to a criterion.
  - `internal/tmux/saver_pane_pid_realtmux_test.go:25` "it does not resolve a prefix-sibling saver session" and `:39` "it reports absence when only a prefix sibling is live" — real tmux on an isolated `-L` socket with `_portal-saver-old` live and `_portal-saver` absent, asserting both the raw read errors and the tri-state collapses, with the sibling's pid read straight off the wire (`livePanePID`, `internal/tmux/hookkey_realtmux_shared_test.go:106`) so the comparison is against truth rather than a second Portal read. A regression to the unpinned form returns the sibling pid at exit 0 and both subtests fail — they observe the defect, not just the fix.
  - `:54` "it returns the pane pid of a live _portal-saver" (sibling *and* saver live) and `:68` "it collapses a missing session to present=false" cover criteria 3 and 4. The latter is what empirically pins the widened stderr match against real tmux: if `list-panes` worded the absence any third way, it would return a non-nil error and fail.
  - `internal/tmux/saver_pane_pid_test.go:127` (empty pane list) and `:152` (non-absence error passthrough) cover criteria 4b and 5; `:28` pins the argv as `=_portal-saver:`, and `internal/tmux/portal_saver_lifecycle_events_test.go:37` pins `SaverPaneID`'s.
  - `internal/tmux/saver_pane_pid_test.go:136` "it does not read a missing window inside a live session as session absence" drives a real `*tmux.CommandError` through the predicate, so the deliberate `can't find window` exclusion the widening rests on fails loudly if dropped — the gap the first fix round identified, closed.
- Notes: Lane placement is correct — the real-tmux file carries no build tag and is a `*_realtmux_test.go` client test on a per-test socket with no daemon and no built binary, which is the carve-out CLAUDE.md grants the unit lane. The readiness hazard raised in fix round 2 (a fuzzy `has-session` wait being inert precisely because this fixture stages a live prefix sibling) is closed at the shared helper rather than locally: `internal/tmuxtest/socket.go:124` now polls `CoordTargetExact(name)`, and `internal/tmuxtest/socket_realtmux_test.go:26-37` asserts the wait does *not* return for a live prefix sibling. There is deliberate overlap with the later parameterised route suite (`internal/tmux/exact_session_target_realtmux_test.go:166,287` cover the same gone/live pair for `SaverPanePIDOrAbsent`), but this file keeps the one case that suite has no fixture for — a genuinely absent session with no sibling at all — and the plan named these subtests explicitly.

CODE QUALITY:
- Project conventions: Followed. Targets go through the `Target`-typed vocabulary and are spent as strings only at the argv boundary; real-tmux coverage stays in the sanctioned unit-lane file shape; no logging, no new component or attr.
- SOLID principles: Good. The classification stays in `errors.go` behind one predicate, so the two call sites express intent (`CoordTargetExact`) and nothing else.
- Complexity: Low.
- Modern idioms: Yes — `slices.ContainsFunc` over the substring set; `golangci-lint`'s `modernize` set is clean on it either way.
- Readability: Good. The comment at `internal/tmux/errors.go:29-35` states the measured conclusion (which command emits which wording, and why `can't find window` is out) rather than the reasoning trail, and its attributions match the measurements recorded in the work unit's analysis files.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
