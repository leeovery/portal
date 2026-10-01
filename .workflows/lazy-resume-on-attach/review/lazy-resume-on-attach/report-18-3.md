TASK: One Declaration of Which Panes a Scrollback Dump Skips (lazy-resume-on-attach-18-3, tick-d17b3b)

ACCEPTANCE CRITERIA:
- A daemon tick over a skeleton-marked pane neither capture-panes it nor writes its scrollback file, while the tick's other panes are dumped as before — `TestDaemonTick_SkipsSkeletonMarkedPanesInScrollback` (`cmd/state_daemon_run_test.go`) passes unedited
- A daemon tick over a pane carrying `@portal-resume-pending` neither capture-panes it nor rewrites its scrollback file — `TestCaptureAndCommit_SkipsResumePendingPanes` (`cmd/state_daemon_resume_pending_test.go`) passes unedited
- A daemon tick in which a waiting pane's session missed the capture commits the carried session and capture-panes none of its panes — `TestDaemonTick_CarriesAFailingWaitingSessionWithoutCapturingIt` (`cmd/state_carry_missed_session_test.go`) passes unedited
- The lazy-panel fixture's capture round skips exactly the panes the daemon's tick skips, `Carried` included, and every suite driving it passes with no assertion changed
- `rg -n 'paneSkipsScrollback' --glob '*.go'` finds nothing, and neither dump over a `CaptureCycle` indexes `Skeleton`, `Pending` or `Carried` itself — both ask `CaptureCycle.SkipsScrollback`
- `go test ./...`, `go test -tags integration -p 1 ./...` and `golangci-lint run` pass; the only test-file edit is the fixture's skip

STATUS: issues_found

