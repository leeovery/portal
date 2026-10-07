TASK: Failed Session Listing Stands The Commit Cycle Down (killing-all-sessions-wipes-restore-state-1-1, tick-eb3a09)

ACCEPTANCE CRITERIA:
- The saved state holds sessions and their scrollback files. The production client's `list-sessions` fails while the shared commit cycle runs. The cycle returns an error, `sessions.json` is unchanged, and every scrollback file is still present with its content.
- With that saved state and the restore-in-progress marker reading as unset, the production client's `list-sessions` fails during the daemon's tick, during its shutdown flush and during `commit-now`. In each case `sessions.json` and every scrollback file are unchanged afterwards.
- A `list-sessions` succeeds and names only Portal's own underscore-prefixed sessions. That is not a failure: the capture yields an empty index with no error.
- The session listing fails while bootstrap's restore reads it. Restore rebuilds every session in the saved state, as it does today, and no commit of an empty index follows it.
- The session listing fails. The picker lists no sessions, the resolver matches no session, and shell completion offers no session names. None of them reports an error, exactly as today.

STATUS: complete

SPEC CONTEXT: Spec 2.1 makes a failed `list-sessions` an error to the shared commit cycle (`state.RunCommitCycle`), so the daemon's tick, its shutdown flush and `commit-now` all stand down instead of reading the failure as zero sessions and committing an empty index whose housekeeping pass deletes every transcript. The change must land on the committing path only. The shared listing (`ListSessions` / `ListSessionNames`) keeps reading a failure as "no server, no sessions" for the picker, resolver, completion and, above all, restore, because a failing shared listing would make restore bring back nothing and the next tick would then commit that empty index. Spec 2.5 says a stand-down writes no commit and runs no housekeeping pass. Spec 5.1 keeps the empty-save contract tests. Spec 6.6 reworks the fake-error capture subtest and renames the `TestListSessions` no-server case.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tmux/tmux.go:224-243: `ListSessionNames` stays over the swallowing `ListSessions` (tmux.go:151-158, swallow unchanged). New `ListSessionNamesProbe` runs over `ListSessionsProbe`, through the shared `sessionNames` helper.
  - internal/state/capture.go:20-30: `CaptureClient` now requires `ListSessionNamesProbe`, with a doc comment stating why a failed read must be an error.
  - internal/state/capture.go:95-103: `captureStructure` reads through the probe and returns the empty index plus the error before any pane or environment read. Later work (task 1.4 / 4-2) layered `ErrTmuxStoppedAnswering` wrapping and the unparseable-listing carve-out on top, and both are consistent with this task.
  - internal/state/scrollback.go:361-365 and internal/state/commit_cycle.go:127-130: a failed capture returns from `captureAndRefile` and from `RunCommitCycle` before the re-file, the dump, `commitOver` and housekeeping. Nothing is written or deleted.
  - Committers: the tick (cmd/state_daemon.go:203-208), the shutdown flush (cmd/state_daemon.go:389-393) and commit-now (cmd/state_commit_now.go:117-119) each end through their existing failure route.
  - Restore's listing read is unchanged (internal/restore/restore.go:114-124, still `ListSessionNames`). The resolver (internal/resolver/query.go:90,143,159), the picker (internal/tui/pending_resume.go:19) and completion (cmd/completion.go:16) still read the shared listing. No production caller of the swallowing listing sits on the committing path; the only production `ListSessionNamesProbe` implementation is `*tmux.Client`.
- Notes: No drift. The change touches only the committing path, as the task requires.

TESTS:
- Status: Adequate
- Coverage:
  - AC1: internal/state/commit_cycle_test.go:696-736 `TestRunCommitCycleStandsDownOnAFailedSessionListing` drives the production `*tmux.Client` over a scripted commander whose `list-sessions` returns a `*tmux.CommandError`. It asserts the error wraps the listing failure, the dump never runs, `sessions.json` is byte-identical, and the scrollback names and contents are unchanged. The test discriminates: under the old swallowing read, the confirmation answers own-server and an empty index commits over the seed.
  - AC2: cmd/state_commit_listing_failure_test.go:99-141 has three subtests: tick, `defaultShutdownFlush` and `state commit-now` through `runRootCmd`. Each runs a production client over `daemonFakeCommander`, which answers `@portal-restoring` as an absent option (unset through `TryGetServerOption`) and fails `list-sessions` with a `*tmux.CommandError`. Each asserts `sessions.json` and the scrollback contents are unchanged. commit-now also asserts `errCommitNowFailed`.
  - AC3: internal/state/capture_test.go:1677 ("returns an empty index with nil error when keep is empty after filtering", production client listing only `_portal-saver`) and internal/tmux/list_sessions_probe_test.go:164-175.
  - AC4: internal/restore/restore_test.go:800-848. Restore with a failing `list-sessions` creates both saved sessions. A following `RunCommitCycle` over the same client errors wrapping the listing failure and leaves `sessions.json` as restore left it.
  - AC5: internal/tui/session_listing_failure_test.go (picker: `SessionsMsg` with no sessions, nil `Err`), internal/resolver/query_listing_failure_test.go (bare chain and glob both miss without error), and cmd/completion_test.go:65-76 (default completion seam against a dead socket offers no names, directive NoFileComp).
  - The spec 6.6 rework is done. internal/state/capture_test.go:1618-1632 now drives the production client's failing `list-sessions` rather than a fake-only error, and also asserts no `list-panes` or `show-environment` call. The `TestListSessions` case is renamed to "returns empty slice when list-sessions fails, which the picker reads as no server" (internal/tmux/tmux_test.go:41).
  - The spec 5.1 empty-save contract tests are kept. capture_test.go:761 and :1677 are untouched. `TestStateCommitNow_WritesEmptySessionsJSONWhenZeroLiveSessions` (cmd/state_commit_now_test.go:151) is unchanged apart from the fake's method rename the interface change forces.
- Notes: The only overlap is a little, at the method level: `ListSessionNames` swallowing a failure is pinned both in list_sessions_probe_test.go:195 and indirectly by the `ListSessions` cases. These are distinct public methods, so nothing to report.

CODE QUALITY:
- Project conventions: Followed. DI through the narrow `CaptureClient` interface. Tests drive the production client over `commandertest` / `daemonFakeCommander` rather than fake-only errors. No `t.Parallel`. The cmd seams are staged via `withCommitNowDeps` / `withFuncSeam`. The completion test overrides `TMUX` to a temp-dir socket, so the real server is never reached.
- SOLID principles: Good. The probe and the swallowing listing share one parse and one name projection (`sessionNames`).
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. Comments in the changed code hold against it and reference no process artifacts.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
