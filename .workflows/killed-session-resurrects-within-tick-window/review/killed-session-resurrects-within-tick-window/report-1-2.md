TASK: Add `@portal-restoring` short-circuit to `commit-now` (killed-session-resurrects-within-tick-window-1-2)

ACCEPTANCE CRITERIA:
- Marker set → no-op, sessions.json byte-identical pre/post.
- Marker clear → proceed with structural commit.
- Short-circuit branch touches `save.requested` (daemon-fallback handoff).
- Touch failure on short-circuit is best-effort (WARN log, swallowed); still exit 0.
- INFO-level log under `state.ComponentDaemon` on marker-set skip.
- (Cycle-2 task 2-2) `IsRestoring` query error → symmetric to `(true, nil)`: presume marker set, WARN, proceed as short-circuit.

STATUS: Complete

SPEC CONTEXT:
Spec § `@portal-restoring` Defence (Required) mandates short-circuit, mirroring daemon's `tick()` entry guard. § `save.requested` Discipline requires the short-circuit (exit 0) to touch `save.requested` so the daemon's first post-restoration tick commits immediately. § `save.requested` Touch Failure Handling specifies best-effort touch: WARN, swallow, original exit (0) dominates. `_portal-saver` self-kill timeline 1 (bootstrap step 4 version-upgrade) depends on this short-circuit.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/state_commit_now.go:193-203` — short-circuit branch (query → switch → INFO log → touch → exit 0).
  - `cmd/state_commit_now.go:226-230` — `touchAfterShortCircuit` helper.
  - `cmd/state_commit_now.go:71-83` — `IsRestoring` and `TouchSaveRequested` DI seam fields.
  - `cmd/state_commit_now.go:98-99` — production defaults wiring `state.IsRestoringSet(tmux.DefaultClient())` and `state.TouchSaveRequested`.
- Notes:
  - `switch`/`case` dispatches three `IsRestoring` outcomes. Structural primitives never fire when marker is set or query errors.
  - Query-failure branch matches daemon's `cmd/state_daemon.go:95-100` discipline (same `ComponentDaemon`, same WARN-with-cause pattern).

TESTS:
- Status: Adequate
- Coverage (`cmd/state_commit_now_test.go`):
  - `TestStateCommitNow_ShortCircuits_DoesNotWriteSessionsJSONWhenRestoring` (line 557) — byte-equivalence, zero structural primitive calls.
  - `TestStateCommitNow_ShortCircuits_TouchesSaveRequested` (line 601).
  - `TestStateCommitNow_ShortCircuits_LogsInfoSkipEvent` (line 624) — INFO under ComponentDaemon mentioning `@portal-restoring`.
  - `TestStateCommitNow_ShortCircuits_ExitsZero` (line 656).
  - `TestStateCommitNow_ShortCircuits_ExitsZeroWhenSaveRequestedTouchFails` (line 673) — touch failure swallowed, exit 0, WARN logged.
  - `TestStateCommitNow_TreatsIsRestoringErrorAsMarkerPresumedSet` (line 714) — query-error path: byte-identity, no structural calls, touch fired, WARN with cause.
  - `TestStateCommitNow_ProceedsNormallyWhenRestoringClear` (line 783) — marker-clear happy path.
- Notes: Each enumerated edge case has a dedicated test. Not over-tested.

CODE QUALITY:
- Project conventions: Followed — cmd-package `*Deps`/`resolve*Deps()` DI idiom, logger nil-receiver no-op, Cobra `SilenceErrors`/`SilenceUsage`, no `t.Parallel()`.
- SOLID: Good — single-responsibility helpers.
- Complexity: Low — linear `RunE` with one switch dispatch.
- Modern idioms: `errors.Is`/`errors.Unwrap`, `%w` wrap, structured logger.
- Readability: Doc comments cite spec section names (lines 178-192).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] `touchAfterShortCircuit` log message doesn't differentiate marker-set vs query-failure callers. Each caller's preceding log supplies context — acceptable.
- [idea] Default `IsRestoring` closure instantiates a fresh `tmux.DefaultClient()` per call. Fine for one-shot subprocess.
- [quickfix] `restoringCals` typo in `commitNowFixture` test file line 80.