SPEC CONTEXT: The saver must leave a pane's scrollback alone while it is mid-restore (skeleton-marked) or carries `@portal-resume-pending` — capturing a waiting pane writes back its transcript minus the screenful the panel covers, with no other copy of those lines. Task 18-1 added a third case: a waiting pane's session that missed the capture is carried whole from the previous index, and none of its panes may be dumped because the carried address may now answer to another pane. This task is a structural refactor: one declaration of that three-set rule on the type that owns the sets, taken by every dump over a `CaptureCycle`, with production behaviour unchanged.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/state/scrollback.go:271-283` — `func (c CaptureCycle) SkipsScrollback(paneKey string) bool`, beside `CaptureCycle` (`:256-269`), true for a key in any of `Skeleton`, `Pending`, `Carried`; nil sets index safely. Its doc comment carries the waiting-panel reason and the carried-address reason.
  - `cmd/state_daemon.go:304` — `scrollbackDump.run` calls `capture.SkipsScrollback(paneKey)`; `paneSkipsScrollback` is gone from the file (read in full: lines 1-469).
  - `internal/restore/lazy_resume_panel_integration_test.go:497` — `captureRound`'s dump replaces its two inline `Skeleton`/`Pending` checks with one `capture.SkipsScrollback(key)`, so its doc comment's "the way the daemon takes one" (`:480`) is true again, `Carried` included.
  - `internal/state/commit_cycle.go:39-41` — the `Dump` field doc names `CaptureCycle.SkipsScrollback` as the skip a dump takes.
- Notes: The two dumps over a `CaptureCycle` (daemon, fixture) both ask the method and neither indexes a set itself; the fixture's `capture.Pending` at `:528` is a returned value for assertions, not a skip. The `Dump` closures in `internal/state/capture_carry_test.go` (`:96`, and the dump-less commit-now arm) and `internal/state/commit_cycle_test.go` (`:230`, `:274`, the dump-less `commitNowCycle`) capture nothing. `runDaemonTick` (`cmd/bootstrap/daemon_tick_test_helpers_test.go:27-90`) still takes `CaptureStructure`'s skip set directly with its switchable `withoutSkipGuard`, as the task required. No drift.

TESTS:
- Status: Adequate
- Coverage: Each arm of `SkipsScrollback` is observed through the daemon's real dump by a test that fails if that arm is dropped:
  - Skeleton only: `TestDaemonTick_KeepsARenumberedWaitingPaneTranscriptWhileSkeletonMarked` (`cmd/state_daemon_run_test.go:471-537`): no capture-pane for the skeleton-marked `work:1.0` (exact-target-aware comparison at `:530`) and no scrollback file for it (`:534`).
  - Pending only: `TestCaptureAndCommit_SkipsResumePendingPanes` (`cmd/state_daemon_resume_pending_test.go:120-141`): capture targets equal `=work:0.0` alone.
  - Carried only: `TestDaemonTick_CarriesAFailingWaitingSessionWithoutCapturingIt` (`cmd/state_carry_missed_session_test.go:83-121`): `foo:0.1` is unmarked and its session missed the capture, so only `Carried` holds it. The test asserts the capture targets equal `=other:0.0` alone.
  - The fixture shares the method, so it skips exactly what the daemon skips by construction.
  - No direct unit test of the method was added, which matches the task's rule that the fixture's skip is the only test-file edit.
- Notes: The test criterion 1 names as its evidence does not observe the skeleton skip (see FINDINGS). The skeleton arm is still covered by the renumbered-pane test above, so criterion 1 is met in substance.

CODE QUALITY:
- Project conventions: Followed — one declaration per rule, in the package that owns the sets; the doc comment states reasons, not process references.
- SOLID principles: Good — the skip rule now lives on the type that holds the three sets.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None in the delivered change. The comment's claim (a capture of a pane on the panel's screen writes back the transcript minus the covered screenful) matches how tmux saves only the visible lines on alternate-screen entry while history stays on the live grid.

BLOCKING ISSUES:
- None

FINDINGS:
- [out-of-scope] cmd/state_daemon_run_test.go:465 — `TestDaemonTick_SkipsSkeletonMarkedPanesInScrollback` compares the raw capture-pane target `call[6]` against `"work:0.1"`. The daemon composes the exact-match form through `tmux.PaneTargetExact` (`cmd/state_daemon.go:319`), and its siblings assert that form for this argv slot: `captureTargets` returns `=work:0.0` (`cmd/state_daemon_resume_pending_test.go:106`), and the fake keys its lookup on `sessionFromExactTarget(args[6])` (`cmd/state_daemon_run_test.go:134`). So the comparison never matches, and the test makes no other assertion. Fix: compare `sessionFromExactTarget(call[6]) == "work:0.1"` as `:530` does, and assert that no scrollback file exists for `skipKey`, as `:534` does for its pane. This is a test assertion that predates this task, and the task forbids editing the test, so it belongs to whoever owns the skeleton-skip suite. — FAILS: the test still passes if `SkipsScrollback` stops checking `Skeleton`, so the test that criterion 1 names as its evidence proves nothing about the skeleton skip. Today only `TestDaemonTick_KeepsARenumberedWaitingPaneTranscriptWhileSkeletonMarked` guards that arm.

UNSETTLED:
- "`TestDaemonTick_SkipsSkeletonMarkedPanesInScrollback` (`cmd/state_daemon_run_test.go`) passes unedited" / "`TestCaptureAndCommit_SkipsResumePendingPanes` ... passes unedited" / "`TestDaemonTick_CarriesAFailingWaitingSessionWithoutCapturingIt` ... passes unedited" — reading settles the behaviour. That the tests pass needs a `go test ./cmd` run, and that they are unedited needs a diff of the task's commit.
- "every suite driving it (`internal/restore/lazy_resume_panel_integration_test.go`, `lazy_resume_discard_integration_test.go`, `lazy_resume_renumbered_restore_integration_test.go`, `lazy_resume_burst_integration_test.go`) passes with no assertion changed" — needs `go test -tags integration -p 1 ./internal/restore` and a diff of the task's commit.
- "`rg -n 'paneSkipsScrollback' --glob '*.go'` finds nothing" — reading confirms the declaration and its sole call are gone from `cmd/state_daemon.go`. Absence across the whole repo needs the `rg` run.
- "`go test ./...`, `go test -tags integration -p 1 ./...` and `golangci-lint run` pass; the only test-file edit is the fixture's skip" — needs both lanes, the linter, and a diff of the task's commit restricted to `*_test.go`.
